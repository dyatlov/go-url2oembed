package url2oembed

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"time"

	// to fetch gif info from url
	_ "image/gif"
	// to fetch jpeg info from url
	_ "image/jpeg"
	// to fetch png info from url
	_ "image/png"

	"github.com/dyatlov/go-htmlinfo/htmlinfo"
	"github.com/dyatlov/go-oembed/oembed"
)

// Parser implements an url parsing code
type Parser struct {
	oe                *oembed.Oembed
	client            *http.Client
	MaxHTMLBodySize   int64
	MaxBinaryBodySize int64
	WaitTimeout       time.Duration
}

// OembedRedirectGoodError is a hack to stop following redirects and get oembed resource
type OembedRedirectGoodError struct {
	url  string
	item *oembed.Item
}

var (
	imageTypeRegex = regexp.MustCompile(`^image/.*`)
	htmlTypeRegex  = regexp.MustCompile(`^text/html`)
)

// GetItem return embed item
func (orge *OembedRedirectGoodError) GetItem() *oembed.Item {
	return orge.item
}

// GetURL returns url of resource with embeding implemented
func (orge *OembedRedirectGoodError) GetURL() string {
	return orge.url
}

func (orge *OembedRedirectGoodError) Error() string {
	return fmt.Sprintf("Found resource supporting oembed: %s", orge.url)
}

// NewParser returns new Parser instance
// Oembed pointer is optional, it just speeds up information gathering
func NewParser(oe *oembed.Oembed) *Parser {
	parser := &Parser{oe: oe}
	parser.init()
	return parser
}

func (p *Parser) skipRedirectIfFoundOembed(req *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return errors.New("stopped after 10 redirects")
	}

	if p.oe == nil {
		return nil
	}

	item := p.oe.FindItem(req.URL.String())

	if item != nil {
		return &OembedRedirectGoodError{url: req.URL.String(), item: item}
	}

	return nil
}

func (p *Parser) init() {
	p.MaxHTMLBodySize = 40000
	p.MaxBinaryBodySize = 1024
	p.WaitTimeout = 10 * time.Second
}

// Parse parses an url and returns structurized representation
func (p *Parser) Parse(u string) *oembed.Info {
	if p.client == nil {
		transport := &http.Transport{DisableKeepAlives: true}
		p.client = &http.Client{Timeout: p.WaitTimeout, Transport: transport, CheckRedirect: p.skipRedirectIfFoundOembed}
	}

	info := p.parseOembed(u)

	// and now we try to set missing image sizes
	if info != nil {
		// TODO: need to optimize this block, thats too much for 0 checking
		var width int64
		var err error
		width, err = info.ThumbnailWidth.Int64()
		if err != nil {
			width = 0
		}
		////
		if len(info.ThumbnailURL) > 0 && width == 0 {
			data, err := p.fetchURL(info.ThumbnailURL)
			if err == nil {
				config, _, err := image.DecodeConfig(bytes.NewReader(data))
				if err == nil {
					info.ThumbnailWidth = json.Number(strconv.FormatInt(int64(config.Width), 10))
					info.ThumbnailHeight = json.Number(strconv.FormatInt(int64(config.Height), 10))
				}
			}
		}
	}

	return info
}

func (p *Parser) parseOembed(u string) *oembed.Info {
	// check if we have it oembeded
	var item *oembed.Item

	if p.oe != nil {
		item := p.oe.FindItem(u)
		if item != nil {
			// try to extract information
			ei, _ := item.FetchOembed(u, p.client)
			if ei != nil {
				return ei
			}
		}
	}

	// fetch url
	data, err := p.fetchURL(u)

	if err != nil {
		if e, ok := err.(*url.Error); ok {
			if e, ok := e.Err.(*OembedRedirectGoodError); ok {

				item = e.GetItem()
				// TODO: optimize this.. calling the same code 2 times
				ei, _ := item.FetchOembed(e.GetURL(), p.client)
				if ei != nil {
					return ei
				}

				data, err = p.fetchURL(u)
			}
		}
	}

	if data != nil {
		contentType := http.DetectContentType(data)

		if imageTypeRegex.MatchString(contentType) {
			return p.getImageInfo(u, data)
		}

		if htmlTypeRegex.MatchString(contentType) {
			return p.FetchOembedFromHTML(u, data)
		}

		return p.getLinkInfo(u)
	}

	return nil
}

func (p *Parser) getImageInfo(u string, data []byte) *oembed.Info {
	pu, _ := url.Parse(u)

	if pu == nil {
		return nil
	}

	config, _, err := image.DecodeConfig(bytes.NewReader(data))

	info := oembed.NewInfo()
	info.Type = "photo"
	info.URL = u
	info.ProviderURL = "http://" + pu.Host
	info.ProviderName = pu.Host

	if err == nil {
		info.Width = json.Number(strconv.FormatInt(int64(config.Width), 10))
		info.Height = json.Number(strconv.FormatInt(int64(config.Height), 10))
	}

	return info
}

func (p *Parser) getLinkInfo(u string) *oembed.Info {
	pu, _ := url.Parse(u)

	if pu == nil {
		return nil
	}

	info := oembed.NewInfo()
	info.Type = "link"
	info.URL = u
	info.ProviderURL = "http://" + pu.Host
	info.ProviderName = pu.Host

	return info
}

// FetchOembedFromHTML returns information extracted from html page
func (p *Parser) FetchOembedFromHTML(pageURL string, data []byte) *oembed.Info {
	buf := bytes.NewReader(data)
	info := htmlinfo.NewHTMLInfo()
	info.Client = p.client
	info.AllowOembedFetching = true

	if info.Parse(buf, &pageURL) != nil {
		return nil
	}

	return info.GenerateOembedFor(pageURL)
}

func (p *Parser) fetchURL(url string) (data []byte, err error) {
	resp, err := p.client.Get(url)
	if err != nil {
		return
	}

	defer resp.Body.Close()

	contentType := resp.Header.Get("Content-Type")

	var reader io.Reader

	// if we have some raw stream then we can't parse html, so need just mime
	if contentType == "" || htmlTypeRegex.MatchString(contentType) {
		reader = io.LimitReader(resp.Body, p.MaxHTMLBodySize)
	} else {
		reader = io.LimitReader(resp.Body, p.MaxBinaryBodySize)
	}

	data, err = ioutil.ReadAll(reader)

	return
}

package url2oembed

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"io"
	"io/ioutil"
	"net"
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
	AcceptLanguage    string
	UserAgent         string
	MaxHTMLBodySize   int64
	MaxBinaryBodySize int64
	WaitTimeout       time.Duration
	fetchURLCalls     int

	// list of IP addresses to blacklist
	BlacklistedIPNetworks []*net.IPNet

	// list of IP addresses to whitelist
	WhitelistedIPNetworks []*net.IPNet
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
	if p.fetchURLCalls >= 10 {
		return errors.New("stopped after 10 redirects")
	}

	p.fetchURLCalls++

	if p.oe == nil {
		return nil
	}

	item := p.oe.FindItem(req.URL.String())

	if item != nil {
		return &OembedRedirectGoodError{url: req.URL.String(), item: item}
	}

	// mutate the subsequent redirect requests with the first Header
	for key, val := range via[0].Header {
		req.Header[key] = val
	}

	return nil
}

func (p *Parser) init() {
	p.MaxHTMLBodySize = 50000
	p.MaxBinaryBodySize = 4096
	p.AcceptLanguage = "en-us"
	p.UserAgent = "ProcLink Bot http://proc.link"
	p.WaitTimeout = 10 * time.Second
}

func (p *Parser) isBlacklistedIP(addr net.IP) bool {
	// if whitelisted then return false
	for _, w := range p.WhitelistedIPNetworks {
		if w.Contains(addr) {
			return false
		}
	}

	// if blacklisted then return true
	for _, b := range p.BlacklistedIPNetworks {
		if b.Contains(addr) {
			return true
		}
	}

	// by default we disable local addresses and bradcast ones
	return !addr.IsGlobalUnicast()
}

func (p *Parser) filterBlacklistedIPs(addrs []net.IP) ([]net.IP, bool) {
	isBlacklisted := false

	var whiteListed []net.IP

	for _, a := range addrs {
		if p.isBlacklistedIP(a) {
			isBlacklisted = true
		} else {
			whiteListed = append(whiteListed, a)
		}
	}

	return whiteListed, isBlacklisted
}

// Dial is used to disable access to blacklisted IP addresses
func (p *Parser) Dial(network, addr string) (net.Conn, error) {
	var (
		host, port string
		err        error
		addrs      []net.IP
	)

	if host, port, err = net.SplitHostPort(addr); err != nil {
		return nil, err
	}

	if addrs, err = net.LookupIP(host); err != nil {
		return nil, err
	}

	if whiteListed, isBlacklisted := p.filterBlacklistedIPs(addrs); isBlacklisted {
		if len(whiteListed) == 0 {
			return nil, errors.New("Host is blacklisted")
		}
		// select first good one
		firstGood := whiteListed[0]
		if len(whiteListed) > 1 {
			for _, candidate := range whiteListed[1:] {
				if candidate.To4() != nil { // we prefer IPv4
					firstGood = candidate
					break
				}
			}
		}

		addr = net.JoinHostPort(firstGood.String(), port)
	}

	return net.Dial(network, addr)
}

// Parse parses an url and returns structurized representation
func (p *Parser) Parse(u string) *oembed.Info {
	if p.client == nil {
		transport := &http.Transport{DisableKeepAlives: true, Dial: p.Dial}
		p.client = &http.Client{Timeout: p.WaitTimeout, Transport: transport, CheckRedirect: p.skipRedirectIfFoundOembed}
	}

	p.fetchURLCalls = 0
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
			p.fetchURLCalls = 0
			data, newURL, _, err := p.fetchURL(info.ThumbnailURL)
			if err == nil {
				info.ThumbnailURL = newURL
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

	var srvContentType string

	if p.oe != nil {
		item := p.oe.FindItem(u)
		if item != nil {
			// try to extract information
			ei, _ := item.FetchOembed(u, p.client)
			if ei != nil && ei.Status < 300 {
				return ei
			}
		}
	}

	// fetch url
	data, newURL, srvContentType, err := p.fetchURL(u)

	if err != nil {
		for {
			if e, ok := err.(*url.Error); ok {
				if e, ok := e.Err.(*OembedRedirectGoodError); ok {
					item = e.GetItem()
					// TODO: optimize this.. calling the same code 2 times
					ei, _ := item.FetchOembed(e.GetURL(), p.client)
					if ei != nil && ei.Status < 300 {
						return ei
					}

					data, newURL, srvContentType, err = p.fetchURL(e.GetURL())

					if err != nil {
						continue
					}
				}
			}

			break
		}
	}

	if data != nil {
		u = newURL

		contentType := http.DetectContentType(data)

		if imageTypeRegex.MatchString(contentType) {
			return p.getImageInfo(u, data)
		}

		if htmlTypeRegex.MatchString(contentType) {
			return p.FetchOembedFromHTML(u, data, srvContentType)
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
func (p *Parser) FetchOembedFromHTML(pageURL string, data []byte, contentType string) *oembed.Info {
	buf := bytes.NewReader(data)
	info := htmlinfo.NewHTMLInfo()
	info.Client = p.client
	info.AcceptLanguage = p.AcceptLanguage
	info.AllowOembedFetching = true

	if info.Parse(buf, &pageURL, &contentType) != nil {
		return nil
	}

	return info.GenerateOembedFor(pageURL)
}

func (p *Parser) fetchURL(url string) (data []byte, u string, contentType string, err error) {
	p.fetchURLCalls++

	req, err := http.NewRequest("GET", url, nil)

	if err != nil {
		return
	}

	req.Header.Add("Accept-Language", p.AcceptLanguage)
	req.Header.Set("User-Agent", p.UserAgent)

	resp, err := p.client.Do(req)
	if err != nil {
		return
	}

	defer resp.Body.Close()

	u = resp.Request.URL.String()

	contentType = resp.Header.Get("Content-Type")

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

package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net"

	"github.com/dyatlov/go-oembed/oembed"
	"github.com/dyatlov/go-url2oembed/url2oembed"
)

// StringsToNetworks converts arrays of string representation of IP ranges into []*net.IPnet slice
func StringsToNetworks(ss []string) ([]*net.IPNet, error) {
	var result []*net.IPNet
	for _, s := range ss {
		_, network, err := net.ParseCIDR(s)
		if err != nil {
			return nil, err
		}
		result = append(result, network)
	}

	return result, nil
}

func main() {
	providersData, err := ioutil.ReadFile("providers.json")

	if err != nil {
		panic(err)
	}

	oe := oembed.NewOembed()
	oe.ParseProviders(bytes.NewReader(providersData))

	parser := url2oembed.NewParser(oe)
	if parser.BlacklistedIPNetworks, err = StringsToNetworks([]string{"195.59.58.240/32"}); err != nil {
		panic(err)
	}

	data := parser.Parse("http://techcrunch.com/2010/11/02/365-days-10-million-3-rounds-2-companies-all-with-5-magic-slides/")

	fmt.Printf("%s\n", data)
}

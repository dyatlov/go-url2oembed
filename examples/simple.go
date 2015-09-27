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

	data := parser.Parse("http://mashable.com/2015/09/11/troye-sivan-wild-taylor-swift/")

	fmt.Printf("%s\n", data)
}

package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net"

	"github.com/dyatlov/go-oembed/oembed"
	"github.com/dyatlov/go-url2oembed/url2oembed"
)

func main() {
	providersData, err := ioutil.ReadFile("providers.json")

	if err != nil {
		panic(err)
	}

	oe := oembed.NewOembed()
	oe.ParseProviders(bytes.NewReader(providersData))

	parser := url2oembed.NewParser(oe)
	parser.BlacklistedIPs = []net.IP{net.ParseIP("195.59.58.240"), net.ParseIP("77.67.21.248")}

	data := parser.Parse("http://mashable.com/2015/09/11/troye-sivan-wild-taylor-swift/")

	fmt.Printf("%s\n", data)
}

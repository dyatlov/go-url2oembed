Go URL2Oembed
===

Package provide just one method: `Parse`. It returns structurized oembed representation of an url.

All url types are supported (html pages, images, videos, binary data).

Source docs: http://godoc.org/github.com/dyatlov/go-url2oembed/url2oembed

Install: `go get github.com/dyatlov/go-url2oembed/url2oembed`

Use: `import "github.com/dyatlov/go-url2oembed/url2oembed"`

Example:

```go
package main

import (
	"bytes"
	"fmt"
	"io/ioutil"

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

	data := parser.Parse("http://mashable.com/2015/09/11/troye-sivan-wild-taylor-swift/")

	fmt.Printf("%s\n", data)
}
```

Output:

```js
{
    "type": "link",
    "url": "http://mashable.com/2015/09/11/troye-sivan-wild-taylor-swift/",
    "provider_url": "http://mashable.com",
    "provider_name": "Mashable",
    "title": "'Insanity ensued' after Taylor Swift endorsed Troye Sivan's new music",
    "description": "\"I hardly slept that night — feels so amazing to be validated by someone who I look up to so much,\" Troye Sivan explains after Taylor Swift tweeted him. ",
    "width": 0,
    "height": 0,
    "thumbnail_url": "http://rack.0.mshcdn.com/media/ZgkyMDE1LzA5LzExLzFmL3RheWxvcnN3aWZ0LmY5NTdhLmpwZwpwCXRodW1iCTEyMDB4NjI3IwplCWpwZw/c5d0564f/9c2/taylor-swift-troye-sivan-endorsement.jpg",
    "thumbnail_width": 1200,
    "thumbnail_height": 627,
    "author_name": "",
    "author_url": "",
    "html": ""
}
```

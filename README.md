# go-xmlrpc

xmlrpc interface for go

## Usage

```go
package main

import (
	"github.com/mattn/go-xmlrpc"
	"fmt"
	"log"
)

func main() {
	res, e := xmlrpc.NewClient(
		"http://your-blog.example.com/xmlrpc.php",
	).Call(
		"metaWeblog.getRecentPosts",
		"blog-id",
		"user-id",
		"password",
		10,
	)
	if e != nil {
		log.Fatal(e)
	}
	for _, p := range res {
		for k, v := range p.(xmlrpc.Struct) {
			fmt.Printf("%s=%v\n", k, v)
		}
		fmt.Println()
	}
}
```

## Installation

```
$ go get github.com/mattn/go-xmlrpc
```

## License

MIT

## Author

Yasuhiro Matsumoto (a.k.a. mattn)

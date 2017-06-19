# gyfcat
Gyfcat API client

## Sample usage
* Preamble:
```go
package main

import (
	"fmt"
	"log"

	"github.com/orijtech/gfycat/v1"
)
```

* Search
```go
func search() {
	client := new(gfycat.Client)

	res, err := client.Search(&gfycat.Request{
		Query:  "NBA finals",
		InTest: true,

		MaxPageNumber: 4,
	})
	if err != nil {
		log.Fatal(err)
	}

	for page := range res.Pages {
		if page.Err != nil {
			fmt.Printf("%#d: err: %v\n", page.PageNumber, page.Err)
			continue
		}

		for i, gfy := range page.Gfys {
			fmt.Printf("\t(#%d): %#v\n", i, gfy)
		}
	}
}
```

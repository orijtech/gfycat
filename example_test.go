// Copyright 2017 orijtech. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package gfycat_test

import (
	"context"
	"fmt"
	"log"

	"github.com/orijtech/gfycat/v1"
)

func Example_client_Search() {
	client := new(gfycat.Client)

	res, err := client.Search(context.Background(), &gfycat.Request{
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

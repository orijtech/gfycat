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

package gfycat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"sync"
	"time"

	"go.opencensus.io/plugin/ochttp"
	"go.opencensus.io/trace"

	"github.com/orijtech/otils"
)

const (
	baseURL     = "https://api.gfycat.com/v1/gfycats"
	testBaseURL = "https://api.gfycat.com/v1test/gfycats"
)

type Client struct {
	sync.RWMutex
	apiKey string

	rt http.RoundTripper
}

func (c *Client) SetHTTPRoundTripper(rt http.RoundTripper) {
	c.Lock()
	c.rt = rt
	c.Unlock()
}

type Gfy struct {
	GfyID      string   `json:"gfyid,omitempty"`
	GfyName    string   `json:"gfyName"`
	GfyNumber  int64    `json:"gfyNumber,string"`
	URL        string   `json:"url"`
	Width      int      `json:"width,omitempty"`
	Height     int      `json:"height,omitempty"`
	FrameRate  float32  `json:"frameRate,omitempty"`
	FrameCount int      `json:"numFrames,omitempty"`
	MP4Size    int64    `json:"mp4Size,omitempty"`
	WebmSize   int64    `json:"webmSize,omitempty"`
	GIFSize    int64    `json:"gifSize,omitempty"`
	MD5Sum     string   `json:"md5,omitempty"`
	Tags       []string `json:"tags,omitempty"`
	Owner      string   `json:"username,omitempty"`
	Title      string   `json:"title,omitempty"`

	Published otils.NumericBool `json:"published,omitempty"`

	ViewCount       int64    `json:"views,omitempty"`
	AverageColor    string   `json:"avgColor,omitempty"`
	CreateDateEpoch int64    `json:"createDate,omitempty"`
	Description     string   `json:"description,omitempty"`
	DomainWhitelist []string `json:"domainWhitelist,omitempty"`

	MP4URL            string `json:"mp4Url"`
	WebmURL           string `json:"webmUrl"`
	MobileURL         string `json:"mobileUrl"`
	MobilePosterURL   string `json:"mobilePosterUrl"`
	MiniURL           string `json:"miniUrl"`
	MiniPosterURL     string `json:"miniPosterUrl"`
	PosterURL         string `json:"posterUrl"`
	Thumb360URL       string `json:"thumb360Url"`
	Thumb360PosterURL string `json:"thumb360PosterUrl"`
	Thumb100PosterURL string `json:"thumb100PosterUrl"`

	Max5MBGIFURL string `json:"max5mbGif"`
	Max2MBGIFURL string `json:"max2mbGif"`
	Max1MBGIFURL string `json:"max1mbGif"`
	GIF100PxURL  string `json:"gif100px"`
	MJPGURL      string `json:"mjpUrl"`
	WebpURL      string `json:"webpUrl"`

	NSFW otils.NumericBool `json:"nsfw"`
}

type Request struct {
	Query         string `json:"query"`
	LimitPerPage  int    `json:"limit_per_page"`
	MaxPageNumber uint64 `json:"max_page_number"`

	ThrottleDurationMs int `json:"throttle_duration_ms"`

	// InTest if set, signifies that the app shouldn't
	// use the production endpoint and should instead use the test
	// endpoint. Useful for running tests and before one is authenticated.
	InTest bool `json:"in_test"`
}

func (req *Request) baseURL() string {
	if req.InTest {
		return testBaseURL
	}
	return baseURL
}

const NoThrottle = -1

type Page struct {
	Err  error  `json:"error"`
	Gfys []*Gfy `json:"gfys"`

	PageNumber uint64 `json:"page_number"`
}

type SearchResponse struct {
	Pages  <-chan *Page
	Cancel func() error
}

var errAlreadyCanceled = errors.New("already canceled")

func makeCanceler() (<-chan struct{}, func() error) {
	var cancelOnce sync.Once
	cancelChan := make(chan struct{})

	cancelFn := func() error {
		err := errAlreadyCanceled
		cancelOnce.Do(func() {
			close(cancelChan)
			err = nil
		})
		return err
	}
	return cancelChan, cancelFn
}

type resultsWrap struct {
	Cursor string `json:"cursor"`
	Count  int64  `json:"found"`
	Gfys   []*Gfy `json:"gfycats"`
}

func (c *Client) Search(ctx context.Context, req *Request) (*SearchResponse, error) {
	ctx, span := trace.StartSpan(ctx, "gfycat/v1.(*Client).Search")
	defer span.End()

	if req == nil {
		req = new(Request)
	}

	maxPageNumber := req.MaxPageNumber
	pageExceeds := func(pageNumber uint64) bool {
		if maxPageNumber <= 0 {
			return false
		}

		return pageNumber >= maxPageNumber
	}

	cancelChan, cancelFn := makeCanceler()
	pagesChan := make(chan *Page, 1)

	throttleDurationMs := time.Duration(0)
	if req.ThrottleDurationMs != NoThrottle {
		throttleDurationMs = time.Duration(req.ThrottleDurationMs) * time.Millisecond
	}

	go func() {
		defer close(pagesChan)

		cursor := ""
		pageNumber := uint64(0)

		for {
			page := &Page{PageNumber: pageNumber}
			pager := &pager{
				LimitPerPage: req.LimitPerPage,
				Cursor:       cursor,
				SearchText:   req.Query,
			}

			qv, err := otils.ToURLValues(pager)
			if err != nil {
				page.Err = err
				pagesChan <- page
				return
			}

			cctx, span := trace.StartSpan(ctx, "paging")
			span.Annotate([]trace.Attribute{
				trace.Int64Attribute("page", int64(pageNumber)),
			}, "page")

			theURL := fmt.Sprintf("%s/search", req.baseURL())
			if len(qv) > 0 {
				theURL = fmt.Sprintf("%s?%s", theURL, qv.Encode())
			}
			httpReq, err := http.NewRequest("GET", theURL, nil)
			if err != nil {
				span.End()
				page.Err = err
				pagesChan <- page
				return
			}

			slurp, _, err := c.doHTTPReq(cctx, httpReq)
			if err != nil {
				page.Err = err
				pagesChan <- page
				return
			}

			rWrap := new(resultsWrap)
			err = json.Unmarshal(slurp, rWrap)
			span.End()
			if err != nil {
				page.Err = err
				pagesChan <- page
				return
			}

			cursor = rWrap.Cursor
			if cursor == "" {
				return
			}

			if len(rWrap.Gfys) == 0 {
				return
			}

			page.Gfys = rWrap.Gfys
			pagesChan <- page
			pageNumber += 1
			if pageExceeds(pageNumber) {
				return
			}

			select {
			case <-cancelChan:
				return
			case <-time.After(throttleDurationMs):
			}
		}
	}()

	sres := &SearchResponse{
		Cancel: cancelFn,
		Pages:  pagesChan,
	}

	return sres, nil
}

func (c *Client) doHTTPReq(ctx context.Context, req *http.Request) ([]byte, http.Header, error) {
	ctx, span := trace.StartSpan(ctx, "gfycat/v1.(*Client).doHTTPReq")
	defer span.End()

	res, err := c.httpClient().Do(req)
	if err != nil {
		return nil, nil, err
	}
	if res.Body != nil {
		defer res.Body.Close()
	}

	if !otils.StatusOK(res.StatusCode) {
		return nil, res.Header, errors.New(res.Status)
	}
	slurp, err := ioutil.ReadAll(res.Body)
	return slurp, res.Header, err
}

type pager struct {
	Cursor       string `json:"cursor,omitempty"`
	LimitPerPage int    `json:"count,omitempty"`
	SearchText   string `json:"search_text,omitempty"`
}

func (c *Client) httpClient() *http.Client {
	c.RLock()
	rt := c.rt
	c.RUnlock()

	return &http.Client{Transport: &ochttp.Transport{Base: rt}}
}

// Package upstream fetches the upstream RSSHub feed into memory. Unlike pkg/proxy
// (which streams straight to the response), the handled branch needs the whole
// body buffered so pkg/feed can parse it with etree — hence a separate client.
package upstream

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"resty.dev/v3"
)

// Response is a buffered upstream reply.
type Response struct {
	Status      int
	ContentType string
	Body        []byte
}

// Fetcher GETs feeds from a fixed upstream base URL.
type Fetcher struct {
	base   *url.URL
	client *resty.Client
}

// New builds a Fetcher for the upstream base URL with a per-request timeout. It
// returns an error if baseURL is unparseable or missing scheme/host.
func New(baseURL string, timeout time.Duration) (*Fetcher, error) {
	base, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("parse upstream base url %q: %w", baseURL, err)
	}
	if base.Scheme == "" || base.Host == "" {
		return nil, fmt.Errorf("upstream base url %q must include scheme and host", baseURL)
	}
	return &Fetcher{base: base, client: resty.New().SetTimeout(timeout)}, nil
}

// Fetch GETs path (with rawQuery preserved) from the upstream and buffers the
// reply. A transport error is returned as err; a non-2xx status is returned as
// data so the caller can pass it through unchanged.
func (f *Fetcher) Fetch(ctx context.Context, path, rawQuery string) (*Response, error) {
	target := *f.base
	target.Path = singleSlash(f.base.Path + path)
	target.RawQuery = rawQuery

	resp, err := f.client.R().SetContext(ctx).Get(target.String())
	if err != nil {
		return nil, fmt.Errorf("fetch upstream %s: %w", target.String(), err)
	}
	return &Response{
		Status:      resp.StatusCode(),
		ContentType: resp.Header().Get("Content-Type"),
		Body:        resp.Bytes(),
	}, nil
}

func singleSlash(p string) string {
	for strings.Contains(p, "//") {
		p = strings.ReplaceAll(p, "//", "/")
	}
	return p
}

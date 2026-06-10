// Package proxy reverse-proxies requests to the single upstream RSSHub instance.
// It is used for paths that do not match an enabled prefix (and for explicit
// ?format=json on enabled prefixes): the request and response pass through
// unchanged and nothing is cached (spec §2, §3).
package proxy

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/labstack/echo/v5"
	"go.uber.org/zap"

	"github.com/eli-yip/rss-ai/pkg/mlog"
)

// Proxy forwards requests to a fixed upstream base URL.
type Proxy struct {
	rp     *httputil.ReverseProxy
	logger *mlog.Logger
}

// New builds a passthrough proxy for the upstream base URL. It returns an error
// if baseURL is unparseable.
func New(baseURL string, logger *mlog.Logger) (*Proxy, error) {
	base, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("parse upstream base url %q: %w", baseURL, err)
	}
	if base.Scheme == "" || base.Host == "" {
		return nil, fmt.Errorf("upstream base url %q must include scheme and host", baseURL)
	}

	svcLogger := mlog.S(logger, "proxy")
	rp := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(base)         // scheme + host (+ any base path prefix)
			pr.Out.Host = base.Host // route by upstream host, not the inbound Host
			// SetURL joins base.Path ahead of the inbound path; when base has no
			// path this leaves the inbound path untouched. Guard against a stray
			// double slash when base path is "/".
			pr.Out.URL.Path = singleSlash(pr.Out.URL.Path)
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			mlog.L(r.Context(), svcLogger).Warn("upstream fetch failed",
				zap.String("upstream_url", r.URL.String()), zap.Error(err))
			w.WriteHeader(http.StatusBadGateway)
		},
	}

	return &Proxy{rp: rp, logger: svcLogger}, nil
}

// Passthrough forwards c's request to the upstream and streams the response back
// unchanged. The proxy writes directly to the response, so this returns nil.
func (p *Proxy) Passthrough(c *echo.Context) error {
	p.rp.ServeHTTP(c.Response(), c.Request())
	return nil
}

func singleSlash(p string) string {
	for strings.Contains(p, "//") {
		p = strings.ReplaceAll(p, "//", "/")
	}
	return p
}

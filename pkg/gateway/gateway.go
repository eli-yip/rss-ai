// Package gateway is the catch-all entry point. For each request it resolves the
// path against the enabled prefixes (spec §3/§4) and decides handled vs
// passthrough. plan-1 reverse-proxies both branches to upstream unchanged; the
// handled branch is the seam where plan-2 inserts fetch -> rewrite -> cache.
package gateway

import (
	"github.com/labstack/echo/v5"
	"go.uber.org/zap"

	"github.com/eli-yip/rss-ai/pkg/handler"
	"github.com/eli-yip/rss-ai/pkg/mlog"
	"github.com/eli-yip/rss-ai/pkg/proxy"
)

// mode is the resolved disposition of a request.
type mode string

const (
	modePassthrough mode = "passthrough" // unregistered path, or explicit ?format=json
	modeHandled     mode = "handled"     // enabled prefix, XML output
)

// Gateway ties handler resolution to the upstream proxy.
type Gateway struct {
	resolver *handler.Resolver
	proxy    *proxy.Proxy
	logger   *mlog.Logger
}

func New(resolver *handler.Resolver, proxy *proxy.Proxy, logger *mlog.Logger) *Gateway {
	return &Gateway{resolver: resolver, proxy: proxy, logger: mlog.S(logger, "gateway")}
}

// decide resolves the request path and returns the resolution plus the mode.
// Explicit ?format=json is always passthrough — surgical title rewriting is
// XML-only, so JSON Feed is never modified even on an enabled prefix (spec §3).
func (g *Gateway) decide(c *echo.Context) (handler.Resolution, mode) {
	res := g.resolver.Resolve(c.Request().URL.Path)
	if !res.Matched {
		return res, modePassthrough
	}
	if c.QueryParam("format") == "json" {
		return res, modePassthrough
	}
	return res, modeHandled
}

// Handle is the catch-all echo handler.
func (g *Gateway) Handle(c *echo.Context) error {
	res, m := g.decide(c)

	mlog.L(c.Request().Context(), g.logger).Debug("gateway resolve",
		zap.String("path", c.Request().URL.Path),
		zap.String("handler", res.Prefix),
		zap.String("mode", string(m)),
		zap.Bool("specialized", res.Specialized),
	)

	// plan-1: both branches pass through to upstream unchanged.
	// TODO plan-2: for modeHandled, fetch upstream -> rewrite titles via
	// res.Handler / res.EffectivePrompt() -> cache -> return modified XML.
	return g.proxy.Passthrough(c)
}

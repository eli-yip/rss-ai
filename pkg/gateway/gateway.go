// Package gateway is the catch-all entry point. For each request it resolves the
// path against the enabled prefixes (spec §3/§4) and decides handled vs
// passthrough. The handled branch buffers the upstream feed, rewrites item titles
// (RSS 2.0 only), and returns modified XML; passthrough (unregistered paths,
// ?format=json, ?format=atom) reverse-proxies upstream unchanged.
package gateway

import (
	"net/http"
	"time"

	"github.com/labstack/echo/v5"
	"go.uber.org/zap"

	"github.com/eli-yip/rss-ai/pkg/handler"
	"github.com/eli-yip/rss-ai/pkg/mlog"
	"github.com/eli-yip/rss-ai/pkg/proxy"
	"github.com/eli-yip/rss-ai/pkg/rewrite"
	"github.com/eli-yip/rss-ai/pkg/upstream"
)

// mode is the resolved disposition of a request.
type mode string

const (
	modePassthrough mode = "passthrough" // unregistered path, or explicit json/atom
	modeHandled     mode = "handled"     // enabled prefix, default RSS 2.0 output
)

// Gateway ties handler resolution to the upstream proxy (passthrough) and the
// fetch + rewrite pipeline (handled).
type Gateway struct {
	resolver *handler.Resolver
	proxy    *proxy.Proxy
	fetcher  *upstream.Fetcher
	rewriter *rewrite.Rewriter
	logger   *mlog.Logger
}

func New(resolver *handler.Resolver, proxy *proxy.Proxy, fetcher *upstream.Fetcher, rewriter *rewrite.Rewriter, logger *mlog.Logger) *Gateway {
	return &Gateway{
		resolver: resolver,
		proxy:    proxy,
		fetcher:  fetcher,
		rewriter: rewriter,
		logger:   mlog.S(logger, "gateway"),
	}
}

// decide resolves the request path and returns the resolution plus the mode.
// Explicit ?format=json or ?format=atom is always passthrough — surgical title
// rewriting is RSS-2.0-only this release, so other formats are never modified
// even on an enabled prefix (spec §3, plan-2 deviation).
func (g *Gateway) decide(c *echo.Context) (handler.Resolution, mode) {
	res := g.resolver.Resolve(c.Request().URL.Path)
	if !res.Matched {
		return res, modePassthrough
	}
	switch c.QueryParam("format") {
	case "json", "atom":
		return res, modePassthrough
	}
	return res, modeHandled
}

// Handle is the catch-all echo handler. It emits one request.done aggregate event
// per request (spec §11.4); per-item cache detail is logged at DEBUG in
// pkg/rewrite.
func (g *Gateway) Handle(c *echo.Context) error {
	start := time.Now()
	res, m := g.decide(c)

	var (
		status int
		stats  rewrite.Stats
		err    error
	)
	if m == modeHandled {
		status, stats, err = g.handleRewrite(c, res)
	} else {
		err = g.proxy.Passthrough(c)
		if resp, uerr := echo.UnwrapResponse(c.Response()); uerr == nil {
			status = resp.Status
		}
	}

	mlog.L(c.Request().Context(), g.logger).Info("request.done",
		zap.String("path", c.Request().URL.Path),
		zap.String("handler", res.Prefix),
		zap.String("mode", string(m)),
		zap.Int("status", status),
		zap.Int64("duration_ms", time.Since(start).Milliseconds()),
		zap.Int("item_count", stats.ItemCount),
		zap.Int("cache_hits", stats.CacheHits),
		zap.Int("cache_misses", stats.CacheMisses),
		zap.Int("ai_calls", stats.AICalls),
		zap.Bool("timed_out", stats.TimedOut),
	)
	return err
}

// handleRewrite fetches the upstream feed, rewrites item titles, and writes the
// modified XML. A transport error becomes 502; a non-2xx upstream and an
// unparseable feed are passed through unchanged.
func (g *Gateway) handleRewrite(c *echo.Context, res handler.Resolution) (int, rewrite.Stats, error) {
	ctx := c.Request().Context()
	var stats rewrite.Stats

	resp, err := g.fetcher.Fetch(ctx, c.Request().URL.Path, c.Request().URL.RawQuery)
	if err != nil {
		mlog.L(ctx, g.logger).Warn("upstream.fetch",
			zap.String("path", c.Request().URL.Path), zap.Error(err))
		return http.StatusBadGateway, stats, c.NoContent(http.StatusBadGateway)
	}
	if resp.Status < 200 || resp.Status >= 300 {
		return resp.Status, stats, writeBlob(c, resp.Status, resp.ContentType, resp.Body)
	}

	out, stats, rerr := g.rewriter.RewriteFeed(ctx, res, resp.Body)
	if rerr != nil {
		// RewriteFeed returns the raw bytes on a parse error; serve them as-is.
		mlog.L(ctx, g.logger).Warn("rewrite failed; serving upstream unchanged", zap.Error(rerr))
	}
	return http.StatusOK, stats, writeBlob(c, http.StatusOK, resp.ContentType, out)
}

func writeBlob(c *echo.Context, status int, contentType string, body []byte) error {
	if contentType == "" {
		contentType = "application/xml"
	}
	return c.Blob(status, contentType, body)
}

// Package rewrite is the handled-branch orchestrator (spec §5.3 + §7). It turns
// raw RSS plus a handler.Resolution into rewritten XML: cache-first per item, one
// AI call per (handler, id) via singleflight, an AI-rate limiter, and the
// timeout-vs-cancel rule — the AI call runs on a detached, longer-lived context
// so a request that gives up after wait_timeout still gets its row filled in the
// background.
package rewrite

import (
	"context"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
	"golang.org/x/sync/singleflight"
	"golang.org/x/time/rate"

	"github.com/eli-yip/rss-ai/pkg/aiclient"
	"github.com/eli-yip/rss-ai/pkg/feed"
	"github.com/eli-yip/rss-ai/pkg/handler"
	"github.com/eli-yip/rss-ai/pkg/mlog"
	"github.com/eli-yip/rss-ai/pkg/store"
)

// Cache is the subset of *store.Store the rewriter needs. Declared here (consumer
// side) so unit tests can inject an in-memory fake; *store.Store satisfies it.
type Cache interface {
	LookupTitles(ctx context.Context, handler string, ids []string) (map[string]store.ItemTitle, error)
	SaveTitle(ctx context.Context, it store.ItemTitle) error
}

// Stats is the per-feed outcome, surfaced into the gateway's request.done event.
type Stats struct {
	ItemCount   int
	CacheHits   int
	CacheMisses int
	AICalls     int
	TimedOut    bool
}

// Rewriter rewrites feed item titles with caching + bounded concurrency.
type Rewriter struct {
	cache       Cache
	ai          aiclient.Client
	limiter     *rate.Limiter
	group       singleflight.Group
	aiTimeout   time.Duration
	waitTimeout time.Duration
	logger      *mlog.Logger
}

// New builds a Rewriter. rpm bounds the AI call rate (defaults to 10000 when
// non-positive); aiTimeout caps each background AI call; waitTimeout caps how
// long a request waits before falling back to source titles.
func New(cache Cache, ai aiclient.Client, rpm int, aiTimeout, waitTimeout time.Duration, logger *mlog.Logger) *Rewriter {
	if rpm <= 0 {
		rpm = 10000
	}
	burst := max(rpm/60, 1)
	return &Rewriter{
		cache:       cache,
		ai:          ai,
		limiter:     rate.NewLimiter(rate.Limit(float64(rpm)/60.0), burst),
		aiTimeout:   aiTimeout,
		waitTimeout: waitTimeout,
		logger:      mlog.S(logger, "rewrite"),
	}
}

// RewriteFeed parses raw, rewrites every item title (cache-first, AI on miss),
// and returns the modified XML plus stats. On a parse error it returns raw
// unchanged so an unparseable feed is never corrupted.
func (rw *Rewriter) RewriteFeed(ctx context.Context, res handler.Resolution, raw []byte) ([]byte, Stats, error) {
	var stats Stats

	doc, err := feed.Parse(raw)
	if err != nil {
		return raw, stats, err
	}
	items := doc.Items()
	stats.ItemCount = len(items)

	ids := make([]string, 0, len(items))
	for _, it := range items {
		if it.ID != "" {
			ids = append(ids, it.ID)
		}
	}

	cached, err := rw.cache.LookupTitles(ctx, res.Prefix, ids)
	if err != nil {
		// Degrade to no-cache (still rewrite) rather than serving raw titles.
		mlog.L(ctx, rw.logger).Warn("cache lookup failed", zap.String("handler", res.Prefix), zap.Error(err))
		cached = map[string]store.ItemTitle{}
	}

	type future struct {
		idx int
		ch  <-chan singleflight.Result
	}
	var futures []future
	var aiCalls atomic.Int64

	for i, it := range items {
		if it.ID == "" {
			continue // unidentifiable; leave the title untouched
		}
		if row, ok := cached[it.ID]; ok && row.SourceTitle == it.Title {
			doc.SetTitle(i, row.RewrittenTitle)
			stats.CacheHits++
			continue
		}
		stats.CacheMisses++
		futures = append(futures, future{idx: i, ch: rw.rewriteAsync(ctx, res, it, &aiCalls)})
	}

	if len(futures) > 0 {
		deadline := time.Now().Add(rw.waitTimeout)
		for _, f := range futures {
			timer := time.NewTimer(max(time.Until(deadline), 0))
			select {
			case r := <-f.ch:
				timer.Stop()
				if r.Err == nil {
					if title, ok := r.Val.(string); ok {
						doc.SetTitle(f.idx, title)
					}
				}
				// On AI error: leave the source title (spec §7.4).
			case <-timer.C:
				// Give up waiting; the background goroutine finishes and writes
				// the row, and a later request reuses it (spec §7.3).
				stats.TimedOut = true
			}
		}
	}

	stats.AICalls = int(aiCalls.Load())

	out, err := doc.Bytes()
	if err != nil {
		return raw, stats, err
	}
	return out, stats, nil
}

// rewriteAsync starts (or joins) the singleflight AI call for one item, keyed by
// handler|id. The closure runs on a detached context (background + aiTimeout,
// same trace_id) so it outlives a request that times out.
func (rw *Rewriter) rewriteAsync(ctx context.Context, res handler.Resolution, item feed.Item, aiCalls *atomic.Int64) <-chan singleflight.Result {
	key := res.Prefix + "|" + item.ID
	return rw.group.DoChan(key, func() (any, error) {
		bg := mlog.CopyTraceID(ctx, context.Background())
		bg, cancel := context.WithTimeout(bg, rw.aiTimeout)
		defer cancel()

		if err := rw.limiter.Wait(bg); err != nil {
			return nil, err
		}

		start := time.Now()
		aiCalls.Add(1)
		title, err := rw.ai.Complete(bg, res.EffectivePrompt(), res.Handler.ComposePrompt(item))
		if err != nil {
			mlog.L(bg, rw.logger).Warn("ai.rewrite",
				zap.String("handler", res.Prefix), zap.String("item_id", item.ID),
				zap.Int64("duration_ms", time.Since(start).Milliseconds()), zap.Bool("ok", false), zap.Error(err))
			return nil, err // never cache a failed rewrite (spec §7.4)
		}

		if err := rw.cache.SaveTitle(bg, store.ItemTitle{
			ID: item.ID, Handler: res.Prefix, SourceTitle: item.Title, RewrittenTitle: title,
		}); err != nil {
			// Use the title for this request, but it stays uncached.
			mlog.L(bg, rw.logger).Error("cache save failed",
				zap.String("handler", res.Prefix), zap.String("item_id", item.ID), zap.Error(err))
			return title, nil
		}

		mlog.L(bg, rw.logger).Debug("ai.rewrite",
			zap.String("handler", res.Prefix), zap.String("item_id", item.ID),
			zap.Int64("duration_ms", time.Since(start).Milliseconds()), zap.Bool("ok", true))
		return title, nil
	})
}

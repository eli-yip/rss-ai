package rewrite_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/eli-yip/rss-ai/pkg/aiclient"
	"github.com/eli-yip/rss-ai/pkg/config"
	"github.com/eli-yip/rss-ai/pkg/feed"
	"github.com/eli-yip/rss-ai/pkg/handler"
	"github.com/eli-yip/rss-ai/pkg/mlog"
	"github.com/eli-yip/rss-ai/pkg/rewrite"
	"github.com/eli-yip/rss-ai/pkg/store"
)

// --- fake cache ---------------------------------------------------------------

type fakeCache struct {
	mu   sync.Mutex
	rows map[string]store.ItemTitle // key: handler|id
}

func newFakeCache() *fakeCache { return &fakeCache{rows: map[string]store.ItemTitle{}} }

func (c *fakeCache) key(handler, id string) string { return handler + "|" + id }

func (c *fakeCache) LookupTitles(_ context.Context, handler string, ids []string) (map[string]store.ItemTitle, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make(map[string]store.ItemTitle)
	for _, id := range ids {
		if r, ok := c.rows[c.key(handler, id)]; ok {
			out[id] = r
		}
	}
	return out, nil
}

func (c *fakeCache) SaveTitle(_ context.Context, it store.ItemTitle) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.rows[c.key(it.Handler, it.ID)] = it
	return nil
}

func (c *fakeCache) seed(it store.ItemTitle) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.rows[c.key(it.Handler, it.ID)] = it
}

func (c *fakeCache) len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.rows)
}

// --- fixtures + helpers -------------------------------------------------------

const twoItemRSS = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <item>
      <title>Original One</title>
      <guid>guid-1</guid>
      <description>Body one</description>
    </item>
    <item>
      <title>Original Two</title>
      <link>https://example.com/2</link>
      <description>Body two</description>
    </item>
  </channel>
</rss>`

const oneItemRSS = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <item>
      <title>Source Title</title>
      <guid>only-1</guid>
      <description>body</description>
    </item>
  </channel>
</rss>`

const noIDRSS = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <item>
      <title>Untouchable</title>
    </item>
  </channel>
</rss>`

func testResolution() handler.Resolution {
	return handler.NewResolver(map[string]config.HandlerConfig{
		"/test": {Enabled: true},
	}).Resolve("/test/x")
}

// titles parses out and returns the item titles in order.
func titles(t *testing.T, out []byte) []string {
	t.Helper()
	d, err := feed.Parse(out)
	require.NoError(t, err)
	var ts []string
	for _, it := range d.Items() {
		ts = append(ts, it.Title)
	}
	return ts
}

func newRewriter(c rewrite.Cache, ai aiclient.Client, wait time.Duration) *rewrite.Rewriter {
	return rewrite.New(c, ai, 10000, 5*time.Second, wait, mlog.NewNop())
}

// --- tests --------------------------------------------------------------------

func TestMissCallsAIAndWritesCache(t *testing.T) {
	cache := newFakeCache()
	ai := &aiclient.FakeClient{Reply: func(_, _ string) (string, error) { return "REWRITTEN", nil }}
	rw := newRewriter(cache, ai, 2*time.Second)

	out, stats, err := rw.RewriteFeed(context.Background(), testResolution(), []byte(twoItemRSS))
	require.NoError(t, err)
	require.Equal(t, []string{"REWRITTEN", "REWRITTEN"}, titles(t, out))
	require.Equal(t, rewrite.Stats{ItemCount: 2, CacheMisses: 2, AICalls: 2}, stats)
	require.Equal(t, 2, cache.len(), "both rewrites cached")
}

func TestCacheHitSkipsAI(t *testing.T) {
	cache := newFakeCache()
	cache.seed(store.ItemTitle{ID: "guid-1", Handler: "/test", SourceTitle: "Original One", RewrittenTitle: "Cached One"})
	cache.seed(store.ItemTitle{ID: "https://example.com/2", Handler: "/test", SourceTitle: "Original Two", RewrittenTitle: "Cached Two"})
	ai := &aiclient.FakeClient{Reply: func(_, _ string) (string, error) { return "SHOULD NOT RUN", nil }}
	rw := newRewriter(cache, ai, 2*time.Second)

	out, stats, err := rw.RewriteFeed(context.Background(), testResolution(), []byte(twoItemRSS))
	require.NoError(t, err)
	require.Equal(t, []string{"Cached One", "Cached Two"}, titles(t, out))
	require.Equal(t, rewrite.Stats{ItemCount: 2, CacheHits: 2}, stats)
	require.Equal(t, 0, ai.Calls(), "no AI call on a clean hit")
}

func TestSourceTitleChangedTriggersRewrite(t *testing.T) {
	cache := newFakeCache()
	// Cached under a stale source title -> must re-rewrite.
	cache.seed(store.ItemTitle{ID: "only-1", Handler: "/test", SourceTitle: "Old Source", RewrittenTitle: "Stale"})
	ai := &aiclient.FakeClient{Reply: func(_, _ string) (string, error) { return "Fresh", nil }}
	rw := newRewriter(cache, ai, 2*time.Second)

	out, stats, err := rw.RewriteFeed(context.Background(), testResolution(), []byte(oneItemRSS))
	require.NoError(t, err)
	require.Equal(t, []string{"Fresh"}, titles(t, out))
	require.Equal(t, 1, stats.CacheMisses)
	require.Equal(t, 1, ai.Calls())

	got, _ := cache.LookupTitles(context.Background(), "/test", []string{"only-1"})
	require.Equal(t, "Source Title", got["only-1"].SourceTitle, "row updated to new source")
	require.Equal(t, "Fresh", got["only-1"].RewrittenTitle)
}

func TestSingleflightDedupsConcurrentRequests(t *testing.T) {
	cache := newFakeCache()
	ai := &aiclient.FakeClient{
		Block: make(chan struct{}),
		Reply: func(_, _ string) (string, error) { return "ONCE", nil },
	}
	rw := newRewriter(cache, ai, 2*time.Second)

	type result struct {
		out []byte
	}
	results := make(chan result, 2)
	run := func() {
		out, _, err := rw.RewriteFeed(context.Background(), testResolution(), []byte(oneItemRSS))
		require.NoError(t, err)
		results <- result{out}
	}

	go run()
	// Wait until the first request's AI call is in flight (singleflight key held).
	require.Eventually(t, func() bool { return ai.Calls() == 1 }, time.Second, time.Millisecond)
	go run() // joins the in-flight call instead of starting a second

	// Give the second goroutine a moment to attach, then release.
	time.Sleep(20 * time.Millisecond)
	close(ai.Block)

	for range 2 {
		r := <-results
		require.Equal(t, []string{"ONCE"}, titles(t, r.out))
	}
	require.Equal(t, 1, ai.Calls(), "AI called once despite two concurrent requests")
}

func TestTimeoutFallsBackThenBackgroundWrites(t *testing.T) {
	cache := newFakeCache()
	ai := &aiclient.FakeClient{
		Block: make(chan struct{}),
		Reply: func(_, _ string) (string, error) { return "Background Title", nil },
	}
	// Tiny wait, large aiTimeout: the request gives up but the call keeps running.
	rw := rewrite.New(cache, ai, 10000, 5*time.Second, 20*time.Millisecond, mlog.NewNop())

	out, stats, err := rw.RewriteFeed(context.Background(), testResolution(), []byte(oneItemRSS))
	require.NoError(t, err)
	require.True(t, stats.TimedOut)
	require.Equal(t, []string{"Source Title"}, titles(t, out), "fell back to the source title")

	// Release the AI call; the detached goroutine finishes and writes the row.
	close(ai.Block)
	require.Eventually(t, func() bool { return cache.len() == 1 }, time.Second, time.Millisecond)

	// A subsequent request is now a clean cache hit, no new AI call.
	out2, stats2, err := rw.RewriteFeed(context.Background(), testResolution(), []byte(oneItemRSS))
	require.NoError(t, err)
	require.Equal(t, []string{"Background Title"}, titles(t, out2))
	require.Equal(t, 1, stats2.CacheHits)
	require.Equal(t, 1, ai.Calls(), "background call was the only AI call")
}

func TestAIFailureKeepsSourceAndDoesNotCache(t *testing.T) {
	cache := newFakeCache()
	ai := &aiclient.FakeClient{Reply: func(_, _ string) (string, error) { return "", errors.New("boom") }}
	rw := newRewriter(cache, ai, 2*time.Second)

	out, stats, err := rw.RewriteFeed(context.Background(), testResolution(), []byte(oneItemRSS))
	require.NoError(t, err)
	require.Equal(t, []string{"Source Title"}, titles(t, out))
	require.Equal(t, 1, stats.AICalls)
	require.Equal(t, 0, cache.len(), "failed rewrite is never cached")
}

func TestItemWithoutIDIsLeftUntouched(t *testing.T) {
	cache := newFakeCache()
	ai := &aiclient.FakeClient{Reply: func(_, _ string) (string, error) { return "NOPE", nil }}
	rw := newRewriter(cache, ai, 2*time.Second)

	out, stats, err := rw.RewriteFeed(context.Background(), testResolution(), []byte(noIDRSS))
	require.NoError(t, err)
	require.Equal(t, []string{"Untouchable"}, titles(t, out))
	require.Equal(t, rewrite.Stats{ItemCount: 1}, stats, "no hit, no miss, no AI call")
	require.Equal(t, 0, ai.Calls())
}

func TestParseErrorReturnsRawUnchanged(t *testing.T) {
	cache := newFakeCache()
	ai := &aiclient.FakeClient{}
	rw := newRewriter(cache, ai, 2*time.Second)

	raw := []byte("not xml at all <<<")
	out, _, err := rw.RewriteFeed(context.Background(), testResolution(), raw)
	require.Error(t, err)
	require.Equal(t, raw, out)
}

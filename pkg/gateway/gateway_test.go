package gateway_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"

	"github.com/eli-yip/rss-ai/pkg/aiclient"
	"github.com/eli-yip/rss-ai/pkg/config"
	"github.com/eli-yip/rss-ai/pkg/feed"
	"github.com/eli-yip/rss-ai/pkg/gateway"
	"github.com/eli-yip/rss-ai/pkg/handler"
	"github.com/eli-yip/rss-ai/pkg/mlog"
	"github.com/eli-yip/rss-ai/pkg/proxy"
	"github.com/eli-yip/rss-ai/pkg/rewrite"
	"github.com/eli-yip/rss-ai/pkg/store"
	"github.com/eli-yip/rss-ai/pkg/upstream"
)

// Passthrough branches (unregistered, ?format=json, ?format=atom) reach upstream
// unchanged. The rewrite pipeline is nil here because none of these branches
// touch it.
func TestHandleReachesUpstream(t *testing.T) {
	var gotPath, gotQuery string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		_, _ = w.Write([]byte("ok"))
	}))
	defer up.Close()

	px, err := proxy.New(up.URL, mlog.NewNop())
	require.NoError(t, err)
	g := gateway.New(
		handler.NewResolver(map[string]config.HandlerConfig{"/github": {Enabled: true}}),
		px, nil, nil, mlog.NewNop(),
	)

	cases := []struct{ name, target, wantPath, wantQuery string }{
		{"unmatched passthrough", "/twitter/u?n=1", "/twitter/u", "n=1"},
		{"json passthrough", "/github/issue/1?format=json", "/github/issue/1", "format=json"},
		{"atom passthrough", "/github/issue/1?format=atom", "/github/issue/1", "format=atom"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.target, nil)
			rec := httptest.NewRecorder()
			c := echo.New().NewContext(req, rec)

			require.NoError(t, g.Handle(c))
			require.Equal(t, http.StatusOK, rec.Result().StatusCode)
			require.Equal(t, tc.wantPath, gotPath)
			require.Equal(t, tc.wantQuery, gotQuery)
		})
	}
}

const handledRSS = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <item>
      <title>Source A</title>
      <guid>a</guid>
      <description>body a</description>
    </item>
  </channel>
</rss>`

// memCache is a thread-safe in-memory rewrite.Cache: the rewriter writes
// concurrently (one goroutine per item), so a bare map would race.
type memCache struct {
	mu   sync.Mutex
	rows map[string]store.ItemTitle
}

func (m *memCache) LookupTitles(_ context.Context, h string, ids []string) (map[string]store.ItemTitle, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := map[string]store.ItemTitle{}
	for _, id := range ids {
		if r, ok := m.rows[h+"|"+id]; ok {
			out[id] = r
		}
	}
	return out, nil
}

func (m *memCache) SaveTitle(_ context.Context, it store.ItemTitle) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rows[it.Handler+"|"+it.ID] = it
	return nil
}

// handledGateway builds a gateway whose handled branch fetches from up and
// rewrites titles with a deterministic fake AI.
func handledGateway(t *testing.T, up string) *gateway.Gateway {
	t.Helper()
	fetcher, err := upstream.New(up, 5*time.Second)
	require.NoError(t, err)
	ai := &aiclient.FakeClient{Reply: func(_, _ string) (string, error) { return "AI Title", nil }}
	rw := rewrite.New(&memCache{rows: map[string]store.ItemTitle{}}, ai, 10000, 5*time.Second, 2*time.Second, mlog.NewNop())
	px, err := proxy.New(up, mlog.NewNop())
	require.NoError(t, err)
	return gateway.New(
		handler.NewResolver(map[string]config.HandlerConfig{"/test": {Enabled: true}}),
		px, fetcher, rw, mlog.NewNop(),
	)
}

func TestHandledBranchRewritesTitle(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		_, _ = io.WriteString(w, handledRSS)
	}))
	defer up.Close()

	g := handledGateway(t, up.URL)

	req := httptest.NewRequest(http.MethodGet, "/test/1", nil)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	require.NoError(t, g.Handle(c))

	require.Equal(t, http.StatusOK, rec.Result().StatusCode)
	d, err := feed.Parse(rec.Body.Bytes())
	require.NoError(t, err)
	require.Equal(t, "AI Title", d.Items()[0].Title, "handled branch rewrote the title")
}

func TestHandledBranchPassesNon2xxThrough(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, "missing")
	}))
	defer up.Close()

	g := handledGateway(t, up.URL)

	req := httptest.NewRequest(http.MethodGet, "/test/1", nil)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	require.NoError(t, g.Handle(c))

	require.Equal(t, http.StatusNotFound, rec.Result().StatusCode)
	require.Equal(t, "missing", rec.Body.String())
}

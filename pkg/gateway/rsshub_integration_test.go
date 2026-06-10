package gateway_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
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

// These tests run the gateway against a real RSSHub instance (see
// compose.test.yaml / `just integration`). They are gated on RSS_AI_TEST_UPSTREAM
// and skip when it is unset or the upstream is unreachable, so `go test ./...`
// stays green without Docker.
//
// In plan-1 every branch — handled, explicit ?format=json, and unmatched — is a
// transparent reverse proxy. The contract these tests pin down is therefore:
// proxying through the gateway returns byte-for-byte what RSSHub returns for the
// same URL. RSSHub caches rendered feeds (CACHE_EXPIRE), so the direct probe and
// the proxied fetch of the same path yield identical bodies despite baked-in
// timestamps.

func upstreamBase(t *testing.T) string {
	t.Helper()
	base := os.Getenv("RSS_AI_TEST_UPSTREAM")
	if base == "" {
		t.Skip("RSS_AI_TEST_UPSTREAM not set; skipping RSSHub integration test")
	}

	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(base + "/test/1")
	if err != nil || resp.StatusCode != http.StatusOK {
		if resp != nil {
			_ = resp.Body.Close()
		}
		t.Skipf("RSSHub upstream %s not reachable (%v); skipping", base, err)
	}
	_ = resp.Body.Close()
	return base
}

type httpResult struct {
	status      int
	contentType string
	body        []byte
}

func fetch(t *testing.T, base, target string) httpResult {
	t.Helper()
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Get(base + target)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return httpResult{resp.StatusCode, resp.Header.Get("Content-Type"), body}
}

// rewrittenTitle is the constant the integration fake AI returns, so the only
// expected difference between the direct and handled feeds is every <title>.
const rewrittenTitle = "REWRITTEN BY TEST AI"

// gatewayFor stands up an httptest server fronting a gateway whose resolver is
// built from handlers, with the handled branch fetching from the real upstream
// and rewriting titles via a deterministic fake AI. Returns its base URL.
func gatewayFor(t *testing.T, upstreamBase string, handlers map[string]config.HandlerConfig) string {
	t.Helper()
	px, err := proxy.New(upstreamBase, mlog.NewNop())
	require.NoError(t, err)
	fetcher, err := upstream.New(upstreamBase, 10*time.Second)
	require.NoError(t, err)
	ai := &aiclient.FakeClient{Reply: func(_, _ string) (string, error) { return rewrittenTitle, nil }}
	rw := rewrite.New(&memCache{rows: map[string]store.ItemTitle{}}, ai, 10000, 5*time.Second, 5*time.Second, mlog.NewNop())
	gw := gateway.New(handler.NewResolver(handlers), px, fetcher, rw, mlog.NewNop())

	e := echo.New()
	e.Any("/*", gw.Handle)
	srv := httptest.NewServer(e)
	t.Cleanup(srv.Close)
	return srv.URL
}

// TestRSSHubHandledOnlyTitlesChange is the strongest fidelity assertion: the
// handled feed equals the direct feed round-tripped through etree with every
// item title replaced — i.e. nothing but the titles moved.
func TestRSSHubHandledOnlyTitlesChange(t *testing.T) {
	up := upstreamBase(t)
	gw := gatewayFor(t, up, map[string]config.HandlerConfig{"/test": {Enabled: true}})

	direct := fetch(t, up, "/test/1")
	through := fetch(t, gw, "/test/1")
	require.Equal(t, http.StatusOK, through.status)

	// Expected = parse the direct feed, set every title to the AI constant,
	// serialize. The gateway does exactly this over the same upstream bytes.
	d, err := feed.Parse(direct.body)
	require.NoError(t, err)
	require.NotEmpty(t, d.Items(), "RSSHub /test/1 should have items to rewrite")
	for i := range d.Items() {
		d.SetTitle(i, rewrittenTitle)
	}
	expected, err := d.Bytes()
	require.NoError(t, err)

	require.Equal(t, string(expected), string(through.body),
		"handled feed differs from upstream only in item titles")
}

func TestRSSHubPassthroughTransparency(t *testing.T) {
	up := upstreamBase(t)

	handledGW := gatewayFor(t, up, map[string]config.HandlerConfig{"/test": {Enabled: true}})
	passthroughGW := gatewayFor(t, up, nil) // nothing enabled -> everything passes through

	cases := []struct {
		name   string
		gw     string
		target string
	}{
		{"atom passthrough", handledGW, "/test/1?format=atom"},
		{"json passthrough", handledGW, "/test/1?format=json"},
		{"unmatched passthrough", passthroughGW, "/test/1"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			direct := fetch(t, up, tc.target)
			through := fetch(t, tc.gw, tc.target)

			require.Equal(t, http.StatusOK, through.status)
			require.Equal(t, direct.status, through.status)
			require.Equal(t, direct.contentType, through.contentType)
			require.Equal(t, string(direct.body), string(through.body),
				"passthrough must return upstream bytes unchanged")
		})
	}
}

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

	"github.com/eli-yip/rss-ai/pkg/config"
	"github.com/eli-yip/rss-ai/pkg/gateway"
	"github.com/eli-yip/rss-ai/pkg/handler"
	"github.com/eli-yip/rss-ai/pkg/mlog"
	"github.com/eli-yip/rss-ai/pkg/proxy"
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

// gatewayFor stands up an httptest server fronting a gateway whose resolver is
// built from handlers, proxying to the real upstream. Returns its base URL.
func gatewayFor(t *testing.T, upstream string, handlers map[string]config.HandlerConfig) string {
	t.Helper()
	px, err := proxy.New(upstream, mlog.NewNop())
	require.NoError(t, err)
	gw := gateway.New(handler.NewResolver(handlers), px, mlog.NewNop())

	e := echo.New()
	e.Any("/*", gw.Handle)
	srv := httptest.NewServer(e)
	t.Cleanup(srv.Close)
	return srv.URL
}

func TestRSSHubPassthroughTransparency(t *testing.T) {
	upstream := upstreamBase(t)

	handledGW := gatewayFor(t, upstream, map[string]config.HandlerConfig{"/test": {Enabled: true}})
	passthroughGW := gatewayFor(t, upstream, nil) // nothing enabled -> everything passes through

	cases := []struct {
		name   string
		gw     string
		target string
	}{
		{"handled default RSS", handledGW, "/test/1"},
		{"handled atom", handledGW, "/test/1?format=atom"},
		{"handled json passthrough", handledGW, "/test/1?format=json"},
		{"unmatched passthrough", passthroughGW, "/test/1"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			direct := fetch(t, upstream, tc.target)
			through := fetch(t, tc.gw, tc.target)

			require.Equal(t, http.StatusOK, through.status)
			require.Equal(t, direct.status, through.status)
			require.Equal(t, direct.contentType, through.contentType)
			require.Equal(t, string(direct.body), string(through.body),
				"gateway must return upstream bytes unchanged in plan-1")
		})
	}
}

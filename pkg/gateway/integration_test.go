package gateway_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/eli-yip/rss-ai/pkg/gateway"
	"github.com/eli-yip/rss-ai/pkg/handler"
	"github.com/eli-yip/rss-ai/pkg/mlog"
	"github.com/eli-yip/rss-ai/pkg/proxy"
	"github.com/eli-yip/rss-ai/pkg/server"
)

type okPinger struct{}

func (okPinger) Ping(context.Context) error { return nil }

// Mounting the catch-all must not shadow the static health routes: Echo gives
// static paths precedence over the "/*" wildcard.
func TestCatchAllRoutePrecedence(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("from-upstream"))
	}))
	defer upstream.Close()

	px, err := proxy.New(upstream.URL, mlog.NewNop())
	require.NoError(t, err)
	gw := gateway.New(handler.NewResolver(nil), px, mlog.NewNop())

	srv := server.New(":0", okPinger{}, mlog.NewNop())
	srv.Echo().Any("/*", gw.Handle)

	// /healthz still resolves to the health handler, not the gateway.
	rec := httptest.NewRecorder()
	srv.Echo().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	require.NotContains(t, rec.Body.String(), "from-upstream")

	// an arbitrary path falls through to the gateway -> upstream.
	rec = httptest.NewRecorder()
	srv.Echo().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/twitter/user", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "from-upstream", rec.Body.String())
}

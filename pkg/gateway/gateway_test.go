package gateway_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"

	"github.com/eli-yip/rss-ai/pkg/config"
	"github.com/eli-yip/rss-ai/pkg/gateway"
	"github.com/eli-yip/rss-ai/pkg/handler"
	"github.com/eli-yip/rss-ai/pkg/mlog"
	"github.com/eli-yip/rss-ai/pkg/proxy"
)

// In plan-1 both passthrough and the handled seam reach upstream unchanged; this
// black-box test confirms Handle wires resolve -> proxy for every branch.
func TestHandleReachesUpstream(t *testing.T) {
	var gotPath, gotQuery string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		_, _ = w.Write([]byte("ok"))
	}))
	defer upstream.Close()

	px, err := proxy.New(upstream.URL, mlog.NewNop())
	require.NoError(t, err)
	g := gateway.New(
		handler.NewResolver(map[string]config.HandlerConfig{"/github": {Enabled: true}}),
		px, mlog.NewNop(),
	)

	cases := []struct{ name, target, wantPath, wantQuery string }{
		{"passthrough", "/twitter/u?n=1", "/twitter/u", "n=1"},
		{"json", "/github/issue/1?format=json", "/github/issue/1", "format=json"},
		{"handled", "/github/issue/1?format=atom", "/github/issue/1", "format=atom"},
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

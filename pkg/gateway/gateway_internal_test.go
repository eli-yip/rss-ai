package gateway

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"

	"github.com/eli-yip/rss-ai/pkg/config"
	"github.com/eli-yip/rss-ai/pkg/handler"
	"github.com/eli-yip/rss-ai/pkg/mlog"
)

func newTestGateway(t *testing.T, handlers map[string]config.HandlerConfig) *Gateway {
	t.Helper()
	// decide() touches neither proxy, fetcher, nor rewriter.
	return New(handler.NewResolver(handlers), nil, nil, nil, mlog.NewNop())
}

func decideFor(t *testing.T, g *Gateway, target string) (handler.Resolution, mode) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	c := echo.New().NewContext(req, httptest.NewRecorder())
	return g.decide(c)
}

func TestDecideUnmatchedIsPassthrough(t *testing.T) {
	g := newTestGateway(t, map[string]config.HandlerConfig{"/github": {Enabled: true}})

	res, m := decideFor(t, g, "/twitter/user")
	require.False(t, res.Matched)
	require.Equal(t, modePassthrough, m)
}

func TestDecideJSONIsPassthrough(t *testing.T) {
	g := newTestGateway(t, map[string]config.HandlerConfig{"/github": {Enabled: true}})

	res, m := decideFor(t, g, "/github/issue/1?format=json")
	require.True(t, res.Matched) // prefix is enabled...
	require.Equal(t, modePassthrough, m) // ...but explicit JSON is never modified
}

func TestDecideAtomIsPassthrough(t *testing.T) {
	g := newTestGateway(t, map[string]config.HandlerConfig{"/github": {Enabled: true}})

	// plan-2: RSS 2.0 only, so atom passes through alongside json.
	res, m := decideFor(t, g, "/github/issue/1?format=atom")
	require.True(t, res.Matched)
	require.Equal(t, modePassthrough, m)
}

func TestDecideDefaultRSSIsHandled(t *testing.T) {
	g := newTestGateway(t, map[string]config.HandlerConfig{"/github": {Enabled: true}})

	res, m := decideFor(t, g, "/github/issue/1") // default RSS 2.0
	require.True(t, res.Matched)
	require.Equal(t, modeHandled, m)
}

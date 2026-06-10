package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"

	"github.com/eli-yip/rss-ai/pkg/ctxutil"
	"github.com/eli-yip/rss-ai/pkg/mlog"
)

func TestTraceIDMiddlewareSetsContext(t *testing.T) {
	e := echo.New()
	var seen bool
	h := traceIDMiddleware()(func(c *echo.Context) error {
		_, seen = ctxutil.ValueFrom[uint64](c.Request().Context(), mlog.TraceIDCtxKey)
		return c.NoContent(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	require.NoError(t, h(c))
	require.True(t, seen, "trace id should be present in request context")
}

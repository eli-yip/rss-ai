package proxy_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"

	"github.com/eli-yip/rss-ai/pkg/mlog"
	"github.com/eli-yip/rss-ai/pkg/proxy"
)

func TestPassthroughFidelity(t *testing.T) {
	var gotPath, gotQuery, gotHeader, gotMethod string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		gotHeader = r.Header.Get("X-Test")
		w.Header().Set("X-Upstream", "yes")
		w.WriteHeader(http.StatusTeapot)
		_, _ = w.Write([]byte("upstream-body"))
	}))
	defer upstream.Close()

	p, err := proxy.New(upstream.URL, mlog.NewNop())
	require.NoError(t, err)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/twitter/user?format=atom&n=2", nil)
	req.Header.Set("X-Test", "abc")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	require.NoError(t, p.Passthrough(c))

	// upstream saw the original request unchanged.
	require.Equal(t, http.MethodGet, gotMethod)
	require.Equal(t, "/twitter/user", gotPath)
	require.Equal(t, "format=atom&n=2", gotQuery)
	require.Equal(t, "abc", gotHeader)

	// client got the upstream response unchanged.
	res := rec.Result()
	defer res.Body.Close()
	require.Equal(t, http.StatusTeapot, res.StatusCode)
	require.Equal(t, "yes", res.Header.Get("X-Upstream"))
	body, _ := io.ReadAll(res.Body)
	require.Equal(t, "upstream-body", string(body))
}

func TestPassthroughUpstreamDown(t *testing.T) {
	// A reserved-for-documentation address that refuses connections quickly.
	p, err := proxy.New("http://127.0.0.1:1", mlog.NewNop())
	require.NoError(t, err)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	require.NoError(t, p.Passthrough(c))
	require.Equal(t, http.StatusBadGateway, rec.Result().StatusCode)
}

func TestNewRejectsBadURL(t *testing.T) {
	_, err := proxy.New("://not a url", mlog.NewNop())
	require.Error(t, err)
}

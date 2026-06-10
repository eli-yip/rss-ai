package server_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/eli-yip/rss-ai/pkg/mlog"
	"github.com/eli-yip/rss-ai/pkg/server"
)

type fakePinger struct{ err error }

func (f fakePinger) Ping(context.Context) error { return f.err }

func TestHealthz(t *testing.T) {
	srv := server.New(":0", fakePinger{}, mlog.NewNop())

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	srv.Echo().ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
}

func TestReadyzOK(t *testing.T) {
	srv := server.New(":0", fakePinger{err: nil}, mlog.NewNop())

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()
	srv.Echo().ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
}

func TestReadyzDown(t *testing.T) {
	srv := server.New(":0", fakePinger{err: errors.New("db down")}, mlog.NewNop())

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()
	srv.Echo().ServeHTTP(rec, req)

	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
}

package upstream_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/eli-yip/rss-ai/pkg/upstream"
)

func TestFetchPreservesPathQueryAndBody(t *testing.T) {
	var gotPath, gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("<rss/>"))
	}))
	defer srv.Close()

	f, err := upstream.New(srv.URL, 5*time.Second)
	require.NoError(t, err)

	resp, err := f.Fetch(context.Background(), "/test/1", "format=rss&limit=2")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.Status)
	require.Equal(t, "application/xml", resp.ContentType)
	require.Equal(t, "<rss/>", string(resp.Body))
	require.Equal(t, "/test/1", gotPath)
	require.Equal(t, "format=rss&limit=2", gotQuery)
}

func TestFetchReturnsNon2xxAsData(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("nope"))
	}))
	defer srv.Close()

	f, err := upstream.New(srv.URL, 5*time.Second)
	require.NoError(t, err)

	resp, err := f.Fetch(context.Background(), "/missing", "")
	require.NoError(t, err, "non-2xx is data, not an error")
	require.Equal(t, http.StatusNotFound, resp.Status)
	require.Equal(t, "nope", string(resp.Body))
}

func TestFetchTransportErrorReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	base := srv.URL
	srv.Close() // now unreachable

	f, err := upstream.New(base, time.Second)
	require.NoError(t, err)

	_, err = f.Fetch(context.Background(), "/x", "")
	require.Error(t, err)
}

func TestNewRejectsBadBaseURL(t *testing.T) {
	_, err := upstream.New("not-a-url", time.Second)
	require.Error(t, err)
}

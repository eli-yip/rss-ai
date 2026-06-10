package httputil_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/eli-yip/rss-ai/pkg/httputil"
)

func TestNewResp(t *testing.T) {
	r := httputil.NewResp("ok", 42)
	require.Equal(t, "ok", r.Message)
	require.Equal(t, 42, r.Data)
}

func TestNewMessage(t *testing.T) {
	m := httputil.NewMessage("hi")
	require.Equal(t, "hi", m.Message)
}

func TestNewHTTPError(t *testing.T) {
	err := httputil.NewHTTPError(404, "not found")
	require.Equal(t, 404, err.Code)
	require.Equal(t, "not found", err.Error())
}

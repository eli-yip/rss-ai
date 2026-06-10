package handler

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/eli-yip/rss-ai/pkg/feed"
)

type stubHandler struct{ prompt string }

func (s stubHandler) Prompt() string                      { return s.prompt }
func (s stubHandler) ComposePrompt(item feed.Item) string { return item.Title }

func TestRegistryRegisterAndLookup(t *testing.T) {
	h := stubHandler{prompt: "p"}
	Register("/__test_a", h)

	got, ok := lookup("/__test_a")
	require.True(t, ok)
	require.Equal(t, h, got)

	_, ok = lookup("/__test_missing")
	require.False(t, ok)
}

func TestRegistryRejectsEmptyPrefix(t *testing.T) {
	require.Panics(t, func() { Register("", stubHandler{}) })
}

func TestRegistryRejectsDuplicate(t *testing.T) {
	Register("/__test_dup", stubHandler{})
	require.Panics(t, func() { Register("/__test_dup", stubHandler{}) })
}

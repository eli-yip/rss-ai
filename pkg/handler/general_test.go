package handler

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/eli-yip/rss-ai/pkg/feed"
)

func TestGeneralHandlerPrompt(t *testing.T) {
	h := generalHandler{}
	require.NotEmpty(t, h.Prompt())
	require.Equal(t, defaultPrompt, h.Prompt())
}

func TestGeneralHandlerComposePrompt(t *testing.T) {
	h := generalHandler{}

	withBody := h.ComposePrompt(feed.Item{Title: "T", Body: "B"})
	require.Contains(t, withBody, "T")
	require.Contains(t, withBody, "B")

	require.Equal(t, "Just title", h.ComposePrompt(feed.Item{Title: "Just title"}))
}

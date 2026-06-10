package telegram

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/require"

	"github.com/eli-yip/rss-ai/pkg/feed"
	"github.com/eli-yip/rss-ai/pkg/handler"
)

// compile-time proof the type satisfies the seam.
var _ handler.Handler = Handler{}

func TestPromptIsTelegramSpecific(t *testing.T) {
	p := Handler{}.Prompt()
	require.NotEmpty(t, p)
	require.Contains(t, strings.ToLower(p), "telegram")
}

func TestComposePromptStripsHTMLAndKeepsBody(t *testing.T) {
	h := Handler{}
	out := h.ComposePrompt(feed.Item{
		Title: "Big news today: the",
		Body:  `Big news today: the project shipped.<br>Read <a href="https://x">more</a>`,
	})

	require.Contains(t, out, "Big news today: the project shipped.")
	require.Contains(t, out, "Read more")
	require.NotContains(t, out, "<") // no raw HTML tags survive
}

func TestComposePromptSuppressesTitleHintWhenPrefixOfBody(t *testing.T) {
	h := Handler{}
	// Title is the truncated head of the body — RSSHub's synthetic title. It
	// adds nothing, so it must not be echoed back as a hint.
	out := h.ComposePrompt(feed.Item{
		Title: "Big news today",
		Body:  "Big news today the project shipped and everyone cheered",
	})
	require.NotContains(t, strings.ToLower(out), "hint")
}

func TestComposePromptIncludesTitleHintWhenDistinct(t *testing.T) {
	h := Handler{}
	out := h.ComposePrompt(feed.Item{
		Title: "Channel weekly digest",
		Body:  "Completely unrelated body content about a shipped project",
	})
	require.Contains(t, strings.ToLower(out), "hint")
	require.Contains(t, out, "Channel weekly digest")
}

func TestComposePromptEmptyBodyFallsBackToTitle(t *testing.T) {
	h := Handler{}
	require.Equal(t, "Just a title", h.ComposePrompt(feed.Item{Title: "Just a title"}))
	// Body that is only markup also counts as empty after flattening.
	require.Equal(t, "Only markup", h.ComposePrompt(feed.Item{Title: "Only markup", Body: "<br><br>"}))
}

func TestComposePromptTruncatesLongBody(t *testing.T) {
	h := Handler{}
	long := strings.Repeat("好", maxBodyChars+500) // multi-byte to prove rune-safe cut
	out := h.ComposePrompt(feed.Item{Title: "x", Body: long})

	require.True(t, utf8.ValidString(out))
	bodyRunes := utf8.RuneCountInString(out)
	require.LessOrEqual(t, bodyRunes, maxBodyChars+200) // body capped; small prompt scaffold allowed
	require.Contains(t, out, ellipsis)
}

// Package telegram is the specialized handler (spec §4) for RSSHub's
// /telegram/channel route. Those items have no real title — RSSHub derives
// <title> by truncating the message, while <description> holds the full message
// HTML. So this handler treats the body as the source of truth: it flattens the
// HTML to text, bounds its length, and feeds it body-first, demoting the
// synthetic title to a hint (dropped when it is just a prefix of the body).
//
// It self-registers under "/telegram/channel" from init(); cmd/rss-ai blank
// imports the package so registration runs before the resolver is built. Output
// is still only the rewritten title — the rewrite/cache machinery is unchanged.
package telegram

import (
	"strings"
	"unicode/utf8"

	"github.com/eli-yip/rss-ai/pkg/feed"
	"github.com/eli-yip/rss-ai/pkg/handler"
)

// Prefix is the route this handler specializes. It is both the registry key and
// the config key (spec §4 same-key rule requires them to match).
const Prefix = "/telegram/channel"

// maxBodyChars bounds the flattened body sent to the model (long-form channel
// posts can be huge); ellipsis marks a truncated body.
const (
	maxBodyChars = 4000
	ellipsis     = "…"
)

const telegramPrompt = "These are Telegram channel messages. The title is " +
	"auto-generated and may be truncated or missing — write a concise, " +
	"informative title (roughly 80 characters or fewer) that captures the " +
	"message's point, in the message's own language. Strip forwarding lines, " +
	"emoji, and promotional noise. Output only the title, with no quotes or markdown."

func init() { handler.Register(Prefix, Handler{}) }

// Handler implements handler.Handler for Telegram channel items.
type Handler struct{}

// New returns the handler value; handy for explicit wiring or tests.
func New() Handler { return Handler{} }

// Prompt is the Telegram-specific system prompt (overridable via config, spec §4).
func (Handler) Prompt() string { return telegramPrompt }

// ComposePrompt builds the user message body-first. With no usable body it falls
// back to the title alone (mirroring the general handler), so the model always
// has something to retitle.
func (Handler) ComposePrompt(item feed.Item) string {
	body := truncateRunes(htmlToText(item.Body), maxBodyChars)
	if body == "" {
		return item.Title
	}

	var b strings.Builder
	b.WriteString("Telegram message:\n")
	b.WriteString(body)
	// The synthetic title only helps when it is not just a slice of the body.
	if hint := strings.TrimSpace(item.Title); hint != "" && !titleIsPrefixOfBody(hint, body) {
		b.WriteString("\n\n(auto-generated title hint: ")
		b.WriteString(hint)
		b.WriteString(")")
	}
	return b.String()
}

// truncateRunes caps s at n runes (never splitting a rune) and appends an
// ellipsis when it had to cut.
func truncateRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	i, count := 0, 0
	for i = range s {
		if count == n {
			return s[:i] + ellipsis
		}
		count++
	}
	return s + ellipsis
}

// titleIsPrefixOfBody reports whether the title is RSSHub's truncated head of
// the body, comparing case- and space-insensitively so minor flattening
// differences still match.
func titleIsPrefixOfBody(title, body string) bool {
	norm := func(s string) string { return strings.ToLower(strings.Join(strings.Fields(s), " ")) }
	return strings.HasPrefix(norm(body), norm(title))
}

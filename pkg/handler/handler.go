// Package handler implements the three-layer handler resolution of the gateway
// spec (§4): config decides which prefixes are enabled, a code-level registry
// holds specialized per-prefix handlers, and a general handler is the fallback.
package handler

import "github.com/eli-yip/rss-ai/pkg/feed"

// Handler is specialized per-prefix logic. The config prompt overrides the
// built-in Prompt (spec §4); ComposePrompt builds the AI user content for an
// item so the model reads the full body before retitling (spec §5.2/§8).
type Handler interface {
	// Prompt returns the handler's built-in default prompt (the system prompt,
	// unless config overrides it via Resolution.EffectivePrompt).
	Prompt() string

	// ComposePrompt builds the AI user message for one item. The general handler
	// appends the item body to the title; specialized handlers may reshape it.
	ComposePrompt(item feed.Item) string
}

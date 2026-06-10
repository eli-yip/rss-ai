package handler

import "github.com/eli-yip/rss-ai/pkg/feed"

// defaultPrompt is the built-in rewrite instruction used when neither config nor
// a specialized handler supplies one (spec §4 prompt priority).
const defaultPrompt = "Rewrite this feed item title to be clear and readable, preserving meaning."

// generalHandler is the fallback used when a prefix is enabled but has no
// specialized handler registered. It is not registered itself; the resolver
// holds a single instance.
type generalHandler struct{}

func (generalHandler) Prompt() string { return defaultPrompt }

// ComposePrompt feeds the model the source title plus the item body so it can
// read the full content before retitling (spec §5.2/§8). With no body it sends
// just the title.
func (generalHandler) ComposePrompt(item feed.Item) string {
	if item.Body == "" {
		return item.Title
	}
	return "Title: " + item.Title + "\n\nContent:\n" + item.Body
}

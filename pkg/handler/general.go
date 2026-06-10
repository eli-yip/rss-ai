package handler

// defaultPrompt is the built-in rewrite instruction used when neither config nor
// a specialized handler supplies one (spec §4 prompt priority).
const defaultPrompt = "Rewrite this feed item title to be clear and readable, preserving meaning."

// generalHandler is the fallback used when a prefix is enabled but has no
// specialized handler registered. It is not registered itself; the resolver
// holds a single instance.
type generalHandler struct{}

func (generalHandler) Prompt() string { return defaultPrompt }

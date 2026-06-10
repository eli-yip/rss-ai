// Package handler implements the three-layer handler resolution of the gateway
// spec (§4): config decides which prefixes are enabled, a code-level registry
// holds specialized per-prefix handlers, and a general handler is the fallback.
package handler

// Handler is specialized per-prefix logic. plan-1 only needs the resolution
// seam, so the interface exposes the built-in default prompt; the config prompt
// overrides it (spec §4). plan-2 adds the title-rewrite method.
type Handler interface {
	// Prompt returns the handler's built-in default prompt.
	Prompt() string
}

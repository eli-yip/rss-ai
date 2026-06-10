package handler

import (
	"sort"
	"strings"

	"github.com/eli-yip/rss-ai/pkg/config"
)

// Resolution is the outcome of matching a request path against the enabled
// prefixes and the registry.
type Resolution struct {
	Matched     bool                 // matched an enabled prefix
	Prefix      string               // the matched prefix key ("" if none)
	Specialized bool                 // a registry handler exists for Prefix
	Handler     Handler              // specialized if Specialized, else general
	Config      config.HandlerConfig // the matched prefix's config entry
}

// EffectivePrompt applies the spec §4 priority: a non-empty config prompt
// overrides the handler's built-in default.
func (r Resolution) EffectivePrompt() string {
	if r.Config.Prompt != "" {
		return r.Config.Prompt
	}
	if r.Handler != nil {
		return r.Handler.Prompt()
	}
	return ""
}

// Resolver matches request paths against the enabled prefixes from config and
// selects the specialized (registry) or general handler.
type Resolver struct {
	prefixes []string // enabled prefixes, sorted longest-first
	configs  map[string]config.HandlerConfig
	general  Handler
}

// NewResolver captures the enabled prefixes from config (Enabled==true only),
// sorted so the longest match wins.
func NewResolver(handlers map[string]config.HandlerConfig) *Resolver {
	configs := make(map[string]config.HandlerConfig)
	prefixes := make([]string, 0, len(handlers))
	for prefix, cfg := range handlers {
		if !cfg.Enabled {
			continue
		}
		configs[prefix] = cfg
		prefixes = append(prefixes, prefix)
	}
	sort.Slice(prefixes, func(i, j int) bool {
		return len(prefixes[i]) > len(prefixes[j])
	})

	return &Resolver{prefixes: prefixes, configs: configs, general: generalHandler{}}
}

// Resolve matches path against the enabled prefixes (longest-first, on segment
// boundaries) and, on a hit, selects the specialized handler registered under
// the same prefix key or falls back to the general handler.
func (r *Resolver) Resolve(path string) Resolution {
	for _, prefix := range r.prefixes {
		if !matchPrefix(path, prefix) {
			continue
		}
		res := Resolution{Matched: true, Prefix: prefix, Config: r.configs[prefix]}
		if h, ok := lookup(prefix); ok {
			res.Specialized = true
			res.Handler = h
		} else {
			res.Handler = r.general
		}
		return res
	}
	return Resolution{}
}

// matchPrefix reports whether path falls under prefix on a path-segment
// boundary: an exact match, or prefix followed by "/". This keeps "/githubfoo"
// from matching "/github".
func matchPrefix(path, prefix string) bool {
	return path == prefix || strings.HasPrefix(path, prefix+"/")
}

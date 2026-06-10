package handler

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/eli-yip/rss-ai/pkg/config"
)

func enabled(prompt string) config.HandlerConfig {
	return config.HandlerConfig{Enabled: true, Prompt: prompt}
}

func TestResolveLongestPrefixWins(t *testing.T) {
	r := NewResolver(map[string]config.HandlerConfig{
		"/github":       enabled(""),
		"/github/issue": enabled(""),
	})

	res := r.Resolve("/github/issue/123")
	require.True(t, res.Matched)
	require.Equal(t, "/github/issue", res.Prefix)

	res = r.Resolve("/github/pulls/1")
	require.True(t, res.Matched)
	require.Equal(t, "/github", res.Prefix)
}

func TestResolveSegmentBoundary(t *testing.T) {
	r := NewResolver(map[string]config.HandlerConfig{"/github": enabled("")})

	// "/githubfoo" must NOT match "/github" — boundary is a path segment.
	res := r.Resolve("/githubfoo/x")
	require.False(t, res.Matched)

	// exact prefix matches.
	res = r.Resolve("/github")
	require.True(t, res.Matched)
	require.Equal(t, "/github", res.Prefix)

	// trailing slash matches.
	res = r.Resolve("/github/")
	require.True(t, res.Matched)
}

func TestResolveDisabledIgnored(t *testing.T) {
	r := NewResolver(map[string]config.HandlerConfig{
		"/github": {Enabled: false, Prompt: "x"},
	})

	res := r.Resolve("/github/issue/1")
	require.False(t, res.Matched)
}

func TestResolveUnmatched(t *testing.T) {
	r := NewResolver(map[string]config.HandlerConfig{"/github": enabled("")})

	res := r.Resolve("/twitter/user")
	require.False(t, res.Matched)
	require.Equal(t, "", res.Prefix)
	require.False(t, res.Specialized)
}

func TestResolveGeneralFallback(t *testing.T) {
	r := NewResolver(map[string]config.HandlerConfig{"/twitter": enabled("")})

	res := r.Resolve("/twitter/user/x")
	require.True(t, res.Matched)
	require.False(t, res.Specialized)
	require.IsType(t, generalHandler{}, res.Handler)
}

func TestResolveSpecializedHandler(t *testing.T) {
	Register("/__test_special", stubHandler{prompt: "special"})
	r := NewResolver(map[string]config.HandlerConfig{"/__test_special": enabled("")})

	res := r.Resolve("/__test_special/item/1")
	require.True(t, res.Matched)
	require.True(t, res.Specialized)
	require.Equal(t, "special", res.Handler.Prompt())
}

// Same-key rule (spec §4): a specialized handler registered at a more specific
// prefix is dormant when config only enables a shorter prefix; the registry is
// queried with the prefix config matched, not the registry's own keys.
func TestResolveSameKeyRule(t *testing.T) {
	Register("/__test_sk/issue", stubHandler{prompt: "issue"})
	r := NewResolver(map[string]config.HandlerConfig{"/__test_sk": enabled("")})

	res := r.Resolve("/__test_sk/issue/1")
	require.True(t, res.Matched)
	require.Equal(t, "/__test_sk", res.Prefix)
	require.False(t, res.Specialized) // the /__test_sk/issue handler is dormant
}

func TestEffectivePromptPriority(t *testing.T) {
	Register("/__test_ep", stubHandler{prompt: "handler-default"})

	// config prompt wins over handler default.
	r := NewResolver(map[string]config.HandlerConfig{"/__test_ep": enabled("config-prompt")})
	res := r.Resolve("/__test_ep/x")
	require.Equal(t, "config-prompt", res.EffectivePrompt())

	// empty config prompt + specialized handler -> handler default.
	r = NewResolver(map[string]config.HandlerConfig{"/__test_ep": enabled("")})
	res = r.Resolve("/__test_ep/x")
	require.Equal(t, "handler-default", res.EffectivePrompt())

	// empty config prompt + general handler -> built-in default.
	r = NewResolver(map[string]config.HandlerConfig{"/twitter": enabled("")})
	res = r.Resolve("/twitter/x")
	require.Equal(t, defaultPrompt, res.EffectivePrompt())
}

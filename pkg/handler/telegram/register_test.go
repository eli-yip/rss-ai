package telegram

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/eli-yip/rss-ai/pkg/config"
	"github.com/eli-yip/rss-ai/pkg/handler"
)

// The package init() registers under Prefix; a resolver enabling that same key
// must therefore resolve channel paths to this specialized handler.
func TestResolverPicksTelegramHandler(t *testing.T) {
	r := handler.NewResolver(map[string]config.HandlerConfig{
		Prefix: {Enabled: true},
	})

	res := r.Resolve(Prefix + "/durov/123")
	require.True(t, res.Matched)
	require.True(t, res.Specialized)
	require.Equal(t, Prefix, res.Prefix)
	require.Equal(t, telegramPrompt, res.EffectivePrompt())
}

// A config prompt still overrides the handler's built-in one (spec §4).
func TestConfigPromptOverrides(t *testing.T) {
	r := handler.NewResolver(map[string]config.HandlerConfig{
		Prefix: {Enabled: true, Prompt: "custom ops prompt"},
	})

	res := r.Resolve(Prefix + "/durov/1")
	require.True(t, res.Specialized)
	require.Equal(t, "custom ops prompt", res.EffectivePrompt())
}

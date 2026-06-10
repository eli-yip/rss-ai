package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/eli-yip/rss-ai/pkg/config"
)

func TestInitLoadsConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := `
[app]
env = "prod"
[log]
level = "debug"
[server]
addr = ":9090"
[db]
dsn = "postgres://localhost/test"
[upstream]
base_url = "https://rsshub.example.com"
[ai]
base_url = "https://api.openai.com/v1"
api_key = "sk-test"
model = "gpt-4o-mini"
rpm = 10000
request_timeout = "30s"
[gateway]
wait_timeout = "10s"
[handlers."/twitter"]
enabled = true
prompt = "rewrite"
`
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	require.NoError(t, config.Init(path))

	require.Equal(t, "prod", config.C.App.Env)
	require.Equal(t, ":9090", config.C.Server.Addr)
	require.Equal(t, 10000, config.C.AI.RPM)
	require.Equal(t, "https://rsshub.example.com", config.C.Upstream.BaseURL)

	h, ok := config.C.Handlers["/twitter"]
	require.True(t, ok)
	require.True(t, h.Enabled)
	require.Equal(t, "rewrite", h.Prompt)
}

func TestInitCreatesDefaultWhenMissing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	require.NoError(t, config.Init(path))
	require.FileExists(t, path)
	require.Equal(t, "dev", config.C.App.Env)
	require.Equal(t, ":8080", config.C.Server.Addr)
	require.Equal(t, "info", config.C.Log.Level)
}

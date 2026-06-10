package store_test

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/eli-yip/rss-ai/pkg/store"
)

func TestStoreAutoMigrateAndPing(t *testing.T) {
	dsn := os.Getenv("RSS_AI_TEST_DSN")
	if dsn == "" {
		t.Skip("RSS_AI_TEST_DSN not set; skipping DB integration test")
	}

	st, err := store.New(dsn)
	require.NoError(t, err)
	require.NoError(t, st.Ping(context.Background()))
}

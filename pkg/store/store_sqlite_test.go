package store_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/eli-yip/rss-ai/pkg/store"
)

// SQLite needs no external service (pure-Go ncruces WASM driver), so these run
// by default — unlike the RSS_AI_TEST_DSN-gated Postgres tests.

func TestNewUnsupportedDSN(t *testing.T) {
	_, err := store.New("mysql://user:pass@host/db")
	require.Error(t, err)
}

func TestSQLiteFileSaveLookupAndUpdate(t *testing.T) {
	dsn := "sqlite://" + filepath.Join(t.TempDir(), "rss-ai.db")
	st, err := store.New(dsn)
	require.NoError(t, err)

	ctx := context.Background()
	const h = "/sqlite"

	require.NoError(t, st.SaveTitle(ctx, store.ItemTitle{
		ID: "id-1", Handler: h, SourceTitle: "src", RewrittenTitle: "rew",
	}))
	require.NoError(t, st.Ping(ctx))

	got, err := st.LookupTitles(ctx, h, []string{"id-1", "absent"})
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, "rew", got["id-1"].RewrittenTitle)

	// Re-saving the same (id, handler) updates in place rather than inserting.
	require.NoError(t, st.SaveTitle(ctx, store.ItemTitle{
		ID: "id-1", Handler: h, SourceTitle: "src2", RewrittenTitle: "rew2",
	}))
	got, err = st.LookupTitles(ctx, h, []string{"id-1"})
	require.NoError(t, err)
	require.Len(t, got, 1, "still a single row after update")
	require.Equal(t, "rew2", got["id-1"].RewrittenTitle)
}

func TestSQLiteInMemory(t *testing.T) {
	st, err := store.New("sqlite://:memory:")
	require.NoError(t, err)
	require.NoError(t, st.Ping(context.Background()))
}

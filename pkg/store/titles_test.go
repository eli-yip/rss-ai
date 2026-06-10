package store_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/eli-yip/rss-ai/pkg/store"
)

// newTestStore opens a store against RSS_AI_TEST_DSN or skips. Each caller uses a
// unique handler value so concurrent/repeated runs stay isolated (the table is
// shared and not torn down).
func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	dsn := os.Getenv("RSS_AI_TEST_DSN")
	if dsn == "" {
		t.Skip("RSS_AI_TEST_DSN not set; skipping DB integration test")
	}
	st, err := store.New(dsn)
	require.NoError(t, err)
	return st
}

func uniqueHandler(t *testing.T) string {
	t.Helper()
	return fmt.Sprintf("/__test_%s_%d", t.Name(), time.Now().UnixNano())
}

func TestSaveTitleThenLookup(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	h := uniqueHandler(t)

	require.NoError(t, st.SaveTitle(ctx, store.ItemTitle{
		ID: "id-1", Handler: h, SourceTitle: "src", RewrittenTitle: "rew",
	}))

	got, err := st.LookupTitles(ctx, h, []string{"id-1"})
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, "src", got["id-1"].SourceTitle)
	require.Equal(t, "rew", got["id-1"].RewrittenTitle)
}

func TestSaveTitleUpdatesInPlace(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	h := uniqueHandler(t)

	require.NoError(t, st.SaveTitle(ctx, store.ItemTitle{
		ID: "id-1", Handler: h, SourceTitle: "old-src", RewrittenTitle: "old-rew",
	}))
	first, err := st.LookupTitles(ctx, h, []string{"id-1"})
	require.NoError(t, err)
	firstUpdated := first["id-1"].UpdatedAt

	time.Sleep(5 * time.Millisecond)
	require.NoError(t, st.SaveTitle(ctx, store.ItemTitle{
		ID: "id-1", Handler: h, SourceTitle: "new-src", RewrittenTitle: "new-rew",
	}))

	got, err := st.LookupTitles(ctx, h, []string{"id-1"})
	require.NoError(t, err)
	require.Len(t, got, 1, "still a single row after update")
	require.Equal(t, "new-src", got["id-1"].SourceTitle)
	require.Equal(t, "new-rew", got["id-1"].RewrittenTitle)
	require.True(t, got["id-1"].UpdatedAt.After(firstUpdated), "updated_at advances")
}

func TestLookupTitlesMixedPresence(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	h := uniqueHandler(t)

	require.NoError(t, st.SaveTitle(ctx, store.ItemTitle{
		ID: "present", Handler: h, SourceTitle: "s", RewrittenTitle: "r",
	}))

	got, err := st.LookupTitles(ctx, h, []string{"present", "absent"})
	require.NoError(t, err)
	require.Len(t, got, 1)
	_, ok := got["present"]
	require.True(t, ok)
	_, ok = got["absent"]
	require.False(t, ok)
}

func TestLookupTitlesEmptyIDs(t *testing.T) {
	st := newTestStore(t)
	got, err := st.LookupTitles(context.Background(), uniqueHandler(t), nil)
	require.NoError(t, err)
	require.Empty(t, got)
}

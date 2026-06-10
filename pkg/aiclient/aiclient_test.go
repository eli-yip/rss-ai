package aiclient_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/eli-yip/rss-ai/pkg/aiclient"
	"github.com/eli-yip/rss-ai/pkg/config"
)

func TestFakeClientScriptedReply(t *testing.T) {
	f := &aiclient.FakeClient{
		Reply: func(system, user string) (string, error) { return "REPLY:" + user, nil },
	}
	got, err := f.Complete(context.Background(), "sys", "hello")
	require.NoError(t, err)
	require.Equal(t, "REPLY:hello", got)
	require.Equal(t, 1, f.Calls())
}

func TestFakeClientEchoesByDefault(t *testing.T) {
	f := &aiclient.FakeClient{}
	got, err := f.Complete(context.Background(), "sys", "user content")
	require.NoError(t, err)
	require.Equal(t, "user content", got)
}

func TestFakeClientPropagatesError(t *testing.T) {
	want := errors.New("boom")
	f := &aiclient.FakeClient{Reply: func(_, _ string) (string, error) { return "", want }}
	_, err := f.Complete(context.Background(), "s", "u")
	require.ErrorIs(t, err, want)
}

func TestFakeClientBlocksUntilReleased(t *testing.T) {
	f := &aiclient.FakeClient{Block: make(chan struct{})}

	done := make(chan string, 1)
	go func() {
		out, _ := f.Complete(context.Background(), "s", "blocked")
		done <- out
	}()

	select {
	case <-done:
		t.Fatal("Complete returned before Block was released")
	case <-time.After(20 * time.Millisecond):
	}

	close(f.Block)
	select {
	case out := <-done:
		require.Equal(t, "blocked", out)
	case <-time.After(time.Second):
		t.Fatal("Complete did not return after Block closed")
	}
}

func TestFakeClientBlockRespectsContext(t *testing.T) {
	f := &aiclient.FakeClient{Block: make(chan struct{})} // never released
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err := f.Complete(ctx, "s", "u")
	require.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestNewBuildsClient(t *testing.T) {
	c, err := aiclient.New(config.AI{
		BaseURL: "https://api.openai.com/v1",
		APIKey:  "sk-test",
		Model:   "gpt-4o-mini",
	})
	require.NoError(t, err)
	require.NotNil(t, c)
}

package aiclient

import (
	"context"
	"sync/atomic"
)

// FakeClient is a scriptable, optionally-blocking Client for tests. The zero
// value echoes the user content. It is safe for concurrent use.
type FakeClient struct {
	// Reply computes the response for a (system, user) call. When nil, Complete
	// echoes the user content.
	Reply func(system, user string) (string, error)
	// Block, when non-nil, holds every call open until it is received from or
	// closed — letting a test exercise the timeout / singleflight paths. Close it
	// to release all in-flight and future calls.
	Block chan struct{}

	calls atomic.Int64
}

func (f *FakeClient) Complete(ctx context.Context, system, user string) (string, error) {
	f.calls.Add(1)
	if f.Block != nil {
		select {
		case <-f.Block:
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	if f.Reply != nil {
		return f.Reply(system, user)
	}
	return user, nil
}

// Calls reports how many times Complete has been invoked.
func (f *FakeClient) Calls() int { return int(f.calls.Load()) }

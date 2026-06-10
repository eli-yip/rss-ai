package ctxutil

import "context"

// ValueOr extracts a typed value from a context, returning fallback if the key
// is missing or the stored type does not match.
func ValueOr[T any](ctx context.Context, key any, fallback T) T {
	v, ok := ctx.Value(key).(T)
	if !ok {
		return fallback
	}
	return v
}

// ValueFrom extracts a typed value from a context. Returns the value and true
// if the key exists and the type matches, else the zero value and false.
func ValueFrom[T any](ctx context.Context, key any) (T, bool) {
	v, ok := ctx.Value(key).(T)
	return v, ok
}

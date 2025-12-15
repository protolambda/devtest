package devtest

import "context"

type noSkipCtxKeyType struct{}

var noSkipCtxKey = noSkipCtxKeyType{}

func IsMustNotSkip(ctx context.Context) bool {
	v := ctx.Value(noSkipCtxKey)
	if v == nil {
		return false
	}
	return v.(bool)
}

// MustNotSkip annotates a context to signal that the test-like must not be skipped.
// If skipped anyway, FailNow should be called.
func MustNotSkip(ctx context.Context, mustNotSkip bool) context.Context {
	return context.WithValue(ctx, noSkipCtxKey, mustNotSkip)
}

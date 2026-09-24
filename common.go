package devtest

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/protolambda/mustbe/assertion"
	"github.com/protolambda/proto-log/log"
)

// TestDeadline is implemented by *testing.T, but not *testing.B and others.
type TestDeadline interface {
	Deadline() (deadline time.Time, ok bool)
}

// TestParallel is implemented by *testing.T, but not *testing.B and others.
type TestParallel interface {
	Parallel()
}

// CommonT is a subset of testing.T, extended with a few common utils.
// This interface should not be used directly. Instead, use T in test-scope, or P when operating at package level.
//
// This CommonT interface is minimal enough such that it can be implemented by tooling,
// and a *testing.T can be used with minimal wrapping.
type CommonT interface {
	Error(args ...any)
	Errorf(format string, args ...any)
	Fail()
	FailNow()
	SkipNow()

	TempDir() string
	Cleanup(fn func())
	CleanupErr(fn func() error)
	Log(args ...any)
	Logf(format string, args ...any)
	Helper()
	Name() string

	Logger() log.Logger
	Context() context.Context

	Must(a assertion.Assertion)
	Mustf(a assertion.Assertion, msg string, args ...any)
}

type testScopeCtxKeyType struct{}

// testScopeCtxKey is a key added to the test-context to identify the test-scope.
var testScopeCtxKey = testScopeCtxKeyType{}

// testScopeValue wraps a string to implement slog.LogValuer for context handling
type testScopeValue string

func (t testScopeValue) LogValue() slog.Value {
	return slog.StringValue(string(t))
}

// TestScope retrieves the test-scope from the context
func TestScope(ctx context.Context) string {
	scope := ctx.Value(testScopeCtxKey)
	if scope == nil {
		return ""
	}
	if scopeVal, ok := scope.(testScopeValue); ok {
		return string(scopeVal)
	}
	return ""
}

// AddTestScope combines the sub-scope with the test-scope of the context,
// and returns a context with the updated scope value.
func AddTestScope(ctx context.Context, scope string) context.Context {
	prev := TestScope(ctx)
	newScope := testScopeValue(prev + "/" + scope)
	return context.WithValue(ctx, testScopeCtxKey, newScope)
}

// scopeContext creates the context of a test-scope, like testing.TB.Context:
// it is canceled just before the Cleanup functions of tb run.
// The context has the values of parent, and is canceled when parent is done.
func scopeContext(parent context.Context, tb testing.TB) context.Context {
	// Derived from the tb context, the context is canceled synchronously with it,
	// before the testing package runs the cleanup functions.
	ctx, cancel := context.WithCancel(valuesContext{Context: tb.Context(), values: parent})
	stop := context.AfterFunc(parent, cancel)
	tb.Cleanup(func() {
		stop()
		// The tb context is canceled by now, unless tb is a custom testing.TB implementation.
		cancel()
	})
	return ctx
}

// valuesContext is the Context, with a fallback to the values of another context.
type valuesContext struct {
	context.Context
	values context.Context
}

func (c valuesContext) Value(key any) any {
	if v := c.Context.Value(key); v != nil {
		return v
	}
	return c.values.Value(key)
}

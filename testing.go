package devtest

import (
	"context"
	"fmt"
	"io"
	"runtime"
	"testing"
	"time"

	"github.com/protolambda/mustbe"
	"github.com/protolambda/mustbe/assertion"
	"github.com/protolambda/mustbe/be"
	"github.com/protolambda/proto-log/log"
)

var (
	// RootContext is the context that is used for the root of the test suite.
	// It should be set for good before any tests are run.
	RootContext = context.Background()
)

// T is the test-scope testing interface.
//
// T embeds testing.TB, so it can be passed to code that expects a testing.TB,
// and new testing.TB features are forwarded without changes to T.
// The methods that produce output (Log, Logf, Error, Errorf, Fatal, Fatalf, Skip, Skipf)
// are routed through the Logger instead, and attributed to the first caller that is not marked with Helper.
type T interface {
	CommonT

	// TempDir creates a temporary directory, and returns the file-path.
	// This directory is cleaned up at the end of the test, and must not be shared between tests.
	TempDir() string

	// Cleanup runs the given function at the end of the test-scope,
	// or at the end of the sub-test (if this is a nested test).
	// This function will clean-up before the package-level testing scope may be complete.
	// Do not use the test-scope cleanup with shared resources.
	Cleanup(fn func())

	// Run runs the given function as a sub-test.
	// This requires the wrapped testing.TB to be a *testing.T: sub-benchmarks are not supported.
	Run(name string, fn func(T))

	// Context returns a context that is canceled just before the Cleanup functions run,
	// like testing.TB.Context. Cleanup functions can wait for resources that shut down on Context().Done().
	// The context inherits the values and cancellation of the parent-test-scope (or RootContext).
	// If the test has a deadline, the context expires 3 seconds before it.
	Context() context.Context

	// WithContext makes a copy of T with a specific context.
	// The ctx must match the test-scope of the existing context.
	// This function is used to create a T with annotated context, e.g. a specific resource, rather than a sub-scope.
	WithContext(ctx context.Context) T

	// Parallel signals that this test is to be run in parallel with (and only with) other parallel tests.
	Parallel()

	// Skip is equivalent to Log followed by SkipNow.
	Skip(args ...any)
	// Skipped reports whether the test was skipped.
	Skipped() bool
	// Skipf is equivalent to Logf followed by SkipNow.
	Skipf(format string, args ...any)
	// SkipNow marks the test as skipped and stops test execution.
	SkipNow()

	// Deadline reports the time at which the test binary will have
	// exceeded the timeout specified by the -timeout flag.
	//
	// The ok result is false if the -timeout flag indicates "no timeout" (0).
	Deadline() (deadline time.Time, ok bool)

	// Output returns a writer to write to the underlying TB.Output
	Output() io.Writer

	// Helper marks the calling function as a test helper function.
	// Output is attributed to the first caller that is not a helper.
	Helper()

	// Fatal is equivalent to Log followed by FailNow, but logged at error level.
	Fatal(args ...any)
	// Fatalf is equivalent to Logf followed by FailNow, but logged at error level.
	Fatalf(format string, args ...any)

	// TestOnly distinguishes the interface from other testing interfaces,
	// such as P, the one used at package-level for shared resources.
	// It is a no-op marker: implementations outside of this package can implement it.
	TestOnly()

	testing.TB
}

// This testing subset supports Must/Mustf
var _ mustbe.MT = T(nil)

// testingT implements the T interface by wrapping around a regular golang testing.T
type testingT struct {
	testing.TB // embedded, to forward the testing.TB methods that T does not override.
	logger     log.Logger
	ctx        context.Context
	// helpers is shared with the copies made by WithContext.
	helpers *helperSet
}

func (t *testingT) Helper() {
	t.helpers.mark(1)
}

func (t *testingT) Error(args ...any) {
	t.Helper()
	// Note: the test-logger catches panics when the test is logged to after test-end.
	// Note: we do not use t.Error directly, to keep the log-formatting more consistent.
	logAt(t.logger, t.helpers, log.LevelError, sprintln(args...))
	t.Fail()
}

func (t *testingT) Errorf(format string, args ...any) {
	t.Helper()
	// Note: the test-logger catches panics when the test is logged to after test-end.
	// Note: we do not use t.Errorf directly, to keep the log-formatting more consistent.
	logAt(t.logger, t.helpers, log.LevelError, fmt.Sprintf(format, args...))
	t.Fail()
}

func (t *testingT) Fatal(args ...any) {
	t.Helper()
	t.Error(args...)
	t.FailNow()
}

func (t *testingT) Fatalf(format string, args ...any) {
	t.Helper()
	t.Errorf(format, args...)
	t.FailNow()
}

func (t *testingT) Fail() {
	// if we already closed and failed, then this error is stale
	if t.ctx.Err() != nil && t.Failed() {
		return
	}
	t.TB.Fail()
}

func (t *testingT) FailNow() {
	// If we already closed and failed the test-scope, then there is nothing to do.
	// This happens on e.g. a go-routine spawned by an Eventually-assertion, when the time runs out,
	// the ctx is closed, a shared resource fails to do a lookup because of the ctx-timeout,
	// and the eventually-condition then does a no-error check, causing the test-scope to error after it already had.
	if t.ctx.Err() != nil && t.TB.Failed() {
		// Exit the go-routine that is running us (actual testing.T FailNow does this too).
		// Still runs deferred calls on this go-routine.
		runtime.Goexit()
		return
	}
	t.TB.FailNow()
}

func (t *testingT) TempDir() string {
	return t.TB.TempDir()
}

func (t *testingT) Cleanup(fn func()) {
	t.TB.Cleanup(fn)
}

func (t *testingT) CleanupErr(fn func() error) {
	t.Cleanup(func() {
		t.Mustf(be.NoError(fn()), "Cleanup error")
	})
}

func (t *testingT) Log(args ...any) {
	t.Helper()
	// Note: the test-logger catches panics when the test is logged to after test-end.
	// Note: we do not use t.Log directly, to keep the log-formatting more consistent.
	logAt(t.logger, t.helpers, log.LevelInfo, sprintln(args...))
}

func (t *testingT) Logf(format string, args ...any) {
	t.Helper()
	// Note: the test-logger catches panics when the test is logged to after test-end.
	// Note: we do not use t.Logf directly, to keep the log-formatting more consistent.
	logAt(t.logger, t.helpers, log.LevelInfo, fmt.Sprintf(format, args...))
}

func (t *testingT) Name() string {
	return t.TB.Name()
}

func (t *testingT) Logger() log.Logger {
	return t.logger
}

func (t *testingT) Context() context.Context {
	return t.ctx
}

func (t *testingT) WithContext(ctx context.Context) T {
	expected := TestScope(t.ctx)
	got := TestScope(ctx)
	t.Mustf(be.Equal(expected, got), "cannot replace context with different test-scope")
	logger := t.logger.WithContext(ctx)
	out := &testingT{
		TB:      t.TB,
		logger:  logger,
		ctx:     ctx,
		helpers: t.helpers,
	}
	return out
}

func (t *testingT) Must(a assertion.Assertion) {
	t.Helper()
	mustbe.Must(t, a)
}

func (t *testingT) Mustf(a assertion.Assertion, msg string, args ...any) {
	t.Helper()
	mustbe.Must(t, assertion.Annotated{Inner: a, Msg: msg, Args: args})
}

func (t *testingT) Run(name string, fn func(T)) {
	t.Helper()
	if tt, ok := t.TB.(*testing.T); ok {
		tt.Run(name, func(subGoT *testing.T) {
			ctx := scopeContext(AddTestScope(t.ctx, name), subGoT)
			logger := t.logger.WithContext(ctx) // attach the sub-test context as default log-context
			subT := &testingT{
				TB:      subGoT,
				logger:  logger,
				ctx:     ctx,
				helpers: newHelperSet(),
			}
			fn(subT)
		})
	} else {
		t.Error("Must be in test env to run sub-test")
		t.FailNow()
	}
}

func (t *testingT) Parallel() {
	t.Helper()
	tt, ok := t.TB.(TestParallel)
	if ok {
		t.Log("Running test in parallel")
		tt.Parallel()
	} else {
		t.Error("Must be in test env to run in parallel")
		t.FailNow()
	}
}

func (t *testingT) Skip(args ...any) {
	t.Helper()
	t.Log(args...)
	t.SkipNow()
}

func (t *testingT) Skipf(format string, args ...any) {
	t.Helper()
	t.Logf(format, args...)
	t.SkipNow()
}

func (t *testingT) SkipNow() {
	t.Helper()
	if IsMustNotSkip(t.ctx) {
		t.Error("Unexpected test-skip")
		t.FailNow()
		return
	}
	t.TB.SkipNow()
}

// Deadline reports the time at which the test binary will have
// exceeded the timeout specified by the -timeout flag.
//
// The ok result is false if the -timeout flag indicates "no timeout" (0).
func (t *testingT) Deadline() (deadline time.Time, hasDeadline bool) {
	if tt, ok := t.TB.(TestDeadline); ok {
		return tt.Deadline()
	}
	return time.Time{}, false
}

func (t *testingT) Output() io.Writer {
	return t.TB.Output()
}

func (t *testingT) TestOnly() {}

var _ T = (*testingT)(nil)

// DefaultTestLogLevel is set to info level to show relevant logs without being overly verbose unless configured otherwise.
var DefaultTestLogLevel = log.LevelInfo

// SerialT wraps around a testing.TB, and turns it into a T.
// The T context has the values of RootContext, is canceled just before the Cleanup functions run
// (like testing.TB.Context), and expires 3 seconds before the test deadline, if any (see TestDeadline).
func SerialT(t testing.TB) T {
	ctx := scopeContext(AddTestScope(RootContext, t.Name()), t)
	if tt, ok := t.(TestDeadline); ok {
		if deadline, hasDeadline := tt.Deadline(); hasDeadline {
			var cancel context.CancelFunc
			ctx, cancel = context.WithDeadline(ctx, deadline.Add(-3*time.Second))
			t.Cleanup(cancel)
		}
	}

	logger := log.TestLogger(t, log.LevelMod(DefaultTestLogLevel))
	// Set the default context:
	// any log call without context will use this.
	// utils will close resources based on this.
	logger = logger.WithContext(ctx)
	out := &testingT{
		TB:      t,
		logger:  logger,
		ctx:     ctx,
		helpers: newHelperSet(),
	}
	return out
}

// ParallelT creates a T interface with parallel testing enabled by default.
// This requires the testing.TB to support Parallel (see TestParallel).
func ParallelT(t testing.TB) T {
	out := SerialT(t)
	out.Parallel()
	return out
}

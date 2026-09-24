package devtest

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"

	"github.com/protolambda/mustbe"
	"github.com/protolambda/mustbe/assertion"
	"github.com/protolambda/mustbe/be"
	"github.com/protolambda/proto-log/log"
)

var FailNoMsg = errors.New("fail (no message)")

// P is the package-scope testing interface, to host resources shared between tests,
// e.g. in TestMain, or in test-like Go programs (see Run).
type P interface {
	CommonT

	// WithContext makes a copy of P with a specific context.
	// The ctx must match the test-scope of the existing context.
	// This function is used to create a P with annotated context, e.g. a specific resource.
	// The copy shares the cleanup and the error handling with the original.
	WithContext(ctx context.Context) P

	// Context returns the package-scope context.
	// Like T, the context is canceled just before the Cleanup functions run (see Close).
	Context() context.Context

	// TempDir creates a temporary directory, and returns the file-path.
	// This directory is cleaned up at the end of the package,
	// and can be shared safely between tests that run in that package scope.
	TempDir() string

	// Cleanup runs the given function at the end of the package-scope.
	// This function will clean-up once the package-level testing is fully complete.
	// These resources can thus be shared safely between tests.
	Cleanup(fn func())

	// PackageOnly distinguishes the interface from other testing interfaces,
	// such as T, the one used at test-level for test-scope resources.
	// It is a no-op marker: implementations outside of this package can implement it.
	PackageOnly()

	// Close closes the testing handle. This cancels the context and then runs all cleanup.
	Close()
}

// implP is a P implementation that is used for package-level testing, and may be used by tooling as well.
// This is used in TestMain to manage resources that outlive a single test-scope.
type implP struct {
	// scopeName, for t.Name() purposes
	scopeName string

	// logger is used for logging. Regular test errors will also be redirected to get logged here.
	logger log.Logger

	ctx context.Context

	// shared with the copies made by WithContext
	*pShared
}

// pShared is the state of P that is shared between the copies made by WithContext.
type pShared struct {
	// onErr will be called to register an error (soft-error, or critical before onFail)
	onErr func(err error)
	// onFailNow will be called to register a critical failure.
	// The implementer can choose to panic, crit-log, exit, etc. as preferred.
	onFailNow func()
	// onSkipNow will be called to skip the test immediately.
	onSkipNow func()

	// cancel cancels the package-scope context
	cancel context.CancelFunc

	helpers *helperSet

	// cleanup stack
	cleanupLock    sync.Mutex
	cleanupBacklog []func()
}

var _ P = (*implP)(nil)

func (t *implP) Error(args ...any) {
	t.Helper()
	errMsg := sprintln(args...)
	logAt(t.logger, t.helpers, log.LevelError, errMsg)
	t.onErr(errors.New(errMsg))
}

func (t *implP) Errorf(format string, args ...any) {
	t.Helper()
	errMsg := fmt.Sprintf(format, args...)
	logAt(t.logger, t.helpers, log.LevelError, errMsg)
	t.onErr(errors.New(errMsg))
}

func (t *implP) Fail() {
	t.onErr(FailNoMsg)
}

func (t *implP) FailNow() {
	t.onFailNow()
}

func (t *implP) SkipNow() {
	t.Helper()
	if IsMustNotSkip(t.ctx) {
		t.Error("Unexpected test-skip")
		t.FailNow()
		return
	}
	t.onSkipNow()
}

func (t *implP) TempDir() string {
	t.Helper()
	// The last "*" will be replaced with the random temp dir name
	tempDir, err := os.MkdirTemp("", "devtest-*")
	if err != nil {
		t.Errorf("failed to create temp dir: %v", err)
		t.FailNow()
	}
	t.Mustf(be.NotEqual("", tempDir), "sanity check temp-dir path is not empty")
	t.Mustf(be.NotEqual("/", tempDir), "sanity-check temp-dir is not root")
	t.Cleanup(func() {
		if err := os.RemoveAll(tempDir); err != nil {
			t.logger.Error("Failed to clean up temp dir", "dir", tempDir, "err", err)
		}
	})
	return tempDir
}

func (t *implP) Cleanup(fn func()) {
	t.cleanupLock.Lock()
	defer t.cleanupLock.Unlock()
	t.cleanupBacklog = append(t.cleanupBacklog, fn)
}

func (t *implP) CleanupErr(fn func() error) {
	t.Cleanup(func() {
		t.Mustf(be.NoError(fn()), "Cleanup error")
	})
}

func (t *implP) Log(args ...any) {
	t.Helper()
	logAt(t.logger, t.helpers, log.LevelInfo, sprintln(args...))
}

func (t *implP) Logf(format string, args ...any) {
	t.Helper()
	logAt(t.logger, t.helpers, log.LevelInfo, fmt.Sprintf(format, args...))
}

// Helper marks the calling function as a helper function.
// Output is attributed to the first caller that is not a helper.
func (t *implP) Helper() {
	t.helpers.mark(1)
}

func (t *implP) Name() string {
	return t.scopeName
}

func (t *implP) Logger() log.Logger {
	return t.logger
}

func (t *implP) Context() context.Context {
	return t.ctx
}

func (t *implP) WithContext(ctx context.Context) P {
	t.Helper()
	expected := TestScope(t.ctx)
	got := TestScope(ctx)
	t.Mustf(be.Equal(expected, got), "cannot replace context with different test-scope")
	return &implP{
		scopeName: t.scopeName,
		logger:    t.logger.WithContext(ctx),
		ctx:       ctx,
		pShared:   t.pShared,
	}
}

func (t *implP) Must(a assertion.Assertion) {
	t.Helper()
	mustbe.Must(t, a)
}

func (t *implP) Mustf(a assertion.Assertion, msg string, args ...any) {
	t.Helper()
	mustbe.Must(t, assertion.Annotated{Inner: a, Msg: msg, Args: args})
}

// Close runs the cleanup of this implP implementation.
//
// This cancels the package-wide test context.
//
// It then runs the backlog of cleanup functions, in reverse order (last registered cleanup runs first).
// It's inspired by the Go cleanup handler, fully cleaning up,
// even continuing to clean up when panics happen.
// It does not recover the go-routine from panicking however, that is up to the caller.
func (t *implP) Close() {
	t.cancel()
	t.runCleanup()
}

func (t *implP) runCleanup() {
	// run remaining cleanups, even if a cleanup panics,
	// but don't recover the panic
	defer func() {
		t.cleanupLock.Lock()
		recur := len(t.cleanupBacklog) > 0
		t.cleanupLock.Unlock()
		if recur {
			t.logger.Error("Last cleanup panicked, continuing cleanup attempt now")
			t.runCleanup()
		}
	}()

	for {
		// Pop a cleanup item, and execute it in unlocked state,
		// in case cleanups produce new cleanups.
		var cleanup func()
		t.cleanupLock.Lock()
		if len(t.cleanupBacklog) > 0 {
			last := len(t.cleanupBacklog) - 1
			cleanup = t.cleanupBacklog[last]
			t.cleanupBacklog = t.cleanupBacklog[:last]
		}
		t.cleanupLock.Unlock()
		if cleanup == nil {
			return
		}
		cleanup()
	}
}

func (t *implP) PackageOnly() {}

func NewP(ctx context.Context, logger log.Logger, onErr func(err error), onFailNow func(), onSkipNow func()) P {
	ctx, cancel := context.WithCancel(ctx)
	out := &implP{
		scopeName: "pkg",
		logger:    logger,
		ctx:       AddTestScope(ctx, "pkg"),
		pShared: &pShared{
			onErr:     onErr,
			onFailNow: onFailNow,
			onSkipNow: onSkipNow,
			cancel:    cancel,
			helpers:   newHelperSet(),
		},
	}
	return out
}

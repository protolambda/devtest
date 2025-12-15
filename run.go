package devtest

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sync"

	"github.com/protolambda/proto-log/log"
)

// RunCritErr is joined into the error-cause when Run fails critically.
var RunCritErr = errors.New("critical error")

// RunSkipAfterErr is joined into the error-cause when Run skips after a non-critical error.
var RunSkipAfterErr = errors.New("skipped after error")

// RunSkipErr is joined into the error-cause when Run skips without error.
var RunSkipErr = errors.New("skipped")

// Run runs the fn in its own go-routine,
// and returns a new context that is closed with the result as cause.
// The function has access to P, like a test, to manage errors.
// When fn panics, fails or skips, then execution stops,
// P is cleaned up, and the error is applied to the context-cause.
// See RunCritErr, RunSkipAfterErr, RunSkipErr.
// If fn completes successfully, the context-cause will be context.Canceled.
// The ctx is passed into fn, and used as starting-point for P.
func Run(ctx context.Context, logger log.Logger, fn func(p P)) context.Context {
	// Collect and join the errors we see during function execution
	var errOut error
	errOutLock := new(sync.Mutex)
	onErr := func(err error) {
		errOutLock.Lock()
		defer errOutLock.Unlock()
		errOut = errors.Join(errOut, fmt.Errorf("run err: %w", err))
	}
	// The function will run in its own go-routine.
	// Once we fail / skip, abort execution of the go-routine.

	// FailNow = immediate stop, critical error
	onFailNow := func() {
		errOutLock.Lock()
		defer errOutLock.Unlock()
		errOut = errors.Join(errOut, RunCritErr)
		runtime.Goexit() // deferred calls will still run
	}
	// SkipNow = immediate stop, might be after previous non-crit error
	onSkipNow := func() {
		errOutLock.Lock()
		defer errOutLock.Unlock()
		if errOut != nil {
			errOut = errors.Join(errOut, RunSkipAfterErr)
		} else {
			errOut = RunSkipErr
		}
		runtime.Goexit() // deferred calls will still run
	}
	p := NewP(ctx, logger, onErr, onFailNow, onSkipNow)
	outCtx, cancelCause := context.WithCancelCause(context.Background())
	go func() {
		// Catch any panic in the function.
		// And close the context with the error result.
		defer func() {
			e := recover()
			errOutLock.Lock()
			defer errOutLock.Unlock()
			// A panic is considered a critical error like FailNow.
			if e != nil {
				var x error
				if err, ok := e.(error); ok {
					x = fmt.Errorf("run panic(err): %w", err)
				} else {
					x = fmt.Errorf("run panic(msg): %q", e)
				}
				errOut = errors.Join(errOut, x, RunCritErr)
			}
			cancelCause(errOut)
		}()
		// p.Close must run before we cancel the out context,
		// to ensure all P cleanup functions run before program exit.
		defer p.Close()
		fn(p)
	}()
	return outCtx
}

package devtest_test

import (
	"context"
	"errors"
	"os"
	"strings"

	"github.com/protolambda/devtest"
	"github.com/protolambda/mustbe/be"
	"github.com/protolambda/proto-log/log"
)

func ExampleRun() {
	work := func(p devtest.P) {
		p.Logger().Info("Hello world", "foo", 123)
		tmpDir := p.TempDir()
		p.Logger().Info("Created temp dir")
		p.Cleanup(func() {
			p.Logger().Info("Removing temp dir")
			p.Must(be.NoError(os.Remove(tmpDir)))
			p.Logger().Info("Cleaned up!")
		})
		p.Mustf(be.True(true), "Assert things")
	}

	ctx := context.Background()
	logger := log.New(log.TerminalHandler(os.Stdout, log.WithExcludeTime(true)))
	out := devtest.Run(ctx, logger, work)
	// This runs on its own routine. Await the completion.
	<-out.Done()
	logger.Info("Done!")

	// Output:
	// INFO  Hello world                              foo=123
	// INFO  Created temp dir
	// INFO  Removing temp dir
	// INFO  Cleaned up!
	// INFO  Done!
}

func ExampleMustNotSkip() {
	work := func(p devtest.P) {
		p.Logger().Info("Hello world", "foo", 123)
		p.SkipNow()
		p.Mustf(be.Failed(), "should not reach this")
	}

	ctx := context.Background()
	ctx = devtest.MustNotSkip(ctx, true)
	logger := log.New(log.TerminalHandler(os.Stdout, log.WithExcludeTime(true)))
	out := devtest.Run(ctx, logger, work)
	// This runs on its own routine. Await the completion.
	<-out.Done()

	// Use errors.As to extract the RunError and access error and stack separately
	cause := context.Cause(out)
	var runErr devtest.RunError
	if !errors.As(cause, &runErr) {
		panic("expected a RunError")
	}
	// Get the wrapped error (without stack trace in message)
	logger.Info("wrapped error:", "err", runErr.Unwrap())
	// Get just the stack trace
	hasStack := strings.Contains(runErr.Stack(), "goroutine")
	logger.Info("has stack trace:", "has", hasStack)
	logger.Info("Done!")

	// Output:
	// INFO  Hello world                              foo=123
	// ERROR Unexpected test-skip
	//
	// INFO  wrapped error:                           err="run err: Unexpected test-skip\n\ncritical error"
	// INFO  has stack trace:                         has=true
	// INFO  Done!
}

package devtest_test

import (
	"context"
	"os"

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
	logger.Info("reason:", "err", context.Cause(out))
	logger.Info("Done!")

	// Output:
	// INFO  Hello world                              foo=123
	// ERROR Unexpected test-skip
	//
	// INFO  reason:                                  err="run err: Unexpected test-skip\n\ncritical error"
	// INFO  Done!
}

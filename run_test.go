package devtest

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/protolambda/mustbe"
	"github.com/protolambda/mustbe/be"
	"github.com/protolambda/proto-log/log"
)

func TestRunSuccess(gt *testing.T) {
	t := mustbe.WrapT(gt)
	ctx := t.Context()
	logger := log.TestLogger(gt)
	completed := false
	cleaned := false
	out := Run(ctx, logger, func(p P) {
		p.Cleanup(func() {
			cleaned = true
		})
		p.Logger().Info("Hello world", "foo", 123)
		time.Sleep(time.Second)
		completed = true
	})
	<-out.Done()
	t.Must(be.True(completed))
	t.Must(be.True(cleaned))
	result := context.Cause(out)
	t.Must(be.NonNil(result))
	t.Must(be.DeepEqual(result, context.Canceled))
}

func TestRunCrit(gt *testing.T) {
	t := mustbe.WrapT(gt)
	ctx := t.Context()
	logger := log.TestLogger(gt)
	cleaned := false
	out := Run(ctx, logger, func(p P) {
		p.Cleanup(func() {
			cleaned = true
		})
		p.Logger().Info("Hello world", "foo", 123)
		p.Mustf(be.Failed(), "testing a fail case")
		gt.Fatalf("P should not have allowed continuation!")
	})
	<-out.Done()
	t.Must(be.True(cleaned))
	result := context.Cause(out)
	t.Must(be.NonNil(result))
	t.Must(be.Substring(result.Error(), "testing a fail case"))
	t.Must(be.ErrorIs(result, RunCritErr))
}

func TestRunPanicMsg(gt *testing.T) {
	t := mustbe.WrapT(gt)
	ctx := t.Context()
	logger := log.TestLogger(gt)
	cleaned := false
	out := Run(ctx, logger, func(p P) {
		p.Cleanup(func() {
			cleaned = true
		})
		p.Logger().Info("Hello world", "foo", 123)
		panic("testing panic case")
	})
	<-out.Done()
	t.Must(be.True(cleaned))
	result := context.Cause(out)
	t.Must(be.NonNil(result))
	t.Must(be.Substring(result.Error(), "panic(msg): \"testing panic case\""))
	t.Must(be.ErrorIs(result, RunCritErr))
}

func TestRunPanicErr(gt *testing.T) {
	t := mustbe.WrapT(gt)
	ctx := t.Context()
	logger := log.TestLogger(gt)
	testErr := errors.New("special test panic error")
	out := Run(ctx, logger, func(p P) {
		p.Logger().Info("Hello world", "foo", 123)
		panic(testErr)
	})
	<-out.Done()
	result := context.Cause(out)
	t.Must(be.NonNil(result))
	t.Must(be.Substring(result.Error(), "panic(err): special test panic error"))
	t.Must(be.ErrorIs(result, testErr))
	t.Must(be.ErrorIs(result, RunCritErr))
}

func TestRunSkip(gt *testing.T) {
	t := mustbe.WrapT(gt)
	ctx := t.Context()
	logger := log.TestLogger(gt)
	out := Run(ctx, logger, func(p P) {
		p.Logger().Info("Hello world", "foo", 123)
		p.SkipNow()
		gt.Fatalf("P should not have allowed continuation!")
	})
	<-out.Done()
	result := context.Cause(out)
	t.Must(be.NonNil(result))
	t.Must(be.ErrorIs(result, RunSkipErr))
}

func TestRunErrAndSkip(gt *testing.T) {
	t := mustbe.WrapT(gt)
	ctx := t.Context()
	logger := log.TestLogger(gt)
	out := Run(ctx, logger, func(p P) {
		p.Logger().Info("Hello world", "foo", 123)
		p.Error("soft err 1")
		p.Error("soft err 2")
		p.SkipNow()
		gt.Fatalf("P should not have allowed continuation!")
	})
	<-out.Done()
	result := context.Cause(out)
	t.Must(be.NonNil(result))
	t.Must(be.Substring(result.Error(), "soft err 1"))
	t.Must(be.Substring(result.Error(), "soft err 2"))
	t.Must(be.ErrorIs(result, RunSkipAfterErr))
}

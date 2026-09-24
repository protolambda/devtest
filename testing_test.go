package devtest

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/protolambda/mustbe"
	"github.com/protolambda/mustbe/be"
	"github.com/protolambda/proto-log/log"
)

func TestSkip(gt *testing.T) {
	skips := map[string]func(t T){
		"Skip":    func(t T) { t.Skip("skipping") },
		"Skipf":   func(t T) { t.Skipf("skipping %d", 1) },
		"SkipNow": func(t T) { t.SkipNow() },
	}
	for name, skip := range skips {
		var subGoT *testing.T
		var skippedBefore, skippedAfter bool
		gt.Run(name, func(gt *testing.T) {
			subGoT = gt
			t := SerialT(gt)
			t.Cleanup(func() {
				skippedAfter = t.Skipped()
			})
			skippedBefore = t.Skipped()
			skip(t)
			gt.Fatal("skip should stop the test")
		})
		t := mustbe.WrapT(gt)
		t.Must(be.False(skippedBefore))
		t.Must(be.True(skippedAfter))
		t.Must(be.True(subGoT.Skipped()))
	}
}

func TestParallelT(gt *testing.T) {
	var count atomic.Int32
	gt.Run("group", func(gt *testing.T) {
		for _, name := range []string{"a", "b"} {
			gt.Run(name, func(gt *testing.T) {
				ParallelT(gt)
				count.Add(1)
			})
		}
	})
	t := mustbe.WrapT(gt)
	t.Must(be.Equal(int32(2), count.Load()))
}

func TestParallelHelper(gt *testing.T) {
	var sum atomic.Int32
	Parallel(SerialT(gt), []int32{1, 2, 3}, func(t T, v int32) {
		sum.Add(v)
	})
	t := mustbe.WrapT(gt)
	t.Must(be.Equal(int32(6), sum.Load()))
}

func TestSerialTBenchmark(gt *testing.T) {
	var ctx context.Context
	testing.Benchmark(func(b *testing.B) {
		t := SerialT(b)
		ctx = t.Context()
		_, hasDeadline := t.Deadline()
		t.Must(be.False(hasDeadline))
	})
	t := mustbe.WrapT(gt)
	t.Must(be.NonNil(ctx))
	t.Must(be.ErrorIs(ctx.Err(), context.Canceled))
}

func newTestP(gt *testing.T) P {
	return NewP(context.Background(), log.TestLogger(gt),
		func(err error) { gt.Errorf("unexpected error: %v", err) },
		func() { gt.Fatal("unexpected FailNow") },
		func() { gt.Fatal("unexpected SkipNow") })
}

func TestPClose(gt *testing.T) {
	t := mustbe.WrapT(gt)
	p := newTestP(gt)
	ctx := p.Context()
	var cleanupCtxErr error
	p.Cleanup(func() {
		cleanupCtxErr = ctx.Err()
	})
	t.Must(be.NoError(ctx.Err()))
	p.Close()
	t.Must(be.ErrorIs(cleanupCtxErr, context.Canceled))
	t.Must(be.ErrorIs(ctx.Err(), context.Canceled))
}

type ctxKey struct{}

type ctxValueAssertion struct{}

func (ctxValueAssertion) String() string { return "context has value" }

func (ctxValueAssertion) Check(ctx context.Context) error {
	return be.Equal[any]("value", ctx.Value(ctxKey{})).Check(ctx)
}

func TestPWithContext(gt *testing.T) {
	t := mustbe.WrapT(gt)
	p := newTestP(gt)
	derived := p.WithContext(context.WithValue(p.Context(), ctxKey{}, "value"))
	derived.Must(ctxValueAssertion{})
	cleaned := false
	derived.Cleanup(func() {
		cleaned = true
	})
	p.Close()
	t.Must(be.True(cleaned))
	t.Must(be.ErrorIs(derived.Context().Err(), context.Canceled))
}

func TestPTempDir(gt *testing.T) {
	t := mustbe.WrapT(gt)
	p := newTestP(gt)
	dir := p.TempDir()
	t.Must(be.Prefix(filepath.Base(dir), "devtest-"))
	_, err := os.Stat(dir)
	t.Must(be.NoError(err))
	p.Close()
	_, err = os.Stat(dir)
	t.Must(be.ErrorIs(err, os.ErrNotExist))
}

func TestContextCanceledBeforeCleanup(gt *testing.T) {
	cases := map[string]func(gt *testing.T, check func(name string, ctx context.Context) func()){
		"SerialT": func(gt *testing.T, check func(string, context.Context) func()) {
			t := SerialT(gt)
			t.Cleanup(check("T.Cleanup", t.Context()))
			t.CleanupErr(func() error {
				check("T.CleanupErr", t.Context())()
				return nil
			})
			gt.Cleanup(check("testing.T.Cleanup", t.Context()))
			derived := t.WithContext(context.WithValue(t.Context(), ctxKey{}, "value"))
			derived.Cleanup(check("WithContext", derived.Context()))
		},
		"Run": func(gt *testing.T, check func(string, context.Context) func()) {
			SerialT(gt).Run("sub", func(t T) {
				t.Cleanup(check("sub T.Cleanup", t.Context()))
			})
		},
	}
	for name, fn := range cases {
		var errs []string
		gt.Run(name, func(gt *testing.T) {
			fn(gt, func(name string, ctx context.Context) func() {
				return func() {
					if ctx.Err() == nil {
						errs = append(errs, name)
					}
				}
			})
		})
		t := mustbe.WrapT(gt)
		t.Mustf(be.DeepEqual([]string(nil), errs), "contexts not canceled before cleanup")
	}
}

func TestContextParent(gt *testing.T) {
	prev := RootContext
	defer func() { RootContext = prev }()
	rootCtx, cancelRoot := context.WithCancel(context.WithValue(context.Background(), ctxKey{}, "value"))
	RootContext = rootCtx
	gt.Run("sub", func(gt *testing.T) {
		SerialT(gt).Run("sub", func(t T) {
			t.Must(be.Equal[any]("value", t.Context().Value(ctxKey{})))
			t.Must(be.Substring(TestScope(t.Context()), "/sub/sub"))
			t.Must(be.NoError(t.Context().Err()))
			cancelRoot()
			<-t.Context().Done()
		})
	})
}

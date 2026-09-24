package devtest

import (
	"context"
	"fmt"
	"log/slog"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/protolambda/proto-log/log"
)

// helperSet tracks the functions marked with Helper.
// Log output is attributed to the first caller that is not a helper,
// like the testing package does for the output of testing.TB.
//
// The testing package cannot attribute output that goes through the logger,
// and TB.Helper cannot be forwarded: it would mark the forwarding method, not its caller.
type helperSet struct {
	mu    sync.RWMutex
	names map[string]struct{}
}

func newHelperSet() *helperSet {
	return &helperSet{names: make(map[string]struct{})}
}

// mark marks a function on the call stack as helper.
// A skip of 0 marks the caller of mark.
func (h *helperSet) mark(skip int) {
	var pc [1]uintptr
	// skip [runtime.Callers, mark]
	if runtime.Callers(skip+2, pc[:]) == 0 {
		return
	}
	frame, _ := runtime.CallersFrames(pc[:]).Next()
	h.mu.RLock()
	_, ok := h.names[frame.Function]
	h.mu.RUnlock()
	if ok {
		return
	}
	h.mu.Lock()
	h.names[frame.Function] = struct{}{}
	h.mu.Unlock()
}

// callerPC returns the PC of the first function on the call stack that is not a helper.
// A skip of 0 starts at the caller of callerPC.
func (h *helperSet) callerPC(skip int) uintptr {
	var pcs [64]uintptr
	// skip [runtime.Callers, callerPC]
	n := runtime.Callers(skip+2, pcs[:])
	if n == 0 {
		return 0
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	// Callers returns a PC per logical frame (inlined calls included),
	// and each PC is resolved on its own, like a log handler resolves the source of a record.
	for _, pc := range pcs[:n] {
		frame, _ := runtime.CallersFrames([]uintptr{pc}).Next()
		if _, ok := h.names[frame.Function]; !ok {
			return pc
		}
	}
	return pcs[0]
}

// logAt logs the message, with the first caller that is not a helper as source.
func logAt(logger log.Logger, helpers *helperSet, level slog.Level, msg string) {
	// The logger substitutes its default context for the background context.
	ctx := context.Background()
	h := logger.Handler()
	if !h.Enabled(ctx, level) {
		return
	}
	// skip [logAt]
	r := slog.NewRecord(time.Now(), level, msg, helpers.callerPC(1))
	_ = h.Handle(ctx, r)
}

// sprintln formats like fmt.Sprintln, without the trailing newline.
func sprintln(args ...any) string {
	return strings.TrimSuffix(fmt.Sprintln(args...), "\n")
}

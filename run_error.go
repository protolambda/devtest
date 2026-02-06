package devtest

import (
	"fmt"
)

// RunError wraps an error with a stack trace captured at the point of failure.
// Use errors.As to extract the RunError and access the stack trace separately.
type RunError interface {
	error
	Unwrap() error
	Stack() string
}

type runError struct {
	err   error
	stack string
}

// Error implements the error interface.
func (e *runError) Error() string {
	return fmt.Sprintf("%v\nstack:\n%s", e.err, e.stack)
}

// Unwrap returns the wrapped error, enabling errors.Is and errors.As to work.
func (e *runError) Unwrap() error {
	return e.err
}

// Stack returns the stack trace captured when the error occurred.
func (e *runError) Stack() string {
	return e.stack
}

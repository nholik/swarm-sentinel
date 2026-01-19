package runner

import "fmt"

// RuntimeError captures errors that should not stop the runner loop.
type RuntimeError struct {
	Op  string
	Err error
}

func (e *RuntimeError) Error() string {
	return fmt.Sprintf("%s: %v", e.Op, e.Err)
}

func (e *RuntimeError) Unwrap() error {
	return e.Err
}

func wrapRuntime(op string, err error) error {
	if err == nil {
		return nil
	}
	return &RuntimeError{Op: op, Err: err}
}

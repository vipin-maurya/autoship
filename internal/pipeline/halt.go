package pipeline

import "fmt"

// HaltError is a failure that stops the pipeline and stays stopped until a new
// SHA lands or a human runs `autoship resume` (spec §5). Stages return it
// rather than calling into state directly, so the decision to persist a halt
// stays in one place.
type HaltError struct {
	Stage  string
	Reason string
	Err    error
}

func (e *HaltError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %s: %v", e.Stage, e.Reason, e.Err)
	}
	return fmt.Sprintf("%s: %s", e.Stage, e.Reason)
}

func (e *HaltError) Unwrap() error { return e.Err }

// Halt builds a HaltError for the given stage.
func Halt(stage, reason string, err error) *HaltError {
	return &HaltError{Stage: stage, Reason: reason, Err: err}
}

// Haltf builds a HaltError with a formatted reason.
func Haltf(stage string, format string, args ...any) *HaltError {
	return &HaltError{Stage: stage, Reason: fmt.Sprintf(format, args...)}
}

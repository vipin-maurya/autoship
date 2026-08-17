package state

import "time"

// SetHalt marks the run halted. Halt is sticky: later ticks exit immediately
// rather than rebuilding a broken main every schedule interval (spec §5).
// Time is passed in rather than read from the clock so callers — and tests —
// control it.
func SetHalt(st *State, stage, reason, sha, logPath string, now time.Time) {
	st.Status = StatusHalted
	st.Halted = &Halt{
		Stage:  stage,
		Reason: reason,
		SHA:    sha,
		Log:    logPath,
		At:     now.UTC(),
	}
}

// ClearHalt returns the run to idle, discarding the halt record. This happens
// on `autoship resume` or automatically when a new SHA lands (spec Q5).
func ClearHalt(st *State) {
	st.Status = StatusIdle
	st.Halted = nil
}

// IsHalted reports whether the pipeline is currently halted.
func (s State) IsHalted() bool { return s.Status == StatusHalted }

// Package pipeline orchestrates the S0–S6 stages.
package pipeline

import (
	"errors"

	"github.com/vipinm/autoship/internal/gitrepo"
	"github.com/vipinm/autoship/internal/state"
)

// Decision is what the S0 gate concluded about this tick.
type Decision int

const (
	// Proceed means main has moved and a release run should start.
	Proceed Decision = iota
	// SkipNoChange is the overwhelmingly common case: nothing to do.
	SkipNoChange
	// SkipTransient means the remote could not be reached; try again next tick.
	SkipTransient
	// SkipHalted means a previous run halted and the same SHA is still at the
	// tip, so rebuilding it would fail the same way (spec §5).
	SkipHalted
	// SkipError means the gate itself failed for a non-transient reason.
	SkipError
)

func (d Decision) String() string {
	switch d {
	case Proceed:
		return "proceed"
	case SkipNoChange:
		return "skip: no change"
	case SkipTransient:
		return "skip: remote unreachable"
	case SkipHalted:
		return "skip: halted"
	case SkipError:
		return "skip: gate error"
	default:
		return "unknown"
	}
}

// Decide is the whole of the S0 gate: given the persisted state, the SHA at the
// tip of the release branch, and whatever error resolving it produced, say what
// this tick should do.
//
// It is deliberately pure — no I/O, no clock — because this is the path that
// runs on ~99% of invocations and it is the one behaviour worth exhaustively
// table-testing.
func Decide(st state.State, head string, err error) Decision {
	if err != nil {
		if errors.Is(err, gitrepo.ErrTransient) {
			return SkipTransient
		}
		return SkipError
	}
	if head == "" {
		return SkipError
	}
	if st.IsHalted() {
		// A fix push is the natural "try again" signal, so a new SHA clears a
		// halt on its own (spec Q5). Without one, stay stopped: a broken main
		// must not trigger a multi-gigabyte Gradle build every tick, forever.
		//
		// The comparison is against the SHA the halt happened at, not the last
		// *processed* one — a run that halts never finishes processing its SHA,
		// so LastProcessedSHA would still name the previous release and every
		// tick would look like new work.
		if head != haltedSHA(st) {
			return Proceed
		}
		return SkipHalted
	}
	if head == st.LastProcessedSHA {
		return SkipNoChange
	}
	return Proceed
}

// haltedSHA is the commit a halt happened at, falling back to the last
// processed one for a state written before the halt recorded it.
func haltedSHA(st state.State) string {
	if st.Halted != nil && st.Halted.SHA != "" {
		return st.Halted.SHA
	}
	return st.LastProcessedSHA
}

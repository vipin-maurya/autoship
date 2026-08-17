package pipeline

import (
	"errors"
	"fmt"
	"testing"

	"github.com/vipinm/autoship/internal/gitrepo"
	"github.com/vipinm/autoship/internal/state"
)

func TestDecide(t *testing.T) {
	const oldSHA = "a1b2c3"
	const newSHA = "d4e5f6"

	idle := state.State{LastProcessedSHA: oldSHA, Status: state.StatusIdle}
	halted := state.State{LastProcessedSHA: oldSHA, Status: state.StatusHalted, Halted: &state.Halt{Stage: "S2"}}
	// A run that halts never finishes processing its SHA, so the halt record is
	// the only thing that knows which commit is broken.
	haltedMidRelease := state.State{Status: state.StatusHalted, Halted: &state.Halt{Stage: "S2", SHA: oldSHA}}

	tests := []struct {
		name string
		st   state.State
		head string
		err  error
		want Decision
	}{
		{"same sha", idle, oldSHA, nil, SkipNoChange},
		{"new sha", idle, newSHA, nil, Proceed},
		{"first ever run", state.State{Status: state.StatusIdle}, newSHA, nil, Proceed},
		{"unreachable remote", idle, "", fmt.Errorf("fetch: %w", gitrepo.ErrTransient), SkipTransient},
		{"other git failure", idle, "", errors.New("bad object"), SkipError},
		{"empty head with no error", idle, "", nil, SkipError},
		{"halted, same sha", halted, oldSHA, nil, SkipHalted},
		{"halted, new sha auto-clears", halted, newSHA, nil, Proceed},
		{"halted mid-release, same sha", haltedMidRelease, oldSHA, nil, SkipHalted},
		{"halted mid-release, new sha auto-clears", haltedMidRelease, newSHA, nil, Proceed},
		{"halted and offline stays quiet", halted, "", fmt.Errorf("%w: nope", gitrepo.ErrTransient), SkipTransient},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := Decide(tc.st, tc.head, tc.err); got != tc.want {
				t.Errorf("Decide = %v, want %v", got, tc.want)
			}
		})
	}
}

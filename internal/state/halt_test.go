package state

import (
	"testing"
	"time"
)

func TestSetHalt(t *testing.T) {
	now := time.Date(2026, 8, 17, 9, 19, 41, 0, time.UTC)
	st := State{Status: StatusRunning}
	SetHalt(&st, "S2", "unit tests failed: 3 failures", "a1b2c3", `C:\logs\run.log`, now)

	if !st.IsHalted() {
		t.Fatalf("Status = %q, want %q", st.Status, StatusHalted)
	}
	if st.Halted == nil {
		t.Fatal("Halted block is nil")
	}
	if st.Halted.Stage != "S2" {
		t.Errorf("Stage = %q, want S2", st.Halted.Stage)
	}
	if st.Halted.Reason != "unit tests failed: 3 failures" {
		t.Errorf("Reason = %q", st.Halted.Reason)
	}
	if st.Halted.SHA != "a1b2c3" {
		t.Errorf("SHA = %q", st.Halted.SHA)
	}
	if st.Halted.Log != `C:\logs\run.log` {
		t.Errorf("Log = %q", st.Halted.Log)
	}
	if !st.Halted.At.Equal(now) {
		t.Errorf("At = %v, want %v", st.Halted.At, now)
	}
}

func TestClearHalt(t *testing.T) {
	st := State{Status: StatusHalted, Halted: &Halt{Stage: "S2"}}
	ClearHalt(&st)
	if st.Status != StatusIdle {
		t.Errorf("Status = %q, want %q", st.Status, StatusIdle)
	}
	if st.Halted != nil {
		t.Errorf("Halted = %+v, want nil", st.Halted)
	}
}

func TestHalt_SurvivesRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s := Store{Dir: dir}
	st := State{}
	SetHalt(&st, "S5", "play rejected the bundle", "deadbee", "log.txt", time.Now())
	if err := s.Save(st); err != nil {
		t.Fatal(err)
	}
	got, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !got.IsHalted() || got.Halted == nil || got.Halted.Stage != "S5" {
		t.Errorf("round-tripped state = %+v, want a halted S5 state", got)
	}
}

package cli

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/vipinm/autoship/internal/state"
)

func (f *fixture) status() (int, string, string) {
	f.t.Helper()
	var out, errOut bytes.Buffer
	code := statusCmd([]string{"--config", f.ConfigPath}, &out, &errOut, f.deps())
	return code, out.String(), errOut.String()
}

func (f *fixture) resume() (int, string, string) {
	f.t.Helper()
	var out, errOut bytes.Buffer
	code := resumeCmd([]string{"--config", f.ConfigPath}, &out, &errOut, f.deps())
	return code, out.String(), errOut.String()
}

func TestStatus_ReportsHalt(t *testing.T) {
	fx := newFixture(t)
	st := state.State{LastProcessedSHA: "a1b2c3", LastPublishedVersionName: "1.0.4", LastPublishedVersionCode: 7}
	state.SetHalt(&st, "S2", "unit tests failed: 3 failures", "a1b2c3", `C:\logs\run-2026-08-17.log`, time.Now())
	fx.saveState(st)

	code, stdout, _ := fx.status()
	if code != 1 {
		t.Fatalf("exit code = %d, want 1 for a halted state", code)
	}
	for _, want := range []string{"S2", "unit tests failed: 3 failures", `C:\logs\run-2026-08-17.log`, "halted"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("status output missing %q:\n%s", want, stdout)
		}
	}
}

func TestStatus_ReportsIdle(t *testing.T) {
	fx := newFixture(t)
	fx.saveState(state.State{Status: state.StatusIdle, LastProcessedSHA: "a1b2c3", LastPublishedVersionName: "1.0.4", LastPublishedVersionCode: 7})

	code, stdout, _ := fx.status()
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if !strings.Contains(stdout, "1.0.4 (7)") {
		t.Errorf("status output missing the last published version:\n%s", stdout)
	}
	if strings.Contains(stdout, "HALTED") {
		t.Errorf("idle status reports a halt:\n%s", stdout)
	}
}

func TestStatus_FreshStateIsIdle(t *testing.T) {
	fx := newFixture(t)
	code, stdout, _ := fx.status()
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if !strings.Contains(stdout, "(none)") {
		t.Errorf("fresh status should report nothing published:\n%s", stdout)
	}
}

func TestResume_ClearsHalt(t *testing.T) {
	fx := newFixture(t)
	st := state.State{LastProcessedSHA: "a1b2c3"}
	state.SetHalt(&st, "S5", "play rejected the bundle", "a1b2c3", "log.txt", time.Now())
	fx.saveState(st)

	code, stdout, _ := fx.resume()
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if !strings.Contains(stdout, "S5") {
		t.Errorf("resume output = %q, want it to name the cleared stage", stdout)
	}

	got := fx.loadState()
	if got.Status != state.StatusIdle {
		t.Errorf("status = %q, want %q", got.Status, state.StatusIdle)
	}
	if got.Halted != nil {
		t.Errorf("halt block = %+v, want nil", got.Halted)
	}
	// The version history survives; only the halt is cleared.
	if got.LastProcessedSHA != "a1b2c3" {
		t.Errorf("LastProcessedSHA = %q, want it untouched", got.LastProcessedSHA)
	}
}

func TestResume_WhenNotHalted(t *testing.T) {
	fx := newFixture(t)
	fx.saveState(state.State{Status: state.StatusIdle})
	code, stdout, _ := fx.resume()
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if !strings.Contains(stdout, "nothing to resume") {
		t.Errorf("resume output = %q", stdout)
	}
}

func TestResume_ThenRunRetries(t *testing.T) {
	fx := newFixture(t)
	head := fx.headSHA()
	st := state.State{}
	state.SetHalt(&st, "S2", "unit tests failed", head, "log.txt", time.Now())
	fx.saveState(st)

	// While halted, the tick does nothing.
	if code, _, _ := fx.run(false); code != 0 || fx.Runner.Count() != 0 {
		t.Fatalf("halted tick ran: code=%d calls=%v", code, fx.Runner.ArgLines())
	}
	if code, _, _ := fx.resume(); code != 0 {
		t.Fatal("resume failed")
	}
	if code, _, stderr := fx.run(false); code != 0 {
		t.Fatalf("run after resume = %d (stderr: %s)", code, stderr)
	}
	if fx.Runner.Count() == 0 {
		t.Error("run after resume did not rebuild")
	}
}

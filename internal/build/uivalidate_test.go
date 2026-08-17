package build

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/vipinm/autoship/internal/config"
	"github.com/vipinm/autoship/internal/runner"
)

func TestUIValidation_ModeNone(t *testing.T) {
	spy := &runner.Spy{}
	st := UIValidateStage{Runner: spy, Dir: "d", Cfg: config.UIValidation{Mode: config.UIModeNone}}
	if err := st.Execute(context.Background()); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if spy.Count() != 0 {
		t.Errorf("ran %d commands in mode none, want 0", spy.Count())
	}
}

func TestUIValidation_ModeJVM(t *testing.T) {
	spy := &runner.Spy{}
	cfg := config.UIValidation{Mode: config.UIModeJVM, Task: ":app:testDebugUnitTest --tests '*ScreenRenderTest'"}
	if err := (UIValidateStage{Runner: spy, Dir: "d", Cfg: cfg}).Execute(context.Background()); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if spy.Count() != 1 {
		t.Fatalf("ran %d commands, want 1", spy.Count())
	}
	got := spy.Calls()[0]
	if got.Name != Wrapper() {
		t.Errorf("invoked %q, want %q", got.Name, Wrapper())
	}
	if len(got.Args) != 3 || got.Args[0] != ":app:testDebugUnitTest" || got.Args[2] != "*ScreenRenderTest" {
		t.Errorf("args = %#v, want the task with its --tests filter intact", got.Args)
	}
}

func TestUIValidation_HaltsOnFailure(t *testing.T) {
	spy := &runner.Spy{ExitFor: runner.FailOnArg("ScreenRenderTest", 1)}
	cfg := config.UIValidation{Mode: config.UIModeJVM, Task: ":app:testDebugUnitTest --tests '*ScreenRenderTest'"}
	err := (UIValidateStage{Runner: spy, Dir: "d", Cfg: cfg}).Execute(context.Background())
	if err == nil {
		t.Fatal("Execute = nil error, want a failure")
	}
	if !strings.Contains(err.Error(), StageUIValidation) {
		t.Errorf("error = %v, want it to name %s", err, StageUIValidation)
	}
}

func TestUIValidation_ModeJVMWithoutTask(t *testing.T) {
	spy := &runner.Spy{}
	err := (UIValidateStage{Runner: spy, Dir: "d", Cfg: config.UIValidation{Mode: config.UIModeJVM}}).Execute(context.Background())
	if err == nil {
		t.Fatal("Execute = nil error, want a configuration error")
	}
	if spy.Count() != 0 {
		t.Errorf("ran %d commands, want 0", spy.Count())
	}
}

func TestUIValidation_ModeEmulatorIsReserved(t *testing.T) {
	err := (UIValidateStage{Runner: &runner.Spy{}, Cfg: config.UIValidation{Mode: config.UIModeEmulator}}).Execute(context.Background())
	if !errors.Is(err, ErrNotImplemented) {
		t.Errorf("Execute = %v, want ErrNotImplemented", err)
	}
}

package build

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/vipinm/autoship/internal/config"
	"github.com/vipinm/autoship/internal/runner"
)

func testGradleCfg() config.Gradle {
	return config.Gradle{
		UnitTests: ":app:testDebugUnitTest",
		Lint:      ":app:lintRelease",
		Bundle:    ":app:bundleRelease",
	}
}

func TestGradleStage_RunsTasksInOrder(t *testing.T) {
	spy := &runner.Spy{}
	st := GradleStage{Runner: spy, Dir: `C:\repo`, Cfg: testGradleCfg()}

	if err := st.Execute(context.Background()); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	want := []string{":app:testDebugUnitTest", ":app:lintRelease", ":app:bundleRelease"}
	if got := spy.ArgLines(); !reflect.DeepEqual(got, want) {
		t.Errorf("tasks = %v, want %v", got, want)
	}
	for _, c := range spy.Calls() {
		if c.Dir != `C:\repo` {
			t.Errorf("task ran in %q, want the repo root", c.Dir)
		}
		if c.Name != Wrapper() {
			t.Errorf("invoked %q, want the gradle wrapper %q", c.Name, Wrapper())
		}
	}
}

func TestGradleStage_HaltsOnFailure(t *testing.T) {
	spy := &runner.Spy{ExitFor: runner.FailOnArg(":app:testDebugUnitTest", 1)}
	st := GradleStage{Runner: spy, Dir: `C:\repo`, Cfg: testGradleCfg()}

	err := st.Execute(context.Background())
	if err == nil {
		t.Fatal("Execute = nil error, want a failure")
	}
	if !strings.Contains(err.Error(), StageBuild) {
		t.Errorf("error = %v, want it to name %s", err, StageBuild)
	}
	if !strings.Contains(err.Error(), ":app:testDebugUnitTest") {
		t.Errorf("error = %v, want it to name the failing task", err)
	}
	// The expensive tasks must not run once a cheap gate has failed.
	if spy.Count() != 1 {
		t.Errorf("ran %d tasks after a failure, want 1: %v", spy.Count(), spy.ArgLines())
	}
}

func TestGradleStage_SkipsUnconfiguredLint(t *testing.T) {
	cfg := testGradleCfg()
	cfg.Lint = ""
	spy := &runner.Spy{}
	if err := (GradleStage{Runner: spy, Dir: "d", Cfg: cfg}).Execute(context.Background()); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	want := []string{":app:testDebugUnitTest", ":app:bundleRelease"}
	if got := spy.ArgLines(); !reflect.DeepEqual(got, want) {
		t.Errorf("tasks = %v, want %v", got, want)
	}
}

func TestGradleStage_AppendsExtraArgs(t *testing.T) {
	spy := &runner.Spy{}
	st := GradleStage{Runner: spy, Dir: "d", Cfg: testGradleCfg(), ExtraArgs: []string{"--no-daemon"}}
	if err := st.Execute(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, line := range spy.ArgLines() {
		if !strings.HasSuffix(line, "--no-daemon") {
			t.Errorf("call %q missing the extra args", line)
		}
	}
}

func TestWrapper(t *testing.T) {
	if got := wrapper("windows"); got != `.\gradlew.bat` {
		t.Errorf("wrapper(windows) = %q", got)
	}
	if got := wrapper("linux"); got != "./gradlew" {
		t.Errorf("wrapper(linux) = %q", got)
	}
}

func TestSplitArgs(t *testing.T) {
	tests := []struct {
		in   string
		want []string
	}{
		{":app:bundleRelease", []string{":app:bundleRelease"}},
		{":app:testDebugUnitTest --tests '*ScreenRenderTest'", []string{":app:testDebugUnitTest", "--tests", "*ScreenRenderTest"}},
		{`:app:test --tests "*Render Test"`, []string{":app:test", "--tests", "*Render Test"}},
		{"  spaced   out  ", []string{"spaced", "out"}},
		{"", nil},
	}
	for _, tc := range tests {
		if got := SplitArgs(tc.in); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("SplitArgs(%q) = %#v, want %#v", tc.in, got, tc.want)
		}
	}
}

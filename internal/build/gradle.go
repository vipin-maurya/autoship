// Package build drives Gradle: the unit tests, lint and bundle of S2, the UI
// validation of S3, and locating the artefact those produce.
package build

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"strings"

	"github.com/vipinm/autoship/internal/config"
	"github.com/vipinm/autoship/internal/runner"
)

// Stage labels used in error messages and halt records.
const (
	StageBuild        = "S2"
	StageUIValidation = "S3"
)

// wrapper returns the Gradle wrapper to invoke on the given GOOS. It takes the
// OS as an argument rather than reading runtime.GOOS so both branches are
// testable on either platform (plan A4).
func wrapper(goos string) string {
	if goos == "windows" {
		return `.\gradlew.bat`
	}
	return "./gradlew"
}

// Wrapper is the Gradle wrapper for the running platform.
func Wrapper() string { return wrapper(runtime.GOOS) }

// GradleStage runs the S2 tasks in order: unit tests, then lint, then the
// release bundle. Order matters — the cheap gates run before the expensive
// packaging step.
type GradleStage struct {
	Runner runner.Runner
	// Dir is the repository root, where the Gradle wrapper lives.
	Dir string
	Cfg config.Gradle
	// ExtraArgs is appended to every task invocation (e.g. --no-daemon).
	ExtraArgs []string
}

// task is one named Gradle invocation.
type task struct {
	label string
	spec  string
}

// Execute runs the stage, stopping at the first failing task.
func (s GradleStage) Execute(ctx context.Context) error {
	tasks := []task{
		{"unit tests", s.Cfg.UnitTests},
		{"lint", s.Cfg.Lint},
		{"bundle", s.Cfg.Bundle},
	}
	for _, t := range tasks {
		if strings.TrimSpace(t.spec) == "" {
			// lint is optional; a blank task means "not configured".
			continue
		}
		if err := s.runTask(ctx, t); err != nil {
			return err
		}
	}
	return nil
}

func (s GradleStage) runTask(ctx context.Context, t task) error {
	args := append(SplitArgs(t.spec), s.ExtraArgs...)
	code, err := s.Runner.Run(ctx, s.Dir, Wrapper(), args...)
	if err != nil || code != 0 {
		return fmt.Errorf("%s: %s task %q failed (exit %d): %w", StageBuild, t.label, t.spec, code, errOrExit(err, code))
	}
	return nil
}

func errOrExit(err error, code int) error {
	if err != nil {
		return err
	}
	return fmt.Errorf("exit %d", code)
}

// ErrNotImplemented reports a configured mode that exists in the schema but not
// yet in the code.
var ErrNotImplemented = errors.New("not implemented")

// SplitArgs splits a task specification into arguments, honouring single and
// double quotes so a spec like:
//
//	:app:testDebugUnitTest --tests '*ScreenRenderTest'
//
// survives as three arguments rather than four.
func SplitArgs(spec string) []string {
	var (
		args     []string
		cur      strings.Builder
		quote    rune
		hasToken bool
	)
	flush := func() {
		if hasToken {
			args = append(args, cur.String())
			cur.Reset()
			hasToken = false
		}
	}
	for _, r := range spec {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				cur.WriteRune(r)
			}
		case r == '\'' || r == '"':
			quote = r
			hasToken = true
		case r == ' ' || r == '\t':
			flush()
		default:
			cur.WriteRune(r)
			hasToken = true
		}
	}
	flush()
	return args
}

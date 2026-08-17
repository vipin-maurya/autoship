// Package runner executes external commands with streaming output and a
// deadline. Every stage takes a Runner rather than calling os/exec directly,
// which is what makes the build stages testable without a JVM.
package runner

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
)

// Runner runs a command in dir and returns its exit code.
type Runner interface {
	Run(ctx context.Context, dir, name string, args ...string) (int, error)
}

// ExecRunner runs commands for real, streaming merged stdout/stderr to Out.
type ExecRunner struct {
	// Out receives the command's output as it arrives. A nil Out discards it.
	Out io.Writer
	// Env, when non-nil, replaces the child's environment entirely.
	Env []string
}

// Run executes name with args in dir. It returns the exit code alongside the
// error, so a caller can distinguish "the command failed" (exit 1) from "the
// command could not be started" (exit -1).
func (r ExecRunner) Run(ctx context.Context, dir, name string, args ...string) (int, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	if r.Env != nil {
		cmd.Env = r.Env
	}
	out := r.Out
	if out == nil {
		out = io.Discard
	}
	cmd.Stdout = out
	cmd.Stderr = out

	err := cmd.Run()
	if err == nil {
		return 0, nil
	}
	// A cancelled context is the reason for the failure, and callers test for
	// it with errors.Is, so it has to survive the wrapping.
	if ctxErr := ctx.Err(); ctxErr != nil {
		return -1, fmt.Errorf("%s: %w", name, ctxErr)
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode(), fmt.Errorf("%s exited %d", name, ee.ExitCode())
	}
	return -1, fmt.Errorf("run %s: %w", name, err)
}

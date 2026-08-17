// Package gitrepo drives the installed git binary. Shelling out is deliberate:
// the machine already has a configured git with the user's credentials, and an
// in-process implementation would reimplement it at a cost in binary size for
// no behavioural gain (plan A5).
package gitrepo

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// ErrTransient marks a failure that is expected to fix itself — offline, VPN
// down, remote briefly unreachable. The S0 gate exits 0 on these rather than
// halting (spec §10).
var ErrTransient = errors.New("transient git failure")

// ErrPushRejected reports that the remote refused the push because it has
// moved on. Recovering from this is S6's rebase-and-retry (spec §11).
var ErrPushRejected = errors.New("push rejected: remote has moved")

// Git is a command failure carrying git's own diagnostics.
type Git struct {
	Args   []string
	Code   int
	Output string
}

func (e *Git) Error() string {
	return fmt.Sprintf("git %s: exit %d: %s", strings.Join(e.Args, " "), e.Code, strings.TrimSpace(e.Output))
}

// exec runs git in dir and returns its trimmed combined output.
func run(dir string, args ...string) (string, error) {
	full := append([]string{"-C", dir}, args...)
	cmd := exec.Command("git", full...)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	out := buf.String()
	if err != nil {
		code := -1
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			code = ee.ExitCode()
		}
		return strings.TrimSpace(out), &Git{Args: args, Code: code, Output: out}
	}
	return strings.TrimSpace(out), nil
}

// Fetch updates the remote-tracking ref for branch. Failures are transient by
// default — the network is the only thing fetch depends on — except a branch
// the remote does not have, which no amount of retrying will conjure up and
// which would otherwise make every tick a silent no-op forever.
func Fetch(dir, remote, branch string) error {
	out, err := run(dir, "fetch", remote, branch, "--quiet")
	if err == nil {
		return nil
	}
	if strings.Contains(strings.ToLower(out), "couldn't find remote ref") {
		return fmt.Errorf("remote %s has no branch %s: %w", remote, branch, err)
	}
	return fmt.Errorf("%w: %w", ErrTransient, err)
}

// ResolveRemoteHead fetches and returns the SHA at the tip of remote/branch.
// This plus a string comparison is the whole of the S0 gate (spec §3).
func ResolveRemoteHead(dir, remote, branch string) (string, error) {
	if err := Fetch(dir, remote, branch); err != nil {
		return "", err
	}
	// rev-parse failures are configuration errors — a branch that does not
	// exist will not start existing on the next tick — so they are not
	// wrapped as transient.
	sha, err := run(dir, "rev-parse", remote+"/"+branch)
	if err != nil {
		return "", fmt.Errorf("resolve %s/%s: %w", remote, branch, err)
	}
	return sha, nil
}

// HeadSHA returns the SHA of the local working tree's HEAD.
func HeadSHA(dir string) (string, error) {
	return run(dir, "rev-parse", "HEAD")
}

// Checkout switches the working tree to branch and fast-forwards it onto the
// remote tip, so the release is built from exactly what the gate saw.
func Checkout(dir, remote, branch string) error {
	if _, err := run(dir, "checkout", branch); err != nil {
		return fmt.Errorf("checkout %s: %w", branch, err)
	}
	if _, err := run(dir, "merge", "--ff-only", remote+"/"+branch); err != nil {
		return fmt.Errorf("fast-forward %s onto %s/%s: %w", branch, remote, branch, err)
	}
	return nil
}

// LastTag returns the most recent tag reachable from HEAD, or "" when the repo
// has none yet.
func LastTag(dir string) (string, error) {
	out, err := run(dir, "describe", "--tags", "--abbrev=0")
	if err != nil {
		// "no names found" is a legitimate answer for a repo before its first
		// release, not a failure.
		if strings.Contains(strings.ToLower(out), "no names found") ||
			strings.Contains(strings.ToLower(out), "no tags can describe") {
			return "", nil
		}
		return "", err
	}
	return out, nil
}

// Subjects returns the commit subjects in revRange (e.g. "v1.0.4..HEAD"), most
// recent first. An empty revRange means the whole history.
func Subjects(dir, revRange string) ([]string, error) {
	args := []string{"log", "--pretty=%s"}
	if revRange != "" {
		args = append(args, revRange)
	}
	out, err := run(dir, args...)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(out) == "" {
		return nil, nil
	}
	lines := strings.Split(strings.ReplaceAll(out, "\r\n", "\n"), "\n")
	subjects := make([]string, 0, len(lines))
	for _, l := range lines {
		if s := strings.TrimSpace(l); s != "" {
			subjects = append(subjects, s)
		}
	}
	return subjects, nil
}

// Tag creates an annotated tag at HEAD.
func Tag(dir, name, message string) error {
	_, err := run(dir, "tag", "-a", name, "-m", message)
	return err
}

// CommitAll stages every tracked modification and commits it.
func CommitAll(dir, message string) error {
	_, err := run(dir, "commit", "-a", "-m", message)
	return err
}

// IsClean reports whether the working tree has no uncommitted changes.
func IsClean(dir string) (bool, error) {
	out, err := run(dir, "status", "--porcelain")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) == "", nil
}

// Push publishes branch and, with followTags, any annotated tags on it.
// A rejection because the remote moved is reported as ErrPushRejected so the
// caller can rebase and retry rather than halt blindly (spec §11).
func Push(dir, remote, branch string, followTags bool) error {
	args := []string{"push", remote, branch}
	if followTags {
		args = append(args, "--follow-tags")
	}
	out, err := run(dir, args...)
	if err == nil {
		return nil
	}
	lower := strings.ToLower(out)
	if strings.Contains(lower, "rejected") || strings.Contains(lower, "non-fast-forward") || strings.Contains(lower, "fetch first") {
		return fmt.Errorf("%w: %w", ErrPushRejected, err)
	}
	return err
}

// PullRebase replays local commits on top of the remote tip. A rebase that
// stops on a conflict is aborted, so the working tree is left back on the
// commit that still needs pushing rather than mid-rebase — recovery should be
// a plain `git push`, not an archaeology exercise.
func PullRebase(dir, remote, branch string) error {
	_, err := run(dir, "pull", "--rebase", remote, branch)
	if err != nil {
		if _, abortErr := run(dir, "rebase", "--abort"); abortErr != nil {
			// No rebase was in progress, or it could not be undone; the
			// original failure is what matters either way.
			return err
		}
	}
	return err
}

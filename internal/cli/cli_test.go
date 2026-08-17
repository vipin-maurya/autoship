package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestRun_VersionSubcommand(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := Run([]string{"version"}, &out, &errOut); code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if !strings.Contains(out.String(), "autoship") {
		t.Errorf("stdout = %q, want it to contain %q", out.String(), "autoship")
	}
}

func TestRun_UnknownSubcommand(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := Run([]string{"frobnicate"}, &out, &errOut); code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if !strings.Contains(errOut.String(), "frobnicate") {
		t.Errorf("stderr = %q, want it to name the unknown subcommand", errOut.String())
	}
}

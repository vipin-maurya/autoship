package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/vipinm/autoship/internal/secrets"
)

func runSecrets(t *testing.T, root string, args ...string) (int, string, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code := secretsCmd(args, &out, &errOut, deps{Root: root})
	return code, out.String(), errOut.String()
}

func TestSecrets_SetListDelete(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("DPAPI-backed secrets are windows only")
	}
	root := t.TempDir()
	keyFile := filepath.Join(t.TempDir(), "sa.json")
	if err := os.WriteFile(keyFile, []byte(`{"type":"service_account"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	if code, _, stderr := runSecrets(t, root, "set", "--from-file", keyFile, secrets.PlayServiceAccount); code != 0 {
		t.Fatalf("secrets set = %d (stderr: %s)", code, stderr)
	}
	code, stdout, _ := runSecrets(t, root, "list")
	if code != 0 || !strings.Contains(stdout, secrets.PlayServiceAccount) {
		t.Fatalf("secrets list = %d, %q", code, stdout)
	}

	got, err := (secrets.Store{Dir: SecretsDir(root)}).Get(secrets.PlayServiceAccount)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(got) != `{"type":"service_account"}` {
		t.Errorf("stored secret = %q", got)
	}

	if code, _, stderr := runSecrets(t, root, "delete", secrets.PlayServiceAccount); code != 0 {
		t.Fatalf("secrets delete = %d (stderr: %s)", code, stderr)
	}
	if (secrets.Store{Dir: SecretsDir(root)}).Has(secrets.PlayServiceAccount) {
		t.Error("secret survived delete")
	}
}

func TestSecrets_UsageErrors(t *testing.T) {
	root := t.TempDir()
	for _, args := range [][]string{{}, {"frobnicate"}, {"set"}, {"delete"}} {
		if code, _, _ := runSecrets(t, root, args...); code != 2 {
			t.Errorf("secrets %v = %d, want 2", args, code)
		}
	}
}

func TestSecrets_NeverTakesTheValueAsAnArgument(t *testing.T) {
	// A secret passed as a flag lands in the shell history and the process
	// list, so `set` accepts a name and nothing else.
	root := t.TempDir()
	code, _, stderr := runSecrets(t, root, "set", "play_sa", "hunter2")
	if code != 2 {
		t.Errorf("secrets set with an inline value = %d, want a usage error", code)
	}
	if !strings.Contains(stderr, "usage") {
		t.Errorf("stderr = %q, want usage", stderr)
	}
}

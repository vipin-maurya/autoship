package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/vipinm/autoship/internal/config"
	"github.com/vipinm/autoship/internal/play"
	"github.com/vipinm/autoship/internal/runner"
	"github.com/vipinm/autoship/internal/secrets"
	"github.com/vipinm/autoship/internal/state"
)

// DefaultConfigPath is where the run commands look when --config is absent.
const DefaultConfigPath = "autoship.yaml"

// deps are the seams the CLI resolves at startup. Tests replace them so a run
// can be driven end to end without a JVM, a network, or a Play account.
type deps struct {
	// Root overrides the state root (default %LOCALAPPDATA%\autoship).
	Root string
	// NewRunner builds the command runner for the build stages.
	NewRunner func(out io.Writer) runner.Runner
	// NewPlayClient builds the publisher client.
	NewPlayClient func(ctx context.Context, cfg *config.Config, root string) (play.EditClient, error)
}

// defaultDeps is what a real invocation uses.
var defaultDeps = deps{
	NewRunner: func(out io.Writer) runner.Runner { return runner.ExecRunner{Out: out} },
	NewPlayClient: func(ctx context.Context, cfg *config.Config, root string) (play.EditClient, error) {
		saJSON, err := secrets.Store{Dir: SecretsDir(root)}.Get(secrets.PlayServiceAccount)
		if err != nil {
			return nil, fmt.Errorf("play credentials: %w (run `autoship secrets set %s`)", err, secrets.PlayServiceAccount)
		}
		return play.NewClient(ctx, saJSON, cfg.App.Package)
	},
}

// DefaultRoot is where autoship keeps state, logs and secrets: the OS user
// cache dir plus "autoship" — %LOCALAPPDATA%\autoship on Windows,
// ~/Library/Caches/autoship on macOS, ~/.cache/autoship on Linux.
func DefaultRoot() string {
	if dir, err := os.UserCacheDir(); err == nil {
		return filepath.Join(dir, "autoship")
	}
	return filepath.Join(".", ".autoship")
}

// SecretsDir is shared across repos, since the credentials are per user.
func SecretsDir(root string) string { return filepath.Join(root, "secrets") }

// rootOr returns the configured root, falling back to the default.
func (d deps) rootOr() string {
	if d.Root != "" {
		return d.Root
	}
	return DefaultRoot()
}

// storeFor opens the per-repo state store.
func (d deps) storeFor(cfg *config.Config) state.Store {
	return state.Store{Dir: state.DirFor(d.rootOr(), cfg.Repo.Path)}
}

// loadConfig parses the shared --config flag and loads the file.
func loadConfig(fs *flag.FlagSet, args []string, stderr io.Writer) (*config.Config, string, error) {
	path := fs.String("config", DefaultConfigPath, "path to autoship.yaml")
	fs.SetOutput(stderr)
	if err := fs.Parse(args); err != nil {
		return nil, "", err
	}
	cfg, err := config.Load(*path)
	if err != nil {
		return nil, *path, err
	}
	return cfg, *path, nil
}

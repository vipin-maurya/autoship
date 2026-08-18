package cli

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/vipinm/autoship/internal/secrets"
	"golang.org/x/term"
)

func init() {
	register("secrets", func(args []string, stdout, stderr io.Writer) int {
		return secretsCmd(args, stdout, stderr, defaultDeps)
	})
}

const secretsUsage = `usage: autoship secrets <set|list|delete> [name] [flags]

  set <name>      read a secret from stdin (or prompt on a terminal) and store it encrypted
  list            list the stored secret names
  delete <name>   remove a stored secret

  --from-file <path>   with set: read the secret from a file (used for the
                       Play service-account JSON)

Secrets are encrypted at rest with the OS's native secret store — DPAPI on
Windows, Keychain on macOS, the Secret Service (secret-tool) on Linux — and
are never passed as command-line arguments — an argument would land in the
shell history and in the process list.

Well-known names: ` + secrets.PlayServiceAccount + `, ` + secrets.KeystorePassword + `, ` + secrets.KeyAliasPassword + `
`

func secretsCmd(args []string, stdout, stderr io.Writer, d deps) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, secretsUsage)
		return exitUsage
	}
	store := secrets.Store{Dir: SecretsDir(d.rootOr())}

	switch args[0] {
	case "list":
		names, err := store.List()
		if err != nil {
			fmt.Fprintf(stderr, "autoship secrets: %v\n", err)
			return exitHalt
		}
		if len(names) == 0 {
			fmt.Fprintf(stdout, "no secrets stored in %s\n", store.Dir)
			return exitOK
		}
		for _, n := range names {
			fmt.Fprintln(stdout, n)
		}
		return exitOK

	case "set":
		fs := flag.NewFlagSet("secrets set", flag.ContinueOnError)
		fs.SetOutput(stderr)
		fromFile := fs.String("from-file", "", "read the secret from this file")
		if err := fs.Parse(args[1:]); err != nil {
			return exitUsage
		}
		if fs.NArg() != 1 {
			fmt.Fprint(stderr, secretsUsage)
			return exitUsage
		}
		name := fs.Arg(0)

		value, err := readSecret(*fromFile, stdout)
		if err != nil {
			fmt.Fprintf(stderr, "autoship secrets: %v\n", err)
			return exitHalt
		}
		if err := store.Set(name, value); err != nil {
			if errors.Is(err, secrets.ErrUnsupported) {
				fmt.Fprintf(stderr, "autoship secrets: %v\n", err)
				return exitHalt
			}
			fmt.Fprintf(stderr, "autoship secrets: %v\n", err)
			return exitHalt
		}
		fmt.Fprintf(stdout, "stored %s (encrypted, %s)\n", name, store.Dir)
		return exitOK

	case "delete":
		if len(args) != 2 {
			fmt.Fprint(stderr, secretsUsage)
			return exitUsage
		}
		if err := store.Delete(args[1]); err != nil {
			fmt.Fprintf(stderr, "autoship secrets: %v\n", err)
			return exitHalt
		}
		fmt.Fprintf(stdout, "deleted %s\n", args[1])
		return exitOK

	default:
		fmt.Fprint(stderr, secretsUsage)
		return exitUsage
	}
}

// readSecret takes the value from a file, from an interactive prompt with echo
// off, or from piped stdin — never from a flag.
func readSecret(fromFile string, stdout io.Writer) ([]byte, error) {
	if fromFile != "" {
		raw, err := os.ReadFile(fromFile)
		if err != nil {
			return nil, fmt.Errorf("read secret file: %w", err)
		}
		if len(strings.TrimSpace(string(raw))) == 0 {
			return nil, errors.New("secret file is empty")
		}
		return raw, nil
	}

	fd := int(os.Stdin.Fd())
	if term.IsTerminal(fd) {
		fmt.Fprint(stdout, "secret (input hidden): ")
		raw, err := term.ReadPassword(fd)
		fmt.Fprintln(stdout)
		if err != nil {
			return nil, fmt.Errorf("read secret: %w", err)
		}
		if len(strings.TrimSpace(string(raw))) == 0 {
			return nil, errors.New("secret is empty")
		}
		return raw, nil
	}

	raw, err := io.ReadAll(bufio.NewReader(os.Stdin))
	if err != nil {
		return nil, fmt.Errorf("read secret from stdin: %w", err)
	}
	trimmed := strings.TrimRight(string(raw), "\r\n")
	if strings.TrimSpace(trimmed) == "" {
		return nil, errors.New("secret is empty")
	}
	return []byte(trimmed), nil
}

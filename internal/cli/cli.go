// Package cli dispatches autoship subcommands.
package cli

import (
	"fmt"
	"io"
)

// Version is the autoship binary version.
const Version = "0.1.0"

const usage = `autoship - unattended Android release pipeline

usage: autoship <command> [flags]

commands:
  run           poll the repo and release if main moved
  dry-run       same as run, but never commits the Play edit or pushes
  status        report the current run state
  resume        clear a halt so the next run retries
  secrets       manage locally encrypted credentials
  draft-notes   draft customer release notes from the commit log
  version       print the autoship version
`

// command is one subcommand implementation.
type command func(args []string, stdout, stderr io.Writer) int

// commands is populated by each subcommand file's init, so adding a
// subcommand never means editing this dispatcher.
var commands = map[string]command{}

func register(name string, fn command) {
	if _, dup := commands[name]; dup {
		panic("cli: duplicate subcommand " + name)
	}
	commands[name] = fn
}

// Run dispatches args (os.Args[1:]) and returns the process exit code.
func Run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	switch args[0] {
	case "version", "--version", "-v":
		fmt.Fprintf(stdout, "autoship %s\n", Version)
		return 0
	case "help", "--help", "-h":
		fmt.Fprint(stdout, usage)
		return 0
	}
	if cmd, ok := commands[args[0]]; ok {
		return cmd(args[1:], stdout, stderr)
	}
	fmt.Fprintf(stderr, "unknown subcommand %q\n\n%s", args[0], usage)
	return 2
}

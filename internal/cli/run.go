package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/vipinm/autoship/internal/logging"
	"github.com/vipinm/autoship/internal/pipeline"
	"github.com/vipinm/autoship/internal/play"
	"github.com/vipinm/autoship/internal/state"
)

func init() {
	register("run", func(args []string, stdout, stderr io.Writer) int {
		return runCmd(args, stdout, stderr, false, defaultDeps)
	})
	register("dry-run", func(args []string, stdout, stderr io.Writer) int {
		return runCmd(args, stdout, stderr, true, defaultDeps)
	})
}

// Exit codes. Task Scheduler's history is one of the notification channels
// (spec §10), so these are part of the interface.
const (
	exitOK    = 0
	exitHalt  = 1
	exitUsage = 2
)

func runCmd(args []string, stdout, stderr io.Writer, dryRun bool, d deps) int {
	name := "run"
	if dryRun {
		name = "dry-run"
	}
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	quiet := fs.Bool("quiet", false, "do not echo the run log to stdout")
	timeout := fs.Duration("timeout", 0, "abort the run after this long (default: max_run_duration)")

	cfg, path, err := loadConfig(fs, args, stderr)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitUsage
		}
		fmt.Fprintf(stderr, "autoship %s: %v\n", name, err)
		return exitUsage
	}

	store := d.storeFor(cfg)

	// Overlapping invocations are expected, not exceptional: a build can
	// outlive the schedule interval, and the second process simply leaves
	// (spec §5).
	lock, err := store.Acquire(cfg.MaxRunDur)
	if errors.Is(err, state.ErrLocked) {
		return exitOK
	}
	if err != nil {
		fmt.Fprintf(stderr, "autoship %s: %v\n", name, err)
		return exitHalt
	}
	defer lock.Release()

	var tee io.Writer
	if !*quiet {
		tee = stdout
	}
	log, err := logging.New(store.LogsDir(), logging.RunID(time.Now()), tee)
	if err != nil {
		fmt.Fprintf(stderr, "autoship %s: %v\n", name, err)
		return exitHalt
	}
	defer log.Close()
	log.Info("autoship starting", "version", Version, "config", path, "repo", cfg.Repo.Path, "dryRun", dryRun)

	limit := cfg.MaxRunDur
	if *timeout > 0 {
		limit = *timeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), limit)
	defer cancel()

	newRunner := d.NewRunner
	if newRunner == nil {
		newRunner = defaultDeps.NewRunner
	}
	newClient := d.NewPlayClient
	if newClient == nil {
		newClient = defaultDeps.NewPlayClient
	}
	root := d.rootOr()

	res, err := pipeline.Execute(ctx, pipeline.Deps{
		Cfg:     cfg,
		Store:   store,
		Log:     log.Logger,
		LogPath: log.Path,
		Runner:  newRunner(log.Writer()),
		PlayClient: func(ctx context.Context) (play.EditClient, error) {
			return newClient(ctx, cfg, root)
		},
		DryRun: dryRun,
	})
	if err != nil {
		fmt.Fprintf(stderr, "autoship %s: %v\n", name, err)
		if res.Halt != nil {
			fmt.Fprintf(stderr, "halted at %s; see %s\n", res.Halt.Stage, res.Halt.Log)
		}
		return exitHalt
	}
	return exitOK
}

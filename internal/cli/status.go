package cli

import (
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/vipinm/autoship/internal/state"
)

func init() {
	register("status", func(args []string, stdout, stderr io.Writer) int {
		return statusCmd(args, stdout, stderr, defaultDeps)
	})
	register("resume", func(args []string, stdout, stderr io.Writer) int {
		return resumeCmd(args, stdout, stderr, defaultDeps)
	})
}

func statusCmd(args []string, stdout, stderr io.Writer, d deps) int {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	cfg, _, err := loadConfig(fs, args, stderr)
	if err != nil {
		fmt.Fprintf(stderr, "autoship status: %v\n", err)
		return exitUsage
	}
	store := d.storeFor(cfg)
	st, err := store.Load()
	if err != nil {
		fmt.Fprintf(stderr, "autoship status: %v\n", err)
		return exitHalt
	}

	fmt.Fprintf(stdout, "repo:            %s\n", cfg.Repo.Path)
	fmt.Fprintf(stdout, "branch:          %s/%s\n", cfg.Repo.Remote, cfg.Repo.Branch)
	fmt.Fprintf(stdout, "state:           %s\n", store.Dir)
	fmt.Fprintf(stdout, "status:          %s\n", st.Status)
	fmt.Fprintf(stdout, "last processed:  %s\n", orNone(st.LastProcessedSHA))
	fmt.Fprintf(stdout, "last published:  %s\n", publishedDesc(st))
	fmt.Fprintf(stdout, "last run:        %s\n", timeOrNever(st.LastRunAt))

	if st.Halted != nil {
		fmt.Fprintf(stdout, "\nHALTED at %s (%s)\n", st.Halted.Stage, timeOrNever(st.Halted.At))
		fmt.Fprintf(stdout, "  reason: %s\n", st.Halted.Reason)
		fmt.Fprintf(stdout, "  sha:    %s\n", orNone(st.Halted.SHA))
		fmt.Fprintf(stdout, "  log:    %s\n", orNone(st.Halted.Log))
		fmt.Fprintf(stdout, "\nA new commit on %s clears this automatically; `autoship resume` clears it now.\n", cfg.Repo.Branch)
		return exitHalt
	}
	return exitOK
}

func resumeCmd(args []string, stdout, stderr io.Writer, d deps) int {
	fs := flag.NewFlagSet("resume", flag.ContinueOnError)
	cfg, _, err := loadConfig(fs, args, stderr)
	if err != nil {
		fmt.Fprintf(stderr, "autoship resume: %v\n", err)
		return exitUsage
	}
	store := d.storeFor(cfg)
	st, err := store.Load()
	if err != nil {
		fmt.Fprintf(stderr, "autoship resume: %v\n", err)
		return exitHalt
	}
	if !st.IsHalted() {
		fmt.Fprintf(stdout, "not halted; nothing to resume (status: %s)\n", st.Status)
		return exitOK
	}
	stage := st.Halted.Stage
	state.ClearHalt(&st)
	if err := store.Save(st); err != nil {
		fmt.Fprintf(stderr, "autoship resume: %v\n", err)
		return exitHalt
	}
	fmt.Fprintf(stdout, "cleared the halt at %s; the next run will retry\n", stage)
	return exitOK
}

func orNone(s string) string {
	if s == "" {
		return "(none)"
	}
	return s
}

func timeOrNever(t time.Time) string {
	if t.IsZero() {
		return "(never)"
	}
	return t.UTC().Format(time.RFC3339)
}

func publishedDesc(st state.State) string {
	if st.LastPublishedVersionName == "" && st.LastPublishedVersionCode == 0 {
		return "(none)"
	}
	return fmt.Sprintf("%s (%d)", orNone(st.LastPublishedVersionName), st.LastPublishedVersionCode)
}

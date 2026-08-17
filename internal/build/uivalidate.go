package build

import (
	"context"
	"fmt"
	"strings"

	"github.com/vipinm/autoship/internal/config"
	"github.com/vipinm/autoship/internal/runner"
)

// UIValidateStage is S3. In jvm mode it runs one more Gradle task inside the
// JVM S2 already warmed, which is the whole reason that mode is the default:
// it catches "a screen no longer renders" for no extra process (spec §6).
type UIValidateStage struct {
	Runner    runner.Runner
	Dir       string
	Cfg       config.UIValidation
	ExtraArgs []string
}

// Execute runs the configured validation, if any.
func (s UIValidateStage) Execute(ctx context.Context) error {
	switch s.Cfg.Mode {
	case config.UIModeNone, "":
		return nil
	case config.UIModeJVM:
		if strings.TrimSpace(s.Cfg.Task) == "" {
			return fmt.Errorf("%s: ui_validation.mode is %q but no task is configured", StageUIValidation, config.UIModeJVM)
		}
		args := append(SplitArgs(s.Cfg.Task), s.ExtraArgs...)
		code, err := s.Runner.Run(ctx, s.Dir, Wrapper(), args...)
		if err != nil || code != 0 {
			return fmt.Errorf("%s: ui validation task %q failed (exit %d): %w",
				StageUIValidation, s.Cfg.Task, code, errOrExit(err, code))
		}
		return nil
	case config.UIModeEmulator:
		// The config key is reserved so this can be revisited without a
		// redesign, but a managed device costs +2.5 GB and is not v1 (spec §6).
		return fmt.Errorf("%s: ui_validation.mode %q: %w", StageUIValidation, config.UIModeEmulator, ErrNotImplemented)
	default:
		return fmt.Errorf("%s: unknown ui_validation.mode %q", StageUIValidation, s.Cfg.Mode)
	}
}

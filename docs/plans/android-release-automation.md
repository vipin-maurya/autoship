# Plan — Android Release Automation (`autoship`)

**Spec:** [android-release-automation.md](../specs/android-release-automation.md)
**Date:** 2026-08-17

> **Greenfield.** There is no existing code to plan against — `autoship` is a new Go module. The
> only pre-existing artefacts this plan reads from are the spec and the manual `releasing-app`
> skill, both already reviewed. Task file paths below therefore *create* structure rather than
> extend it.

## Assumptions (O2)

| # | Assumption | Change cost if wrong |
|---|---|---|
| A1 | Module path `github.com/vipinm/autoship` | One `go.mod` line + import rewrite; do it before Task 2 |
| A2 | Stdlib `flag` for subcommands, not cobra | Low. Chosen for spec §9: cobra adds ~2 MB and measurable init cost to a binary whose hot path is a 300 ms no-op |
| A3 | `gopkg.in/yaml.v3` is the only non-Google dependency | Low |
| A4 | Built and run on Windows; non-Windows kept compiling via build tags so tests run anywhere | Low if honoured from Task 5 onward, high if retrofitted |
| A5 | Git driven by `exec`-ing the installed `git`, not `go-git` | Low. `go-git` adds ~12 MB and reimplements what is already on the machine |

## Package layout

```
autoship/
├── cmd/autoship/main.go
└── internal/
    ├── cli/         subcommand dispatch
    ├── config/      autoship.yaml
    ├── state/       state.json, lock, halt
    ├── logging/     per-run log
    ├── gitrepo/     fetch, rev-parse, tag, push
    ├── gradlefile/  build.gradle.kts read + write-back
    ├── release/     semver classification
    ├── runner/      exec with streaming + timeout
    ├── build/       gradle stages, bundle discovery
    ├── artifacts/   versioned folder assembly
    ├── notes/       NotesProvider chain
    ├── play/        Play Publisher edit lifecycle
    ├── secrets/     DPAPI credential store
    └── pipeline/    S0–S6 orchestration
```

---

## Batch 1 — Skeleton, config, state, lock

### Task 1 — Module scaffold and CLI entrypoint
- **Files:** `go.mod`, `cmd/autoship/main.go`, `internal/cli/cli.go`, `internal/cli/cli_test.go`
- **Test first:** `TestRun_VersionSubcommand` — `cli.Run([]string{"version"}, &buf)` returns `0` and `buf` contains `autoship`.
- **Implement:** `go mod init github.com/vipinm/autoship` (go 1.23). `cli.Run(args []string, stdout io.Writer) int` switches on `args[0]` over `version|run|status|resume|dry-run|secrets|draft-notes`; unknown subcommand returns `2` with usage. `main.go` is `os.Exit(cli.Run(os.Args[1:], os.Stdout))`.
- **Proves done:** `go test ./internal/cli/ -run TestRun_VersionSubcommand` → `ok`
- **Depends on:** —
- **Commit:** `chore(autoship): scaffold go module and cli entrypoint`

### Task 2 — Load `autoship.yaml`
- **Files:** `internal/config/config.go`, `internal/config/config_test.go`, `internal/config/testdata/valid.yaml`
- **Test first:** `TestLoad_ValidFile` — loads `testdata/valid.yaml`, asserts `cfg.Repo.Branch == "main"`, `cfg.Gradle.Bundle == ":app:bundleRelease"`, `cfg.Play.Track == "alpha"`, `cfg.Notes.Source == []string{"file"}`.
- **Implement:** Structs mirroring spec §4 with `yaml` tags. `Load(path string) (*Config, error)`. `testdata/valid.yaml` is a verbatim copy of the spec §4 block.
- **Proves done:** `go test ./internal/config/ -run TestLoad_ValidFile` → `ok`
- **Depends on:** 1
- **Commit:** `feat(config): load autoship.yaml`

### Task 3 — Env expansion and validation
- **Files:** `internal/config/config.go`, `internal/config/config_test.go`
- **Test first:** `TestLoad_ExpandsEnv` — with `USERPROFILE=C:\Users\test`, `artifacts.root` resolves with no `${` remaining. `TestValidate_RejectsMissingFields` — empty `repo.path` returns an error whose message contains `repo.path`; empty `play.track` likewise.
- **Implement:** `os.ExpandEnv` over `Repo.Path`, `Artifacts.Root`, `Notes.FilePath`. `(*Config).Validate() error` returning a field-named error for each required-but-empty field. `Load` calls `Validate` before returning.
- **Proves done:** `go test ./internal/config/` → `ok`
- **Depends on:** 2
- **Commit:** `feat(config): expand env vars and validate required fields`

### Task 4 — Atomic state store
- **Files:** `internal/state/state.go`, `internal/state/state_test.go`
- **Test first:** `TestStore_RoundTrip` — `Load` on an empty dir returns a zero `State` and nil error; `Save` then `Load` round-trips `LastProcessedSHA` and `LastPublishedVersionCode`. `TestStore_SaveIsAtomic` — after `Save`, no `*.tmp` remains in the dir.
- **Implement:** `State` struct per spec §5. `Store{Dir string}` with `Load() (State, error)` and `Save(State) error`; `Save` writes `state.json.tmp` then `os.Rename`.
- **Proves done:** `go test ./internal/state/ -run TestStore` → `ok`
- **Depends on:** 1
- **Commit:** `feat(state): atomic run-state persistence`

### Task 5 — PID-aware lock with stale reclaim
- **Files:** `internal/state/lock.go`, `internal/state/alive_windows.go`, `internal/state/alive_other.go`, `internal/state/lock_test.go`
- **Test first:** `TestLock_SecondAcquireReturnsErrLocked` — acquire, then acquire again → `errors.Is(err, ErrLocked)`. `TestLock_ReclaimsStale` — hand-write a lock file with a dead PID and `StartedAt` older than `maxRunDuration`; acquire succeeds.
- **Implement:** `lock` file holding `{pid, started_at}`. `Acquire(maxRunDuration time.Duration) (*Lock, error)`, `(*Lock).Release()`. `processAlive(pid int) bool` behind build tags — `OpenProcess` on Windows, `Signal(0)` elsewhere (A4).
- **Proves done:** `go test ./internal/state/` → `ok`
- **Depends on:** 4
- **Commit:** `feat(state): pid-aware run lock with stale reclaim`

### Task 6 — Wire `run` to config, state and lock
- **Files:** `internal/cli/run.go`, `internal/cli/run_test.go`
- **Test first:** `TestRun_ExitsZeroWhenAlreadyLocked` — with a lock held by a live PID, `run` returns `0` and writes nothing to stderr (spec §5: overlap is expected, not an error).
- **Implement:** `run` subcommand loads config (`--config`, default `./autoship.yaml`), opens the store at `%LOCALAPPDATA%\autoship\<sha256(repo.path)[:12]>\`, acquires the lock, `defer Release()`.
- **Proves done:** `go test ./internal/cli/` → `ok`
- **Depends on:** 3, 5
- **Commit:** `feat(cli): wire run command to config, state and lock`

**Batch verification:** `go vet ./... && go test ./...`

---

## Batch 2 — S0 gate, halt semantics, logging

### Task 7 — Resolve remote head SHA
- **Files:** `internal/gitrepo/git.go`, `internal/gitrepo/git_test.go`
- **Test first:** `TestResolveRemoteHead` — build a fixture in `t.TempDir()`: `git init` an origin, commit, `git clone` it; `ResolveRemoteHead(clone, "origin", "main")` equals the origin's HEAD SHA.
- **Implement:** `Repo{Path string}`. `ResolveRemoteHead` execs `git -C <path> fetch <remote> <branch> --quiet` then `git -C <path> rev-parse <remote>/<branch>`, trimming output (A5).
- **Proves done:** `go test ./internal/gitrepo/ -run TestResolveRemoteHead` → `ok`
- **Depends on:** 1
- **Commit:** `feat(git): resolve remote head sha`

### Task 8 — Classify fetch failures as transient
- **Files:** `internal/gitrepo/git.go`, `internal/gitrepo/git_test.go`
- **Test first:** `TestResolveRemoteHead_UnreachableRemoteIsTransient` — a repo whose `origin` points at a nonexistent path → `errors.Is(err, ErrTransient)`.
- **Implement:** `var ErrTransient = errors.New(...)`; wrap any non-zero exit from the `fetch` step with it. `rev-parse` failures stay non-transient.
- **Proves done:** `go test ./internal/gitrepo/` → `ok`
- **Depends on:** 7
- **Commit:** `feat(git): classify fetch failures as transient`

### Task 9 — S0 gate decision
- **Files:** `internal/pipeline/gate.go`, `internal/pipeline/gate_test.go`
- **Test first:** `TestDecide` — table covering: same SHA → `SkipNoChange`; new SHA → `Proceed`; `ErrTransient` → `SkipTransient`; `status=halted` + same SHA → `SkipHalted`; `status=halted` + **new** SHA → `Proceed` (spec Q5 auto-clear).
- **Implement:** `Decide(st state.State, head string, err error) Decision` — a pure function, no I/O, so the whole gate is table-testable.
- **Proves done:** `go test ./internal/pipeline/ -run TestDecide` → `ok`
- **Depends on:** 4, 8
- **Commit:** `feat(pipeline): s0 gate decision incl. halt auto-clear`

### Task 10 — Halt set and clear
- **Files:** `internal/state/halt.go`, `internal/state/halt_test.go`
- **Test first:** `TestSetHalt` — records stage, reason, sha, log path and sets `Status == "halted"`. `TestClearHalt` — resets `Status` to `idle` and nils the halt block.
- **Implement:** `SetHalt(st *State, stage, reason, sha, logPath string, now time.Time)` and `ClearHalt(st *State)`. Time is injected, not read from the clock, so tests are deterministic.
- **Proves done:** `go test ./internal/state/ -run TestHalt` → `ok`
- **Depends on:** 4
- **Commit:** `feat(state): halt set and clear`

### Task 11 — Per-run log file
- **Files:** `internal/logging/log.go`, `internal/logging/log_test.go`
- **Test first:** `TestNewRunLog_TeesToFileAndWriter` — after logging one line, the file under `logs/` exists, and both it and the passed `io.Writer` contain the message.
- **Implement:** `NewRunLog(dir string, runID string, tee io.Writer) (*slog.Logger, func() error, error)` using `slog` over an `io.MultiWriter`. Run ID is `run-<RFC3339 with colons replaced>`.
- **Proves done:** `go test ./internal/logging/` → `ok`
- **Depends on:** 3
- **Commit:** `feat(logging): per-run log file`

### Task 12 — Short-circuit `run` on no-change (the NFR guard)
- **Files:** `internal/cli/run.go`, `internal/cli/run_test.go`
- **Test first:** `TestRun_NoChangeDoesNotInvokeGradle` — inject a spy satisfying the Gradle runner interface; with `state.LastProcessedSHA` equal to the fixture repo's head, `run` returns `0` **and the spy records zero invocations**. This is the executable form of spec §3's core property; it must exist before any Gradle code does.
- **Implement:** In `run`, after acquiring the lock: resolve head, `Decide`, and on any `Skip*` log the reason and return `0` before constructing the build stages.
- **Proves done:** `go test ./internal/cli/ -run TestRun_NoChange` → `ok`
- **Depends on:** 6, 9, 11
- **Commit:** `feat(pipeline): short-circuit run on no-change`

**Batch verification:** `go vet ./... && go test ./...`

---

## Batch 3 — Preflight: versions and release kind

### Task 13 — Parse `build.gradle.kts`
- **Files:** `internal/gradlefile/version.go`, `internal/gradlefile/version_test.go`, `internal/gradlefile/testdata/build.gradle.kts`
- **Test first:** `TestParse` — fixture containing `versionCode = 7` and `versionName = "1.0.5-SNAPSHOT"` yields `Version{Code: 7, Name: "1.0.5-SNAPSHOT"}`. `TestParse_MissingVersionName` returns an error naming the field.
- **Implement:** `Parse(path string) (Version, error)` using `regexp` for `versionName\s*=\s*"([^"]+)"` and `versionCode\s*=\s*(\d+)`, taking the first match in the file.
- **Proves done:** `go test ./internal/gradlefile/ -run TestParse` → `ok`
- **Depends on:** 1
- **Commit:** `feat(gradlefile): parse versionName and versionCode`

### Task 14 — Strip `-SNAPSHOT`
- **Files:** `internal/gradlefile/version.go`, `internal/gradlefile/version_test.go`
- **Test first:** `TestReleaseName` — `"1.0.5-SNAPSHOT"` → `"1.0.5"`; `"1.0.5"` → `"1.0.5"`; `"1.0.5-rc1"` → `"1.0.5-rc1"` (only the exact `-SNAPSHOT` suffix is stripped).
- **Implement:** `(Version).ReleaseName() string` via `strings.TrimSuffix`.
- **Proves done:** `go test ./internal/gradlefile/` → `ok`
- **Depends on:** 13
- **Commit:** `feat(gradlefile): strip snapshot suffix`

### Task 15 — Classify release kind
- **Files:** `internal/release/kind.go`, `internal/release/kind_test.go`
- **Test first:** `TestClassify` — `1.0.4→1.0.5` = `Patch`; `1.0.5→1.1.0` = `Minor`; `1.1.0→2.0.0` = `Major`; equal → `ErrNoVersionBump`; lower → `ErrVersionWentBackwards`. `TestClassify_NoPrevious` — empty previous → `Major` (first release).
- **Implement:** `Kind` enum, `Classify(prev, next string) (Kind, error)`; parse `major.minor.patch` with `strconv`, no semver dependency.
- **Proves done:** `go test ./internal/release/` → `ok`
- **Depends on:** 1
- **Commit:** `feat(release): classify patch vs non-patch`

### Task 16 — Write back bumped version
- **Files:** `internal/gradlefile/write.go`, `internal/gradlefile/write_test.go`
- **Test first:** `TestBump` — on a copy of the fixture, bumping `1.0.5`/`7` yields file content with `versionCode = 8` and `versionName = "1.0.6-SNAPSHOT"`, **and every other line byte-identical** to the original.
- **Implement:** `Bump(path string, next Version) error` — regexp `ReplaceAll` on the two lines only; read/modify/write whole file.
- **Proves done:** `go test ./internal/gradlefile/ -run TestBump` → `ok`
- **Depends on:** 14
- **Commit:** `feat(gradlefile): write back bumped version`

### Task 17 — S1 preflight stage
- **Files:** `internal/pipeline/preflight.go`, `internal/pipeline/preflight_test.go`
- **Test first:** `TestPreflight_HaltsWhenVersionCodeNotGreater` — `versionCode = 7` with `LastPublishedVersionCode = 7` → halt whose reason contains both `7` and the field name. `TestPreflight_ReturnsRelease` — valid input yields `Release{Name: "1.0.5", Code: 8, Kind: Patch}`.
- **Implement:** `Preflight(cfg, st) (Release, error)` composing Tasks 13–15 and returning a halt-classified error.
- **Proves done:** `go test ./internal/pipeline/ -run TestPreflight` → `ok`
- **Depends on:** 10, 13, 14, 15
- **Commit:** `feat(pipeline): s1 preflight version checks`

**Batch verification:** `go vet ./... && go test ./...`

---

## Batch 4 — Command runner, Gradle, UI validation, artifacts

### Task 18 — Streaming command runner
- **Files:** `internal/runner/exec.go`, `internal/runner/exec_test.go`
- **Test first:** `TestRun_CapturesOutputAndExitCode` — running `go env GOOS` returns exit `0` and non-empty output. `TestRun_TimeoutCancels` — a command exceeding a 50 ms context → `errors.Is(err, context.DeadlineExceeded)`.
- **Implement:** `Runner` interface `Run(ctx, dir, name string, args ...string) (int, error)` writing merged stdout/stderr to an injected `io.Writer`. Real impl `ExecRunner`; interface exists so every stage below is testable with a spy.
- **Proves done:** `go test ./internal/runner/` → `ok`
- **Depends on:** 1
- **Commit:** `feat(runner): streaming command runner with timeout`

### Task 19 — Gradle build and test stage
- **Files:** `internal/build/gradle.go`, `internal/build/gradle_test.go`
- **Test first:** `TestGradleStage_RunsTasksInOrder` — spy runner records exactly `[":app:testDebugUnitTest", ":app:lintRelease", ":app:bundleRelease"]`. `TestGradleStage_HaltsOnFailure` — spy returning exit `1` on the test task → error naming `S2` and the failing task, and the later tasks are **not** invoked.
- **Implement:** `GradleStage{Runner, Config}.Execute(ctx) error` invoking `gradlew.bat` on Windows / `./gradlew` elsewhere (A4).
- **Proves done:** `go test ./internal/build/ -run TestGradleStage` → `ok`
- **Depends on:** 18
- **Commit:** `feat(build): gradle test, lint and bundle stage`

### Task 20 — Locate the release bundle
- **Files:** `internal/build/bundle.go`, `internal/build/bundle_test.go`
- **Test first:** `TestFindBundle` — a temp tree with two `.aab` files returns the most recently modified. `TestFindBundle_None` → error naming the searched directory.
- **Implement:** `FindBundle(moduleDir string) (string, error)` globbing `build/outputs/bundle/release/*.aab`.
- **Proves done:** `go test ./internal/build/` → `ok`
- **Depends on:** 19
- **Commit:** `feat(build): locate release bundle output`

### Task 21 — S3 UI validation stage
- **Files:** `internal/build/uivalidate.go`, `internal/build/uivalidate_test.go`
- **Test first:** `TestUIValidation_ModeNone` — zero spy invocations. `TestUIValidation_ModeJVM` — invokes the configured task. `TestUIValidation_HaltsOnFailure` — non-zero exit → error naming `S3`.
- **Implement:** `UIValidateStage.Execute(ctx) error` switching on `cfg.UIValidation.Mode`; `emulator` returns `ErrNotImplemented` for now (spec §6 keeps the config key reserved).
- **Proves done:** `go test ./internal/build/ -run TestUIValidation` → `ok`
- **Depends on:** 19
- **Commit:** `feat(build): s3 ui validation stage`

### Task 22 — Assemble the versioned artifact folder
- **Files:** `internal/artifacts/assemble.go`, `internal/artifacts/assemble_test.go`
- **Test first:** `TestAssemble` — given root `X` and release `1.0.5`, creates `X/v1.0.5/` and `X/v1.0.5/screenshots/`, and copies the source `.aab` to `X/v1.0.5/app-release.aab` with identical bytes.
- **Implement:** `Assemble(root string, rel Release, bundlePath string) (dir string, err error)`, matching the layout in the `releasing-app` skill.
- **Proves done:** `go test ./internal/artifacts/` → `ok`
- **Depends on:** 20
- **Commit:** `feat(artifacts): assemble versioned release folder`

**Batch verification:** `go vet ./... && go test ./...`

---

## Batch 5 — Notes providers (the swappable seam)

### Task 23 — `NotesProvider` interface and file provider
- **Files:** `internal/notes/notes.go`, `internal/notes/file.go`, `internal/notes/file_test.go`
- **Test first:** `TestFileProvider_ReadsVersionedFile` — `docs/release/notes/1.0.5.txt` present → its contents returned. `TestFileProvider_MissingIsErrNoNotes` — absent → `errors.Is(err, ErrNoNotes)` (**not** a halt — the chain decides that, per spec §7.1).
- **Implement:** `NotesProvider` interface exactly as in spec §7.1; `FileProvider{PathTemplate string}` substituting `${version}`.
- **Proves done:** `go test ./internal/notes/ -run TestFileProvider` → `ok`
- **Depends on:** 3
- **Commit:** `feat(notes): notes provider interface and file provider`

### Task 24 — Commit-derived provider
- **Files:** `internal/notes/commits.go`, `internal/notes/commits_test.go`
- **Test first:** `TestCommitsProvider_RendersSubjects` — given subjects `feat: add settings screen`, `fix: filter persistence`, `chore: bump deps`, output contains the first two as bullets and omits the `chore:` line. `TestCommitsProvider_NoCommits` → `ErrNoNotes`.
- **Implement:** `CommitsProvider{Repo, Template}` running `git log <lastTag>..HEAD --pretty=%s`, filtering `chore|ci|build|test|docs` prefixes, rendering via `text/template`.
- **Proves done:** `go test ./internal/notes/ -run TestCommitsProvider` → `ok`
- **Depends on:** 7, 23
- **Commit:** `feat(notes): commit-derived notes provider`

### Task 25 — Resolve the chain from config
- **Files:** `internal/notes/chain.go`, `internal/notes/chain_test.go`
- **Test first:** `TestChain_FileOnlyExhausts` — `source: [file]` with no file → `ErrNoNotes`. `TestChain_FallsThroughToCommits` — `source: [file, commits]` with no file → commit-derived notes returned, **with no change to any file outside the config**. `TestChain_UnknownProvider` → error naming the bad value.
- **Implement:** `BuildChain(cfg config.Notes, deps Deps) (NotesProvider, error)` mapping names to constructors; `Chain.Notes` tries each in order, returning the first non-`ErrNoNotes` result. This test is the executable proof of the swappability requirement.
- **Proves done:** `go test ./internal/notes/ -run TestChain` → `ok`
- **Depends on:** 24
- **Commit:** `feat(notes): resolve provider chain from config`

### Task 26 — Enforce the Play 500-character limit
- **Files:** `internal/notes/validate.go`, `internal/notes/validate_test.go`
- **Test first:** `TestValidate_RejectsOversize` — 501 characters → error containing `501` and `500`. `TestValidate_AcceptsAtLimit` — exactly 500 → nil.
- **Implement:** `Validate(s string) error` counting runes, applied to whatever the chain returns.
- **Proves done:** `go test ./internal/notes/` → `ok`
- **Depends on:** 25
- **Commit:** `feat(notes): enforce play store 500 char limit`

### Task 27 — Technical release notes
- **Files:** `internal/notes/technical.go`, `internal/notes/technical_test.go`
- **Test first:** `TestTechnicalNotes` — output contains the version name, version code, the commit range, and the lint/test outcome strings passed in.
- **Implement:** `Technical(rel Release, res BuildResult) string` via `text/template`, matching the shape in the `releasing-app` skill's `release-notes-technical.md`.
- **Proves done:** `go test ./internal/notes/ -run TestTechnicalNotes` → `ok`
- **Depends on:** 23
- **Commit:** `feat(notes): generate technical release notes`

**Batch verification:** `go vet ./... && go test ./...`

---

## Batch 6 — Play Publisher (S5)

### Task 28 — Edit client interface and fake
- **Files:** `internal/play/client.go`, `internal/play/fake_test.go`, `internal/play/publish_test.go`
- **Test first:** `TestPublish_FollowsEditLifecycle` — fake records the call order `Insert`, `UploadBundle`, `TrackUpdate`, `Commit`.
- **Implement:** Narrow `EditClient` interface with exactly those five methods plus `Delete`. No Google import yet — this task defines the seam only, so every later test runs offline.
- **Proves done:** `go test ./internal/play/ -run TestPublish_FollowsEditLifecycle` → `ok`
- **Depends on:** 1
- **Commit:** `feat(play): edit client interface`

### Task 29 — Track assignment with release notes
- **Files:** `internal/play/publish.go`, `internal/play/publish_test.go`
- **Test first:** `TestTrackUpdate_SetsTrackAndNotes` — fake receives track `alpha`, the release's version code, `en-US` notes matching the chain output, and status `draft` when `play.rollout: draft`.
- **Implement:** `Publisher.assignTrack(...)` building the track payload from config and `Release`.
- **Proves done:** `go test ./internal/play/ -run TestTrackUpdate` → `ok`
- **Depends on:** 26, 28
- **Commit:** `feat(play): assign bundle to closed testing track`

### Task 30 — Listing and screenshots on non-patch only
- **Files:** `internal/play/listing.go`, `internal/play/listing_test.go`
- **Test first:** `TestPublish_PatchSkipsListing` — `Kind == Patch` → fake records no `PatchListing` and no `UploadImage`. `TestPublish_MinorUpdatesListing` — `Kind == Minor` → both recorded, image count equal to the files in the screenshots dir.
- **Implement:** Gate the listing/image calls on `rel.Kind != Patch`, honouring `play.update_listing_on` (spec §7.3, R5).
- **Proves done:** `go test ./internal/play/ -run TestPublish_.*Listing` → `ok`
- **Depends on:** 22, 29
- **Commit:** `feat(play): update listing and screenshots for non-patch releases`

### Task 31 — Abort the edit on any failure
- **Files:** `internal/play/publish.go`, `internal/play/publish_test.go`
- **Test first:** `TestPublish_DeletesEditOnFailure` — fake returning an error from `UploadBundle` → `Delete` recorded and `Commit` **not** recorded. Repeat for a `TrackUpdate` failure.
- **Implement:** `defer` an abort that calls `Delete` unless the edit committed successfully. This is the guarantee that justified writing the client ourselves (spec §10, §12).
- **Proves done:** `go test ./internal/play/ -run TestPublish_DeletesEditOnFailure` → `ok`
- **Depends on:** 30
- **Commit:** `feat(play): abort edit on any failure`

### Task 32 — Real Google client and bounded retry
- **Files:** `internal/play/client_google.go`, `internal/play/retry.go`, `internal/play/retry_test.go`
- **Test first:** `TestRetry_RetriesServerErrors` — a stub failing twice with `503` then succeeding → 3 attempts, no error. `TestRetry_DoesNotRetryClientErrors` — `400` → 1 attempt, error returned.
- **Implement:** `googleClient` implementing `EditClient` over `androidpublisher/v3`. `withRetry(fn)` — 3 attempts, exponential backoff, retrying only 5xx and network errors (spec §10).
- **Proves done:** `go test ./internal/play/` → `ok`
- **Depends on:** 31
- **Commit:** `feat(play): google publisher client with bounded retry`

**Batch verification:** `go vet ./... && go test ./...`

---

## Batch 7 — Post-release, secrets, dry-run, scheduling

### Task 33 — S6 tag, bump and push
- **Files:** `internal/pipeline/postrelease.go`, `internal/pipeline/postrelease_test.go`
- **Test first:** `TestPostRelease` — against a temp origin+clone fixture: creates tag `v1.0.5`, a commit whose message is the chore bump, and pushes both; origin then contains the tag.
- **Implement:** `PostRelease(...)` composing `gradlefile.Bump` (Task 16) with `git tag`, `git commit -am`, `git push --follow-tags`.
- **Proves done:** `go test ./internal/pipeline/ -run TestPostRelease$` → `ok`
- **Depends on:** 16, 17
- **Commit:** `feat(pipeline): s6 tag, bump and push`

### Task 34 — Rebase and retry a rejected push
- **Files:** `internal/pipeline/postrelease.go`, `internal/pipeline/postrelease_test.go`
- **Test first:** `TestPostRelease_RebasesOnRejectedPush` — push a commit to origin from a second clone so the first clone is behind; `PostRelease` rebases and succeeds. `TestPostRelease_HaltsWithBothIdentifiers` — with rebase forced to fail, the halt reason contains the published version code **and** the unpushed commit SHA (spec §11).
- **Implement:** On push rejection: `git pull --rebase` then retry once; on second failure return a halt error naming both identifiers.
- **Proves done:** `go test ./internal/pipeline/ -run TestPostRelease_` → `ok`
- **Depends on:** 33
- **Commit:** `feat(pipeline): rebase and retry rejected post-release push`

### Task 35 — DPAPI credential store
- **Files:** `internal/secrets/store.go`, `internal/secrets/dpapi_windows.go`, `internal/secrets/dpapi_other.go`, `internal/secrets/store_test.go`
- **Test first:** `TestStore_RoundTrip` — `Set("play_sa", data)` then `Get("play_sa")` returns the same bytes, and the on-disk file does **not** contain the plaintext. Windows-gated; the non-Windows build returns `ErrUnsupported` and the test skips (A4).
- **Implement:** `CryptProtectData`/`CryptUnprotectData` via `golang.org/x/sys/windows`, scoped to the current user. `secrets set` subcommand prompts on a terminal, never via a flag.
- **Proves done:** `go test ./internal/secrets/` → `ok`
- **Depends on:** 1
- **Commit:** `feat(secrets): dpapi-encrypted credential store`

### Task 36 — Dry-run mode
- **Files:** `internal/cli/run.go`, `internal/cli/run_test.go`
- **Test first:** `TestDryRun_SkipsPublishAndPostRelease` — with `--dry-run`, spies confirm the bundle **was** built and the artifact folder **was** assembled, while `Commit` and `git push` were never called.
- **Implement:** Thread a `DryRun bool` through the pipeline; S5 commit and all of S6 become no-ops that log what they would have done (spec §13 step 1).
- **Proves done:** `go test ./internal/cli/ -run TestDryRun` → `ok`
- **Depends on:** 32, 34
- **Commit:** `feat(cli): dry-run mode`

### Task 37 — `status` and `resume`
- **Files:** `internal/cli/status.go`, `internal/cli/status_test.go`
- **Test first:** `TestStatus_ReportsHalt` — with a halted state, output contains the stage, reason and log path, and exit code is `1`. `TestResume_ClearsHalt` — after `resume`, the persisted state has `Status == "idle"`.
- **Implement:** Both subcommands over the existing store and `ClearHalt` (Task 10).
- **Proves done:** `go test ./internal/cli/` → `ok`
- **Depends on:** 10, 36
- **Commit:** `feat(cli): status and resume commands`

### Task 38 — Task Scheduler registration
- **Files:** `scripts/register-task.ps1`, `docs/scheduling.md`
- **Test first:** None — this task is a script and prose; its correctness is proven by the manual command below, not by `go test`.
- **Implement:** PowerShell registering `autoship` via `Register-ScheduledTask`: 15-minute repetition inside a working-hours window (spec Q1), `-WakeToRun:$false`, run-whether-logged-on-or-not, start-in the config directory. `docs/scheduling.md` documents secret setup, the dry-run soak, and the `draft` → `completed` promotion from spec §13.
- **Proves done:** `powershell -File scripts/register-task.ps1 -WhatIf` prints the task definition without registering it; `schtasks /query /tn autoship` lists the task after a real run.
- **Depends on:** 37
- **Commit:** `docs(autoship): windows task scheduler setup`

**Batch verification:** `go vet ./... && go test ./...`

---

## Verification

```bash
go vet ./...
go test ./... -race
go build -o autoship.exe ./cmd/autoship
```

**Completion condition:** all three exit `0`, and `git status --porcelain` is empty.

**Acceptance beyond the unit suite** — these are the spec's load-bearing claims, and each is checked by a named test rather than by inspection:

| Spec claim | Proven by |
|---|---|
| §3 no-op tick never starts a JVM | Task 12 `TestRun_NoChangeDoesNotInvokeGradle` |
| §5 overlapping invocations are safe | Task 6 `TestRun_ExitsZeroWhenAlreadyLocked` |
| §5 halt is sticky, auto-clears on new SHA | Task 9 `TestDecide` |
| §7.1 notes policy is config-swappable | Task 25 `TestChain_FallsThroughToCommits` |
| §7.3 patch releases skip listing upload | Task 30 `TestPublish_PatchSkipsListing` |
| §10 a failed publish never leaves a dangling edit | Task 31 `TestPublish_DeletesEditOnFailure` |
| §11 rejected push halts with both identifiers | Task 34 `TestPostRelease_HaltsWithBothIdentifiers` |

**Manual soak before trusting it** (spec §13): run with `--dry-run` against real `main` movement until the assembled artifact folder matches what the `releasing-app` skill produces by hand; then `rollout: draft` for several releases; only then `completed`.

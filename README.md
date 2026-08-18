# autoship

[![CI](https://github.com/vipinm/autoship/actions/workflows/ci.yml/badge.svg)](https://github.com/vipinm/autoship/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/vipinm/autoship.svg)](https://pkg.go.dev/github.com/vipinm/autoship)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

**Unattended Android releases: `main` moves → a closed-testing build reaches
testers.** No CI minutes, no server, no daemon — a single binary that wakes
up on a schedule, checks if there's anything to ship, and gets out of the way
when there isn't.

```
main gets a commit  ──▶  autoship notices on the next tick  ──▶  testers get a build
```

Runs on **Windows, macOS, and Linux**, on a machine that isn't paying for the
privilege while nothing is happening.

- **Spec:** [`docs/specs/android-release-automation.md`](docs/specs/android-release-automation.md)
- **Plan:** [`docs/plans/android-release-automation.md`](docs/plans/android-release-automation.md)
- **Operating it:** [`docs/scheduling.md`](docs/scheduling.md)

## Why

Cutting a release is deterministic given the repo state — check out `main`,
run tests, bump the version, build, write notes, upload — but a human has to
be present and awake for all of it. The cost isn't difficulty, it's
*attendance*. autoship turns that into: push to `main`, and the next
scheduled tick does the rest.

## Shape

A single binary invoked by the OS scheduler. Not a daemon — a job that is
idle ~99% of the time should not hold RAM to be idle in.

```
scheduler (every 15 min)
        │
        ▼
   ┌─────────┐   no change
   │ S0 gate ├──────────────► exit 0   (git fetch + a string compare, no JVM)
   └────┬────┘
        │ new SHA on origin/main
        ▼
   S1 preflight    version and versionCode sanity
   S2 build+test   gradlew test → lint → bundleRelease
   S3 ui validate  jvm | emulator | none
   S4 assemble     artifact folder, notes, assets
   S5 publish      Play Publisher API → closed testing
   S6 post         tag, bump to the next -SNAPSHOT, commit, push
```

The load-bearing property is S0: the common invocation must never start a JVM,
never touch Gradle, and never allocate meaningfully. `TestRun_NoChangeDoesNotInvokeGradle`
is that property in executable form.

## Cross-platform, natively — not just "it happens to compile"

Every OS-specific piece has a real backend, not a stub: the scheduler and the
credential store are both native to the platform you run on.

| | Windows | macOS | Linux |
|---|---|---|---|
| Scheduler | Task Scheduler | launchd (LaunchAgent) | systemd (`--user` timer) |
| Secrets encrypted at rest via | DPAPI | Keychain | Secret Service (`secret-tool`) |
| Install script | [`scripts/register-task.ps1`](scripts/register-task.ps1) | [`scripts/install-launchd.sh`](scripts/install-launchd.sh) | [`scripts/install-systemd-timer.sh`](scripts/install-systemd-timer.sh) |

No platform gets a weaker fallback: if the native secret store isn't
available, `autoship secrets set` refuses outright rather than writing
plaintext.

## Quick start

```bash
git clone https://github.com/vipinm/autoship.git && cd autoship
go build -o autoship ./cmd/autoship        # autoship.exe on Windows

cp autoship.example.yaml /path/to/your-android-repo/autoship.yaml
# edit it, then:
autoship status --config /path/to/your-android-repo/autoship.yaml

autoship secrets set play_sa --from-file /path/to/play-service-account.json
autoship dry-run --config /path/to/your-android-repo/autoship.yaml
```

That last command is the important one: `--dry-run` does everything except
the two irreversible steps, so you can watch it work before it can publish
anything. Registering it on a schedule and promoting it to a real release is
covered end to end in [`docs/scheduling.md`](docs/scheduling.md).

## Commands

```
autoship run           poll the repo and release if main moved
autoship dry-run       everything except the Play commit and the S6 push
autoship status        report the run state; exit 1 while halted
autoship resume        clear a halt so the next run retries
autoship secrets       manage encrypted credentials (DPAPI / Keychain / Secret Service)
autoship draft-notes   draft customer copy from the commit log, for a human to edit
autoship version
```

## Layout

```
cmd/autoship/          entrypoint
internal/
  cli/                 subcommand dispatch and wiring
  config/              autoship.yaml
  state/                state.json, PID-aware lock, halt
  logging/             per-run log
  gitrepo/             fetch, rev-parse, tag, push, rebase
  gradlefile/          build.gradle.kts read and write-back
  release/             semver classification
  runner/              exec with streaming output and a deadline
  build/               gradle stages, bundle discovery
  artifacts/           versioned folder assembly
  notes/               NotesProvider chain
  play/                Play publisher edit lifecycle
  secrets/             encrypted credential store (DPAPI / Keychain / Secret Service)
  pipeline/            S0–S6 orchestration
scripts/               scheduler install scripts (Task Scheduler / launchd / systemd)
```

## Three things worth knowing before changing it

1. **Halt is sticky.** A broken `main` must not trigger a 4 GB Gradle build
   every 15 minutes forever. A new commit clears it; so does `autoship resume`.
2. **Release notes are an input, not an output.** `docs/release/notes/<version>.txt`
   is the developer's "ready to ship" signal. The policy is a provider chain, so
   switching to generated copy is a config edit (`notes.source: [file, commits]`),
   not a code change.
3. **A failed publish never leaves a dangling edit.** The Play edit is abandoned
   on every failure path, including a dry run.

## Build and test

```bash
go vet ./...
go test ./... -race
go build -o autoship ./cmd/autoship   # autoship.exe on Windows
```

The tests need `git` on `PATH`; they do not need a JVM, an Android SDK, or a
Play account. Platform-gated tests (DPAPI / Keychain / Secret Service) run
for real on each OS in CI — see the badge above.

## Contributing

Small, opinionated, and Go-standard-library-first by design — see
[`CONTRIBUTING.md`](CONTRIBUTING.md) before sending a PR.

## License

[MIT](LICENSE)

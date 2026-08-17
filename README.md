# autoship

Unattended Android releases: `main` moves → a closed-testing build reaches
testers, on a machine that isn't paying for the privilege while nothing is
happening.

- **Spec:** [`docs/specs/android-release-automation.md`](docs/specs/android-release-automation.md)
- **Plan:** [`docs/plans/android-release-automation.md`](docs/plans/android-release-automation.md)
- **Operating it:** [`docs/scheduling.md`](docs/scheduling.md)

## Shape

A single binary invoked by Windows Task Scheduler. Not a daemon — a job that is
idle ~99% of the time should not hold RAM to be idle in.

```
Task Scheduler (every 15 min)
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

## Commands

```
autoship run           poll the repo and release if main moved
autoship dry-run       everything except the Play commit and the S6 push
autoship status        report the run state; exit 1 while halted
autoship resume        clear a halt so the next run retries
autoship secrets       manage DPAPI-encrypted credentials
autoship draft-notes   draft customer copy from the commit log, for a human to edit
autoship version
```

## Layout

```
cmd/autoship/          entrypoint
internal/
  cli/                 subcommand dispatch and wiring
  config/              autoship.yaml
  state/               state.json, PID-aware lock, halt
  logging/             per-run log
  gitrepo/             fetch, rev-parse, tag, push, rebase
  gradlefile/          build.gradle.kts read and write-back
  release/             semver classification
  runner/              exec with streaming output and a deadline
  build/               gradle stages, bundle discovery
  artifacts/           versioned folder assembly
  notes/               NotesProvider chain
  play/                Play publisher edit lifecycle
  secrets/             DPAPI credential store
  pipeline/            S0–S6 orchestration
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

```
go vet ./...
go test ./... -race
go build -o autoship.exe ./cmd/autoship
```

The tests need `git` on PATH; they do not need a JVM, an Android SDK, or a Play
account. Windows-only paths (DPAPI) skip elsewhere.

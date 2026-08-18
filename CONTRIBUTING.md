# Contributing

autoship is small on purpose — a scheduled one-shot, not a service. Keep that
shape in mind before adding to it.

## Before you start

For anything beyond a small fix, open an issue first. It's cheap to agree on
an approach before writing code and expensive to unwind a large PR that took
the wrong one.

## Working in the repo

```bash
go build -o autoship ./cmd/autoship   # autoship.exe on Windows
go vet ./...
go test ./... -race
```

Tests need `git` on `PATH`. They do not need a JVM, an Android SDK, or a Play
account — the pieces that do are faked or skipped (see
[`docs/plans/android-release-automation.md`](docs/plans/android-release-automation.md)
for how each stage is made testable without them). Platform-specific code
(DPAPI, Keychain, Secret Service, the process-alive check) lives behind
`_windows.go` / `_darwin.go` / `_linux.go` build tags — CI (`.github/workflows/ci.yml`)
runs the full suite on all three.

## What a good PR looks like

- **Matches the load-bearing property it touches.** S0 (the no-change path)
  must never start a JVM or touch Gradle — `TestRun_NoChangeDoesNotInvokeGradle`
  is that property in executable form and should stay green.
- **Comes with a test that would have failed without the fix.**
- **Doesn't add a config knob for something with one reasonable default.** The
  three-doc trail — [spec](docs/specs/android-release-automation.md),
  [plan](docs/plans/android-release-automation.md),
  [scheduling guide](docs/scheduling.md) — explains most of the "why" behind
  a decision; check there before assuming something is an oversight.
- **Keeps secrets out of everything but the platform's native store.** No new
  code path should ever write a credential to disk in plaintext, log it, or
  accept it as a CLI argument.

## Reporting a security issue

Please don't open a public issue for a credential-handling or secrets-storage
vulnerability. Email the maintainer directly (see the GitHub profile on the
commits) instead.

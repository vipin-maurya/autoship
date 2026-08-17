# Running autoship unattended

autoship is a scheduled one-shot. It runs, decides, possibly works, and exits.
Between ticks it costs nothing, which is the whole reason it is not a daemon.

This document covers the three things that have to be true before it can be
trusted: the secrets exist, the dry-run soak was boring, and the rollout was
promoted deliberately rather than by default.

---

## 1. Install

```powershell
go build -o C:\tools\autoship.exe .\cmd\autoship
```

Copy [`autoship.example.yaml`](../autoship.example.yaml) into the Android repo
as `autoship.yaml`, and edit it. It is versioned and contains no secrets.

Check it loads:

```powershell
C:\tools\autoship.exe status --config C:\Users\vm899\repos\ExpenseTracker\autoship.yaml
```

---

## 2. Secrets

Two secrets are needed, and neither may sit in plaintext — a scheduled task
runs non-interactively, so "I'll type it when it asks" is not available.
Both are encrypted with DPAPI, scoped to the Windows user that stored them.

```powershell
autoship secrets set play_sa --from-file C:\path\to\play-service-account.json
autoship secrets set keystore_password
autoship secrets set key_alias_password
autoship secrets list
```

Without `--from-file`, the value is read from an interactive prompt with echo
off, or from piped stdin. It is never accepted as a command-line argument: an
argument lands in the shell history and in the process list.

**Grant the Play service account only "Release to testing tracks."** It should
be incapable of touching production even if the key leaks.

The blobs live in `%LOCALAPPDATA%\autoship\secrets\`. They are decryptable only
by the user account that wrote them — the same account the scheduled task must
run as, which is why the task is registered with an S4U principal for the
current user rather than SYSTEM.

---

## 3. The dry-run soak

`--dry-run` performs everything except the two irreversible steps: the Play
edit is prepared and then abandoned, and S6 does not tag, commit or push.

```powershell
autoship dry-run --config C:\Users\vm899\repos\ExpenseTracker\autoship.yaml
```

Register it on the schedule and leave it there until it is boring:

```powershell
powershell -File scripts\register-task.ps1 `
    -ExePath C:\tools\autoship.exe `
    -ConfigPath C:\Users\vm899\repos\ExpenseTracker\autoship.yaml `
    -DryRun
```

**What you are checking:** that `%USERPROFILE%\Documents\ExpenseTracker\v<version>\`
matches what the manual `releasing-app` skill produces by hand — the bundle,
`release-notes-customer.txt`, `release-notes-technical.md`, and the screenshots
folder. If it does not, fix that before real uploads, not after.

Because a dry run never records a published version, the same change is
re-released on the next tick — which is what makes the soak repeatable.

---

## 4. Promotion

The rollout goes through three states, in order, deliberately (spec §13):

| `play.rollout` | What testers see | Stay here until |
|---|---|---|
| *(dry-run)* | nothing | the artifact folder matches the manual skill |
| `draft` | nothing until you click publish in Play Console | several releases have been unremarkable |
| `completed` | builds arrive unattended | — |

Change one line in `autoship.yaml` and re-register the task without `-DryRun`:

```powershell
powershell -File scripts\register-task.ps1 `
    -ExePath C:\tools\autoship.exe `
    -ConfigPath C:\Users\vm899\repos\ExpenseTracker\autoship.yaml
```

---

## 5. The schedule

The default is every 15 minutes inside a 10-hour window starting at 09:00.
Adjust with `-IntervalMinutes`, `-StartTime` and `-WindowHours`.

Two properties are deliberate:

- **`-WakeToRun:$false`.** A release that waits until the machine is open is the
  correct trade for a workstation.
- **`MultipleInstances IgnoreNew`.** A release build can outlive the interval.
  autoship also holds its own PID-aware lock, so an overlapping invocation exits
  0 silently even if Task Scheduler starts one anyway.

Inspect and drive the task:

```powershell
schtasks /query /tn autoship /v /fo list
schtasks /run /tn autoship
schtasks /delete /tn autoship /f
```

---

## 6. When it halts

A halt is sticky on purpose: a broken `main` must not trigger a multi-gigabyte
Gradle build every 15 minutes, forever, in the background.

```powershell
autoship status --config ...\autoship.yaml   # exit code 1 while halted
```

`status` prints the stage, the reason, the commit and the path to that run's
log. Two things clear a halt:

- **A new commit on `main`** — a fix push is the natural "try again" signal, so
  this is automatic.
- **`autoship resume`** — clears it now, without waiting for a push.

Common halts and what they mean:

| Halt | Meaning |
|---|---|
| `S1: versionCode N is not greater than…` | The repo's `versionCode` was not bumped, or a previous run's bump never pushed. |
| `S2: unit tests / lint` | The build is broken. Fix it and push. |
| `S3` | UI validation failed — a screen no longer renders. |
| `S4: no release notes for X` | Write `docs/release/notes/X.txt` and push. This file is the "ready to ship" signal. |
| `S5` | Play rejected the upload. The edit was abandoned, so nothing is dangling. |
| `S6: … already published … but the bump commit … could not be pushed` | The one genuinely awkward state. The build **is** live on the testing track; only the bump commit is stuck. Push it manually from the repo and the next run proceeds. |

Failures that are *not* halts: an unreachable remote (offline, VPN) exits 0 and
retries on the next tick, and so does an overlapping invocation.

---

## 7. Release notes are an input

`docs/release/notes/<version>.txt` must exist on `main` before a version can
ship. That file is the developer's explicit "this version is ready" signal,
which the pipeline otherwise lacks entirely.

To draft one from the commit log and edit it by hand:

```powershell
autoship draft-notes --config ...\autoship.yaml           # print
autoship draft-notes --config ...\autoship.yaml --write   # write the file
```

Generation stays out of the automated path. If you would rather it fell back to
generated copy instead of halting, that is a config edit and no code change:

```yaml
notes:
  source: [file, commits]
```

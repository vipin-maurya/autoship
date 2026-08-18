# Running autoship unattended

autoship is a scheduled one-shot. It runs, decides, possibly works, and exits.
Between ticks it costs nothing, which is the whole reason it is not a daemon.
It runs the same way on Windows, macOS, and Linux — only the scheduler and the
secret store underneath it change.

This document covers the three things that have to be true before it can be
trusted: the secrets exist, the dry-run soak was boring, and the rollout was
promoted deliberately rather than by default.

---

## 1. Install

<details open><summary><b>Windows</b></summary>

```powershell
go build -o C:\tools\autoship.exe .\cmd\autoship
```

</details>

<details><summary><b>macOS</b></summary>

```bash
go build -o /usr/local/bin/autoship ./cmd/autoship
```

</details>

<details><summary><b>Linux</b></summary>

```bash
go build -o ~/.local/bin/autoship ./cmd/autoship
```

</details>

Copy [`autoship.example.yaml`](../autoship.example.yaml) into the Android repo
as `autoship.yaml`, and edit it. It is versioned and contains no secrets.

Check it loads:

```bash
autoship status --config /path/to/MyAndroidApp/autoship.yaml
```

---

## 2. Secrets

Two secrets are needed, and neither may sit in plaintext — a scheduled task
runs non-interactively, so "I'll type it when it asks" is not available.
Every platform encrypts them at rest with its own native secret store, scoped
to the OS user that stored them — never a shared, weaker fallback:

| Platform | Backing store | How |
|---|---|---|
| Windows | DPAPI | `CryptProtectData`, scoped to the Windows user |
| macOS | Keychain | a random AES-256 key held in the login Keychain, sealing the secret on disk |
| Linux | Secret Service | the same AES-256 scheme, with the key held via `secret-tool` (gnome-keyring / kwallet) |

The command line is identical everywhere:

```bash
autoship secrets set play_sa --from-file /path/to/play-service-account.json
autoship secrets set keystore_password
autoship secrets set key_alias_password
autoship secrets list
```

Without `--from-file`, the value is read from an interactive prompt with echo
off, or from piped stdin. It is never accepted as a command-line argument: an
argument lands in the shell history and in the process list.

**Grant the Play service account only "Release to testing tracks."** It should
be incapable of touching production even if the key leaks.

<details open><summary><b>Windows</b></summary>

The blobs live in `%LOCALAPPDATA%\autoship\secrets\`. They are decryptable
only by the user account that wrote them — the same account the scheduled
task must run as, which is why the task is registered with an S4U principal
for the current user rather than SYSTEM.

</details>

<details><summary><b>macOS</b></summary>

The blobs live in `~/Library/Caches/autoship/secrets/`. The AES key that
seals them is a single Keychain item (service `autoship`); the `security` CLI
creates it on first use, no setup required. The login Keychain must be
unlocked, which it is for the duration of any session you're logged into —
the same session the LaunchAgent runs in (§5).

</details>

<details><summary><b>Linux</b></summary>

The blobs live in `~/.cache/autoship/secrets/`. The AES key is held by
whatever implements the Secret Service (gnome-keyring on GNOME, kwallet on
KDE), accessed via `secret-tool`:

```bash
# Debian / Ubuntu
sudo apt install libsecret-tools
# Fedora
sudo dnf install libsecret
# Arch
sudo pacman -S libsecret
```

On a headless box with no desktop session, start gnome-keyring against a
D-Bus session bus once, then leave it running for the systemd user session
that the timer (§5) uses:

```bash
dbus-run-session -- sh -c 'echo "" | gnome-keyring-daemon --unlock; bash'
```

If no Secret Service provider is reachable, `autoship secrets set` fails
clearly rather than falling back to plaintext.

</details>

---

## 3. The dry-run soak

`--dry-run` performs everything except the two irreversible steps: the Play
edit is prepared and then abandoned, and S6 does not tag, commit or push.

```bash
autoship dry-run --config /path/to/MyAndroidApp/autoship.yaml
```

Register it on the schedule and leave it there until it is boring:

<details open><summary><b>Windows</b></summary>

```powershell
powershell -File scripts\register-task.ps1 `
    -ExePath C:\tools\autoship.exe `
    -ConfigPath C:\Users\you\repos\MyAndroidApp\autoship.yaml `
    -DryRun
```

</details>

<details><summary><b>macOS</b></summary>

```bash
scripts/install-launchd.sh \
    --exe /usr/local/bin/autoship \
    --config ~/repos/MyAndroidApp/autoship.yaml \
    --dry-run
```

</details>

<details><summary><b>Linux</b></summary>

```bash
scripts/install-systemd-timer.sh \
    --exe ~/.local/bin/autoship \
    --config ~/repos/MyAndroidApp/autoship.yaml \
    --dry-run
```

</details>

**What you are checking:** that the artifact folder (`artifacts.root` in
`autoship.yaml`, e.g. `%USERPROFILE%\Documents\MyAndroidApp\v<version>\` or
`~/Documents/MyAndroidApp/v<version>/`) matches what the manual
`releasing-app` skill produces by hand — the bundle,
`release-notes-customer.txt`, `release-notes-technical.md`, and the
screenshots folder. If it does not, fix that before real uploads, not after.

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

Change one line in `autoship.yaml` and re-register the task without
`--dry-run` / `-DryRun`:

<details open><summary><b>Windows</b></summary>

```powershell
powershell -File scripts\register-task.ps1 `
    -ExePath C:\tools\autoship.exe `
    -ConfigPath C:\Users\you\repos\MyAndroidApp\autoship.yaml
```

</details>

<details><summary><b>macOS</b></summary>

```bash
scripts/install-launchd.sh \
    --exe /usr/local/bin/autoship \
    --config ~/repos/MyAndroidApp/autoship.yaml
```

</details>

<details><summary><b>Linux</b></summary>

```bash
scripts/install-systemd-timer.sh \
    --exe ~/.local/bin/autoship \
    --config ~/repos/MyAndroidApp/autoship.yaml
```

</details>

---

## 5. The schedule

The default is every 15 minutes inside a 10-hour window starting at 09:00,
on every platform. All three installers accept the same knobs
(`interval` / `start` / `window`, spelled per-OS convention below).

Two properties are deliberate everywhere:

- **The machine is never woken to run it.** A release that waits until the
  machine is open is the correct trade for a workstation.
- **Overlap is harmless.** A release build can outlive the interval. autoship
  holds its own PID-aware lock, so an overlapping invocation exits 0 silently
  no matter what started it.

<details open><summary><b>Windows — Task Scheduler</b></summary>

```powershell
powershell -File scripts\register-task.ps1 `
    -ExePath C:\tools\autoship.exe `
    -ConfigPath C:\Users\you\repos\MyAndroidApp\autoship.yaml `
    -IntervalMinutes 15 -StartTime 09:00 -WindowHours 10
```

Registered with an S4U principal for the current user (not SYSTEM), since
DPAPI and the Gradle/SDK caches are user-scoped.
`-WakeToRun:$false` and `MultipleInstances IgnoreNew` implement the two
properties above.

```powershell
schtasks /query /tn autoship /v /fo list
schtasks /run /tn autoship
schtasks /delete /tn autoship /f
```

</details>

<details><summary><b>macOS — launchd</b></summary>

```bash
scripts/install-launchd.sh \
    --exe /usr/local/bin/autoship \
    --config ~/repos/MyAndroidApp/autoship.yaml \
    --interval 15 --start 09:00 --window 10
```

Installed as a **LaunchAgent** (`~/Library/LaunchAgents/`), not a
LaunchDaemon: it only runs in your logged-in session, which is what Keychain
access and the Gradle/SDK caches need anyway.

```bash
launchctl list | grep dev.autoship
launchctl start dev.autoship.autoship
launchctl unload ~/Library/LaunchAgents/dev.autoship.autoship.plist
```

</details>

<details><summary><b>Linux — systemd timer</b></summary>

```bash
scripts/install-systemd-timer.sh \
    --exe ~/.local/bin/autoship \
    --config ~/repos/MyAndroidApp/autoship.yaml \
    --interval 15 --start 09:00 --window 10
```

Installed as a **systemd --user** timer, for the same reason as the macOS
LaunchAgent: the Secret Service and the Gradle/SDK caches are scoped to your
session. On a headless box, keep the session (and the timer) alive without
staying logged in:

```bash
sudo loginctl enable-linger $USER
```

```bash
systemctl --user list-timers autoship.timer
systemctl --user start autoship.service
systemctl --user disable --now autoship.timer
```

</details>

---

## 6. When it halts

A halt is sticky on purpose: a broken `main` must not trigger a multi-gigabyte
Gradle build every 15 minutes, forever, in the background.

```bash
autoship status --config /path/to/autoship.yaml   # exit code 1 while halted
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

```bash
autoship draft-notes --config /path/to/autoship.yaml            # print
autoship draft-notes --config /path/to/autoship.yaml --write    # write the file
```

Generation stays out of the automated path. If you would rather it fell back to
generated copy instead of halting, that is a config edit and no code change:

```yaml
notes:
  source: [file, commits]
```

# Spec — Android Release Automation (`autoship`)

**Status:** draft
**Date:** 2026-08-17
**Source:** handwritten requirements, 17 Aug 2026
**Supersedes:** the manual `releasing-app` skill (which becomes the fallback / human path)

---

## 1. Problem

Cutting an Android release today is a manual skill run: check out `main`, run tests, run lint,
read `app/build.gradle.kts` for the version, strip `-SNAPSHOT`, build the AAB, hand-assemble a
versioned artifact folder, write two sets of release notes, rename screenshots, fill in the Play
Console form, upload, then bump the version back to the next `-SNAPSHOT`.

Every step is deterministic given the repo state, but a human has to be present and awake for all
of them. The cost isn't difficulty, it's *attendance* — and the pipeline only runs when someone
remembers to run it, so `main` accumulates unreleased changes.

**What we want:** `main` moves → a closed-testing build reaches testers, unattended, on a machine
that isn't paying for the privilege while nothing is happening.

---

## 2. Scope

### In scope

| # | Requirement (from notes) | Interpretation |
|---|---|---|
| R1 | Watch `main` for new changes, deploy if any | Poll `origin/main` SHA; act only on change |
| R2 | Automatically build & test | `testDebugUnitTest`, `lintRelease`, `bundleRelease` |
| R3 | Validate UI | A pass/fail gate — see §6, this is the contested one |
| R4 | Create Play Store assets & notes | *Collect and assemble*; not *invent* — see §7 |
| R5 | Upload assets & description for non-patch | Semver delta decides; patch = AAB + notes only |
| R6 | Version from main's versionName / versionCode | Repo is the source of truth, tool never invents a version |
| R7 | Ship to closed testing | Play track, not production |
| R8 | Fast, reliable, low resources | See §9 for the budget this is held to |

### Non-goals (v1)

- **Production track promotion.** Closed testing is the terminal state. A human promotes.
- **Generating screenshots.** The tool consumes screenshots from a known path; it does not drive
  an emulator to capture them. See §7.2.
- **Writing customer-facing release copy from scratch.** See §7.1 — this is the single biggest
  honest limit on "fully automatic" and is called out rather than papered over.
- **Multi-repo / multi-app.** One config, one app. Config shape allows a second later.
- **Feature graphics, video walkthroughs.** Optional assets stay manual (they already are, per the
  existing skill).
- **Rollback automation.** Detection and halt, yes. Automated un-shipping, no.

---

## 3. Shape of the system

A single Go binary, `autoship.exe`, invoked by **Windows Task Scheduler** on an interval. Not a
daemon. It runs, decides, possibly works, exits.

This is a direct consequence of R8: a job that is idle ~99% of the time should not hold RAM to be
idle in. A scheduled one-shot has *zero* resident cost between runs, which no daemon design can
match.

```
Task Scheduler (every N min)
        │
        ▼
   ┌─────────┐   no change
   │ S0 gate ├──────────────► exit 0   (~300ms, ~12MB, no JVM)
   └────┬────┘
        │ new SHA on origin/main
        ▼
   S1 preflight    version, keystore, versionCode sanity
        ▼
        pin         strip -SNAPSHOT in the Gradle file, commit
        ▼
   S2 build+test   gradlew test → lint → bundleRelease
        ▼
   S3 ui validate  §6
        ▼
   S4 assemble     artifact folder, notes, assets
        ▼
   S5 publish      Play Publisher API → closed testing
        ▼
   S6 post         tag, bump to next -SNAPSHOT, commit, push
```

The pin between S1 and S2 exists because Gradle stamps whatever `versionName` the file
declares into the manifest. Stripping `-SNAPSHOT` only in autoship's own release record would
publish `1.0.6-SNAPSHOT` to Play while tagging `v1.0.6`. Committing the pin also means the tag
S6 creates points at a commit whose checkout reproduces what shipped; a dry run writes the same
file so its build is representative, then restores it.

That pin is deliberately a second commit rather than being squashed or amended into S6's bump.
The two commits differ in exactly the two lines the release turns over, and one commit cannot
hold both values: squashing them puts the next `-SNAPSHOT` in the tagged tree, and amending
leaves the tag pointing at a commit that is no longer an ancestor of the branch — invisible to
`git log` and `git describe`. A repo that does not use `-SNAPSHOT` produces one commit anyway,
since there is nothing to pin.

**The critical design property is S0.** The overwhelmingly common invocation is "nothing changed."
That path must never start a JVM, never touch Gradle, and never allocate meaningfully. It is a
`git fetch` plus a string comparison.

### 3.1 Why Go

Confirming the note's own question: yes, but for reasons other than the one implied.

Go will not reduce peak resource usage — the Gradle daemon (2–4 GB) dominates any run that does
real work, and the orchestrator's footprint is rounding error against it. What Go actually buys:

- **Official Play client.** `google.golang.org/api/androidpublisher/v3` covers the entire edit
  lifecycle: create edit, upload bundle, assign track, set release notes, patch listing, commit.
  No Ruby, no Node, no dependency tree to keep alive on Windows.
- **Single static binary.** Nothing to install, nothing to activate, no runtime drift. Task
  Scheduler points at one `.exe`.
- **Sub-second cold start.** This matters *specifically* because of S0. A Python equivalent pays
  ~150–400ms of interpreter and import cost on every no-op tick; Go pays ~5ms. Across a
  15-minute schedule that difference is irrelevant in absolute terms but it is the difference
  between a gate that is free and one that isn't.

---

## 4. Configuration

`autoship.yaml`, kept in the Android repo (versioned, no secrets):

```yaml
repo:
  path: C:\Users\you\repos\MyAndroidApp
  branch: main
  remote: origin

app:
  module: ":app"
  package: com.example.myapp
  gradle_file: app/build.gradle.kts

gradle:
  unit_tests:  ":app:testDebugUnitTest"
  lint:        ":app:lintRelease"
  bundle:      ":app:bundleRelease"
  apk:         ":app:assembleRelease"   # optional, for the local archive only

ui_validation:
  mode: jvm                # jvm | emulator | none
  task: ":app:testDebugUnitTest --tests '*ScreenRenderTest'"

play:
  track: alpha             # closed testing
  rollout: draft           # draft | completed  — see §10, starts as draft
  update_listing_on: minor # never | minor | any

artifacts:
  root: ${USERPROFILE}\Documents\MyAndroidApp
  screenshots_from: docs/release/screenshots

notes:
  # Ordered strategy chain. First provider that yields notes wins.
  #   file    — read docs/release/notes/<version>.txt from the repo
  #   commits — render from conventional-commit subjects since last release tag
  source: [file]           # e.g. [file, commits] to fall back instead of halting
  file_path: docs/release/notes/${version}.txt
  commit_template: templates/notes-from-commits.tmpl
  on_exhausted: halt       # halt | skip — behaviour when no provider yields
```

Secrets are **not** in this file. See §8.

---

## 5. State & concurrency

State lives outside the repo, at `%LOCALAPPDATA%\autoship\<repo-hash>\`:

```jsonc
// state.json
{
  "last_processed_sha":        "a1b2c3…",
  "last_published_version_code": 7,
  "last_published_version_name": "1.0.5",
  "status":                    "idle",        // idle | running | halted
  "halted": {
    "stage":  "S2",
    "reason": "unit tests failed: 3 failures",
    "sha":    "a1b2c3…",
    "log":    "…\\logs\\run-2026-08-17T09-14-02.log",
    "at":     "2026-08-17T09:19:41Z"
  },
  "last_run_at": "2026-08-17T09:19:41Z"
}
```

**Lock.** `lock` file holding PID + start time. A build can exceed the schedule interval, so
overlapping invocations are expected, not exceptional. The second invocation checks the lock,
verifies the PID is still alive, and exits 0 silently. A lock whose PID is dead (machine slept,
process killed) is considered stale after a configurable `max_run_duration` and reclaimed.

**Halt is sticky.** Once `status: halted`, subsequent ticks exit immediately without rebuilding.
This is deliberate and is the main reliability/resource decision in the design: a broken `main`
must not trigger a 4 GB Gradle build every 15 minutes, forever, in the background, on the user's
workstation. Clearing requires `autoship resume` (or a new SHA on `main`, configurable — the
argument for auto-clear-on-new-SHA is that a fix push is the natural "try again" signal).

---

## 6. UI validation (R3)

The notes say "validate UI" without defining it, and the definition chosen here dominates the
resource budget more than every other decision combined.

| Option | What it catches | Cost per run |
|---|---|---|
| **A. JVM Compose tests** (Robolectric) | Composables render, no crash-on-compose, basic assertions | +30–60s, no extra RAM beyond the JVM already running |
| **B. Gradle Managed Devices** | Real framework behaviour, true instrumented tests | +2.5 GB, +90s boot, flaky on a workstation that also does other things |
| **C. Firebase Test Lab** | Real devices, matrix coverage | ~0 local RAM, but network dependency, GCP account, quota, minutes of latency |

**Recommendation: A for v1.** It runs inside the JVM that S2 already started, adds no process, and
catches the failure class that actually matters for an unattended pipeline — "a screen no longer
renders." B and C both buy fidelity the closed-testing track itself already provides, since the
testers *are* the real-device coverage. Making the automated gate cheap and the human tester pool
the fidelity layer is the correct division for this NFR.

The config keeps `mode: emulator` available so this can be revisited without a redesign.

---

## 7. Assets & notes (R4) — the honest limits

This is where "automatically create Play Store assets & notes" does not survive contact with what
the existing `releasing-app` skill actually requires.

### 7.1 Customer release notes

The skill demands <500 characters of customer-facing benefit-framed copy. That is a writing task,
and it cannot be derived from commit subjects at acceptable quality. Three options:

- **Generate from conventional commits.** Deterministic, free, and produces copy like
  "fix: null check in AliasDao" — which is wrong to put in front of testers.
- **Generate with an LLM call.** Good copy, but injects a non-deterministic network dependency
  into the exact pipeline whose purpose is unattended reliability, and a bad generation ships to
  real people with no review.
- **Require the human to author it. ← recommended**

**Decided (Q2):** the release notes file is an *input*, not an output.
`docs/release/notes/<version>.txt` must exist on `main`. If it doesn't, S4 halts with a clear
message. Writing that file becomes the developer's explicit "this version is ready to ship"
signal — which the pipeline currently lacks entirely, and needs.

A separate, human-invoked `autoship draft-notes` subcommand can generate a first draft from the
commit log for the developer to edit. That keeps generation out of the automated path.

**This policy is deliberately swappable.** It is expressed as a `NotesProvider` strategy chain
(§4, `notes.source`), not as an `if fileMissing { halt }` branch:

```go
type NotesProvider interface {
    // Notes returns the customer-facing copy for a release, or ErrNoNotes
    // if this provider has nothing to offer for it.
    Notes(ctx context.Context, rel Release) (string, error)
}
```

`FileProvider` and `CommitsProvider` both implement it; the configured `source` list is resolved
into a chain at startup and tried in order. Today the chain is `[file]`, which halts when the file
is absent. Moving to `[file, commits]` — generate as a fallback — is a one-line config edit with no
code change, and adding a third strategy later (an LLM drafter, a Jira-driven one) means writing
one type and registering it, not restructuring S4.

The technical notes (`release-notes-technical.md`) *can* be fully generated — commit log, test
counts, lint result, version codes, SDK levels are all mechanically available.

### 7.2 Screenshots

The skill requires screenshots named after the feature they depict
(`screenshot-02-merchant-alias-chaining.png`). Nothing in the repo knows which screen depicts
which feature; that mapping only exists in a human's head, or in an instrumented test that
explicitly declares it.

**Recommendation:** v1 consumes whatever is in `docs/release/screenshots/` and uploads it on
non-patch releases. Generating screenshots is a non-goal, and belongs in the *app* repo as
instrumented tests that name their own captures — a separate piece of work.

### 7.3 Patch vs non-patch (R5)

Semver delta between the version being released and `last_published_version_name`:

- **patch** (1.0.4 → 1.0.5): AAB + customer release notes. No listing, no screenshots.
- **minor / major** (1.0.5 → 1.1.0): additionally patches the store listing (title, short
  description, full description from `docs/release/play_store_listing.md`) and uploads screenshots.

---

## 8. Secrets

Two secrets are needed and neither may sit in plaintext, particularly since a scheduled task runs
non-interactively with a stored credential.

| Secret | Use | Storage |
|---|---|---|
| Play service-account JSON | Publisher API auth | DPAPI-encrypted blob, `CryptProtectData` scoped to the user |
| Keystore password / key alias password | Signing the AAB | Same |

`autoship secrets set` prompts interactively once and writes the encrypted blob. The scheduled run
decrypts in-process. The Play service account is granted only *Release to testing tracks* in Play
Console — it should be incapable of touching production even if compromised.

---

## 9. Resource budget (R8)

The NFR is stated as "fast, reliable, low resources." Concretely, held to:

| Path | Frequency | Wall time | Peak RSS |
|---|---|---|---|
| S0 no-op tick | ~99% of invocations | < 500 ms | ~12 MB |
| Halted tick | while broken | < 50 ms | ~10 MB |
| Full release run | on change to `main` | 4–9 min | 2–4 GB (Gradle-dominated) |
| Full run w/ emulator | if `mode: emulator` | 6–12 min | 5–7 GB |

The only lever the tool itself controls is *how often the expensive path runs*, which is why S0
cheapness and sticky-halt are treated as load-bearing rather than as polish.

---

## 10. Failure modes

| Condition | Behaviour |
|---|---|
| `git fetch` fails (offline, VPN) | Exit 0, no halt. Next tick retries. |
| No new commits | Exit 0. The common case. |
| Unit tests fail | **Halt.** Log path in state, notify. |
| Lint blocking errors | **Halt.** |
| UI validation fails | **Halt.** |
| `versionCode` ≤ last published | **Halt** — the R6 contract is broken and only a human can decide the fix. |
| `versionName` still has `-SNAPSHOT` and equals last released | **Halt** — nothing to release. |
| Release notes file missing | **Halt** (§7.1). |
| Keystore missing / wrong passphrase | **Halt.** |
| Play API 4xx (e.g. edit conflict, bad AAB) | Abort the edit cleanly, **halt**. Never leave a dangling edit. |
| Play API 5xx / timeout | Bounded retry (3×, exponential backoff), then abort edit and halt. |
| Overlapping invocation | Second process exits 0 silently. |
| Machine sleeps mid-run | Stale lock reclaimed after `max_run_duration`; run is *not* auto-resumed — halt and let a human look. |
| Post-release push rejected (main moved) | AAB is already uploaded. **Halt loudly** — this is the one genuinely awkward state, see §11. |

**Notification:** exit code for Task Scheduler's own history, plus a log file, plus an optional
webhook (`notify.url`). The existing sdlc `notify.py` ntfy pattern is the obvious thing to reuse.

---

## 11. The uncomfortable ordering problem

S5 (upload to Play) is irreversible. S6 (tag + version bump + push) can fail *after* it.

If `main` moves between S0 and S6, the post-release push is rejected, and we're left with a
published closed-testing build whose version bump never landed — so the next run would try to
release the same `versionCode` and halt at preflight.

**Mitigation:** S6 rebases the bump commit onto the new `origin/main` and retries once. If it still
fails, halt with an explicit message naming both the published versionCode and the un-pushed
commit, so recovery is a one-line manual push rather than a mystery.

**Accepted:** this window is real and cannot be fully closed without locking `main`, which is worse
than the problem.

---

## 12. Rejected alternatives

- **fastlane (`supply` + `screengrab`).** Genuinely does most of S4–S5 today and is battle-tested.
  Rejected on the NFR: a Ruby toolchain on Windows is a standing maintenance cost, startup is
  seconds not milliseconds, and the no-op gate — the path that runs 99% of the time — would pay
  that cost every tick. If S0 lived in Go and only S5 shelled out to fastlane, this becomes
  defensible; noted as a fallback if the Play API client proves painful.
- **Triple-T `gradle-play-publisher`.** A Gradle plugin that does the Play upload well, and would
  delete most of §8 and S5. Rejected as the *whole* answer because change detection, versioning,
  state, halting, and scheduling still need a driver outside Gradle — but it is a legitimate
  implementation choice *for S5 specifically*, trading Go API code for Gradle config in the app
  repo. **Decided (Q4): Go API client.** Revisit only if the client proves painful in practice.
- **GitHub Actions.** Zero local resource cost and better isolation, but requires the signing
  keystore and Play credentials as cloud secrets, and pays a cold Gradle cache on every run.
  Ruled out by the runtime decision (local Windows machine), not on merit.
- **Long-running daemon with a filesystem/webhook watcher.** Lower latency to react, but holds
  memory 24/7 for a job that is idle almost always, and needs its own supervision story on
  Windows. Task Scheduler already solves supervision, restart, and logging.
- **Trigger on push via a git hook.** Only fires on *this* machine's pushes, misses anything
  merged via the web UI or from elsewhere. Polling is dumber and correct.

---

## 13. Rollout plan

1. **`--dry-run`** — full pipeline including AAB build, everything except the Play edit commit.
   Run it against real changes until the artifact folder is byte-for-byte what the manual skill
   produces.
2. **`rollout: draft`** — real uploads, but the release sits as a draft in Play Console for human
   review before testers see it. Stay here for several releases.
3. **`rollout: completed`** — testers get builds unattended. Only after step 2 has been boring for
   a while.

**Rollback:** halt the rollout in Play Console (manual, one click); the Go side needs no rollback
because its only repo mutation is the S6 bump commit, which is a plain `git revert`.

---

## 14. Open questions

Defaults are chosen so the spec is complete without answers; these are the ones worth overriding.

| # | Question | Default taken |
|---|---|---|
| Q1 | Polling interval? | **15 min**, only during working hours (Task Scheduler handles the window) |
| Q2 | Is "release notes as a required input file" (§7.1) acceptable, or must notes be fully generated? | **Decided — required input file**, halt if missing. Implemented as a swappable `NotesProvider` chain so the policy can change via config alone (§7.1). |
| Q3 | UI validation = JVM Compose tests, or is a real emulator non-negotiable? | **JVM (Robolectric)** |
| Q4 | Play upload via Go API client, or delegate S5 to `gradle-play-publisher`? | **Decided — Go API client.** Keeps the app repo free of publishing config and keeps auth, retry, and edit-abort behaviour (§10) under our control rather than a plugin's. |
| Q5 | Does a new SHA on `main` auto-clear a halt, or is `autoship resume` always required? | **Auto-clear on new SHA** — a fix push is the natural retry signal |
| Q6 | Which app repo is target #1, and is `main` protected / does it receive direct pushes? | **MyAndroidApp**, assumed direct pushes allowed (affects §11) |
| Q7 | Does `autoship` live in this `utils` repo, its own repo, or inside the app repo? | **Its own repo** — it is a tool, not a skill, and will outgrow both |

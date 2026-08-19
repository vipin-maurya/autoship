package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vipinm/autoship/internal/config"
	"github.com/vipinm/autoship/internal/play"
	"github.com/vipinm/autoship/internal/runner"
	"github.com/vipinm/autoship/internal/state"
)

// fakePlay records the edit lifecycle without touching the network.
type fakePlay struct {
	calls []string
}

func (f *fakePlay) rec(name string) error { f.calls = append(f.calls, name); return nil }

func (f *fakePlay) Insert(context.Context) (string, error) { return "edit-1", f.rec("Insert") }
func (f *fakePlay) UploadBundle(context.Context, string, string) (int64, error) {
	return 0, f.rec("UploadBundle")
}
func (f *fakePlay) TrackUpdate(context.Context, string, play.Track) error {
	return f.rec("TrackUpdate")
}
func (f *fakePlay) PatchListing(context.Context, string, play.Listing) error {
	return f.rec("PatchListing")
}
func (f *fakePlay) UploadImage(_ context.Context, _, _, _, _ string) error {
	return f.rec("UploadImage")
}
func (f *fakePlay) Commit(context.Context, string) error { return f.rec("Commit") }
func (f *fakePlay) Delete(context.Context, string) error { return f.rec("Delete") }

func (f *fakePlay) did(name string) bool {
	for _, c := range f.calls {
		if c == name {
			return true
		}
	}
	return false
}

// fixture is a complete, self-contained release environment: an origin repo, a
// clone configured as the release repo, an autoship.yaml, a state root, and
// spies standing in for Gradle and Play.
type fixture struct {
	t          *testing.T
	Origin     string
	Clone      string
	ConfigPath string
	Root       string
	Runner     *runner.Spy
	Play       *fakePlay
	Store      state.Store
}

const gradleKts = `android {
    defaultConfig {
        versionCode = 8
        versionName = "1.0.5-SNAPSHOT"
    }
}
`

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %s in %s: %v: %s", strings.Join(args, " "), dir, err, out)
	}
	return strings.TrimSpace(string(out))
}

func configureRepo(t *testing.T, dir string) {
	t.Helper()
	git(t, dir, "config", "user.email", "autoship@example.test")
	git(t, dir, "config", "user.name", "autoship test")
	git(t, dir, "config", "commit.gpgsign", "false")
	git(t, dir, "config", "receive.denyCurrentBranch", "updateInstead")
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	root := t.TempDir()
	origin := filepath.Join(root, "origin")
	clone := filepath.Join(root, "clone")

	writeFile(t, filepath.Join(origin, "app", "build.gradle.kts"), gradleKts)
	writeFile(t, filepath.Join(origin, "docs", "release", "notes", "1.0.5.txt"),
		"Faster search and fewer duplicate merchants.\n")
	writeFile(t, filepath.Join(origin, "docs", "release", "play_store_listing.md"),
		"## Store Listing Details\n- **App Name**: MyAndroidApp\n- **Short Description**: Local expense tracking.\n- **Full Description**: On-device personal finance.\n")
	writeFile(t, filepath.Join(origin, "docs", "release", "screenshots", "screenshot-01-home.png"), "png")

	git(t, root, "init", "--initial-branch=main", origin)
	configureRepo(t, origin)
	git(t, origin, "add", ".")
	git(t, origin, "commit", "-m", "feat: initial")
	git(t, root, "clone", origin, clone)
	configureRepo(t, clone)

	// The Gradle spy never produces a bundle, so stand one in where the real
	// bundleRelease task would leave it.
	writeFile(t, filepath.Join(clone, "app", "build", "outputs", "bundle", "release", "app-release.aab"),
		"pretend this is a bundle")

	stateRoot := filepath.Join(root, "state")
	cfgPath := filepath.Join(root, "autoship.yaml")
	writeFile(t, cfgPath, fmt.Sprintf(`repo:
  path: %s
  branch: main
  remote: origin
app:
  module: ":app"
  package: com.example.myapp
  gradle_file: app/build.gradle.kts
gradle:
  unit_tests: ":app:testDebugUnitTest"
  lint: ":app:lintRelease"
  bundle: ":app:bundleRelease"
ui_validation:
  mode: jvm
  task: ":app:testDebugUnitTest --tests '*ScreenRenderTest'"
play:
  track: alpha
  rollout: draft
  update_listing_on: minor
artifacts:
  root: %s
  screenshots_from: docs/release/screenshots
notes:
  source: [file]
  file_path: docs/release/notes/${version}.txt
  on_exhausted: halt
`, yamlPath(clone), yamlPath(filepath.Join(root, "artifacts"))))

	fx := &fixture{
		t:          t,
		Origin:     origin,
		Clone:      clone,
		ConfigPath: cfgPath,
		Root:       stateRoot,
		Runner:     &runner.Spy{},
		Play:       &fakePlay{},
		Store:      state.Store{Dir: state.DirFor(stateRoot, clone)},
	}
	return fx
}

// yamlPath escapes a Windows path for a double-quoted YAML scalar.
func yamlPath(p string) string {
	return `"` + strings.ReplaceAll(p, `\`, `\\`) + `"`
}

func (f *fixture) deps() deps {
	return deps{
		Root:      f.Root,
		NewRunner: func(io.Writer) runner.Runner { return f.Runner },
		NewPlayClient: func(context.Context, *config.Config, string) (play.EditClient, error) {
			return f.Play, nil
		},
	}
}

// run invokes the run (or dry-run) command against the fixture.
func (f *fixture) run(dryRun bool) (code int, stdout, stderr string) {
	f.t.Helper()
	var out, errOut bytes.Buffer
	code = runCmd([]string{"--config", f.ConfigPath, "--quiet"}, &out, &errOut, dryRun, f.deps())
	return code, out.String(), errOut.String()
}

// saveState persists a state for the fixture's repo.
func (f *fixture) saveState(st state.State) {
	f.t.Helper()
	if err := f.Store.Save(st); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fixture) loadState() state.State {
	f.t.Helper()
	st, err := f.Store.Load()
	if err != nil {
		f.t.Fatal(err)
	}
	return st
}

func (f *fixture) headSHA() string { return git(f.t, f.Origin, "rev-parse", "HEAD") }

// TestRun_NoChangeDoesNotInvokeGradle is the executable form of the spec's core
// property (§3): the overwhelmingly common invocation must never start a JVM.
func TestRun_NoChangeDoesNotInvokeGradle(t *testing.T) {
	fx := newFixture(t)
	fx.saveState(state.State{LastProcessedSHA: fx.headSHA(), Status: state.StatusIdle})

	code, _, stderr := fx.run(false)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, stderr)
	}
	if fx.Runner.Count() != 0 {
		t.Errorf("gradle was invoked %d times on a no-change tick: %v", fx.Runner.Count(), fx.Runner.ArgLines())
	}
	if len(fx.Play.calls) != 0 {
		t.Errorf("play was called on a no-change tick: %v", fx.Play.calls)
	}
}

func TestRun_HaltedTickDoesNothing(t *testing.T) {
	fx := newFixture(t)
	st := state.State{LastProcessedSHA: fx.headSHA()}
	state.SetHalt(&st, "S2", "unit tests failed", fx.headSHA(), "log.txt", time.Now())
	fx.saveState(st)

	code, _, _ := fx.run(false)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if fx.Runner.Count() != 0 {
		t.Errorf("a halted tick ran gradle: %v", fx.Runner.ArgLines())
	}
	if !fx.loadState().IsHalted() {
		t.Error("halt was cleared without a new commit")
	}
}

func TestRun_ExitsZeroWhenAlreadyLocked(t *testing.T) {
	fx := newFixture(t)
	// A lock held by this very (live) process is the same situation as a lock
	// held by an overlapping run.
	lock, err := fx.Store.Acquire(time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Release()

	code, _, stderr := fx.run(false)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if stderr != "" {
		t.Errorf("stderr = %q, want nothing: an overlapping run is expected, not an error", stderr)
	}
	if fx.Runner.Count() != 0 {
		t.Errorf("a locked-out run invoked gradle: %v", fx.Runner.ArgLines())
	}
}

func TestDryRun_SkipsPublishAndPostRelease(t *testing.T) {
	fx := newFixture(t)
	originHead := fx.headSHA()

	code, _, stderr := fx.run(true)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, stderr)
	}

	// The build really ran.
	wantTasks := []string{":app:testDebugUnitTest", ":app:lintRelease", ":app:bundleRelease"}
	got := fx.Runner.ArgLines()
	if len(got) < len(wantTasks) {
		t.Fatalf("gradle calls = %v, want at least %v", got, wantTasks)
	}
	for i, want := range wantTasks {
		if got[i] != want {
			t.Errorf("gradle call %d = %q, want %q", i, got[i], want)
		}
	}

	// The artifact folder really was assembled.
	folder := filepath.Join(filepath.Dir(fx.Clone), "artifacts", "v1.0.5")
	if _, err := os.Stat(filepath.Join(folder, "app-release.aab")); err != nil {
		t.Errorf("bundle not assembled: %v", err)
	}
	if _, err := os.Stat(filepath.Join(folder, "release-notes-customer.txt")); err != nil {
		t.Errorf("customer notes not written: %v", err)
	}
	if _, err := os.Stat(filepath.Join(folder, "release-notes-technical.md")); err != nil {
		t.Errorf("technical notes not written: %v", err)
	}

	// Nothing irreversible happened.
	if fx.Play.did("Commit") {
		t.Errorf("dry run committed a play edit: %v", fx.Play.calls)
	}
	if !fx.Play.did("Delete") {
		t.Errorf("dry run left the play edit dangling: %v", fx.Play.calls)
	}
	if after := fx.headSHA(); after != originHead {
		t.Error("dry run pushed to origin")
	}
	if tags := git(t, fx.Origin, "tag", "--list"); strings.TrimSpace(tags) != "" {
		t.Errorf("dry run pushed tags: %q", tags)
	}
	// A dry run must not claim the version was published.
	st := fx.loadState()
	if st.LastPublishedVersionCode != 0 {
		t.Errorf("dry run recorded a published version code: %d", st.LastPublishedVersionCode)
	}
	// ...nor consume the commit, or the real run after it would see no change.
	if st.LastProcessedSHA != "" {
		t.Errorf("dry run marked %s processed", st.LastProcessedSHA)
	}
	if code, _, stderr := fx.run(false); code != 0 {
		t.Fatalf("run after a dry run = %d, stderr:\n%s", code, stderr)
	}
	if !fx.Play.did("Commit") {
		t.Errorf("run after a dry run published nothing: %v", fx.Play.calls)
	}
}

func TestRun_FullReleasePublishesAndRecordsState(t *testing.T) {
	fx := newFixture(t)

	code, _, stderr := fx.run(false)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, stderr)
	}
	if !fx.Play.did("Commit") {
		t.Errorf("play edit not committed: %v", fx.Play.calls)
	}
	// 1.0.5 follows nothing, so it is a first release: the listing is updated.
	if !fx.Play.did("PatchListing") {
		t.Errorf("first release did not update the listing: %v", fx.Play.calls)
	}

	st := fx.loadState()
	if st.Status != state.StatusIdle {
		t.Errorf("status = %q, want idle", st.Status)
	}
	if st.LastPublishedVersionName != "1.0.5" || st.LastPublishedVersionCode != 8 {
		t.Errorf("published = %s (%d), want 1.0.5 (8)", st.LastPublishedVersionName, st.LastPublishedVersionCode)
	}
	if st.LastProcessedSHA == "" {
		t.Error("LastProcessedSHA not recorded")
	}

	// S6 landed: the tag and the bump are on origin.
	if tags := git(t, fx.Origin, "tag", "--list"); !strings.Contains(tags, "v1.0.5") {
		t.Errorf("origin tags = %q, want v1.0.5", tags)
	}
	raw, err := os.ReadFile(filepath.Join(fx.Clone, "app", "build.gradle.kts"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `versionName = "1.0.6-SNAPSHOT"`) {
		t.Errorf("gradle file not bumped:\n%s", raw)
	}

	// And a second tick now has nothing to do.
	fx.Runner = &runner.Spy{}
	fx.Play = &fakePlay{}
	if code, _, _ := fx.run(false); code != 0 {
		t.Fatalf("second run exit code = %d, want 0", code)
	}
	if fx.Runner.Count() != 0 {
		t.Errorf("second tick rebuilt: %v", fx.Runner.ArgLines())
	}
}

func TestRun_HaltsAndRecordsTheFailingStage(t *testing.T) {
	fx := newFixture(t)
	fx.Runner.ExitFor = runner.FailOnArg(":app:lintRelease", 1)

	code, _, stderr := fx.run(false)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr, "S2") {
		t.Errorf("stderr = %q, want it to name the failing stage", stderr)
	}

	st := fx.loadState()
	if !st.IsHalted() {
		t.Fatal("state is not halted after a build failure")
	}
	if st.Halted.Stage != "S2" {
		t.Errorf("halt stage = %q, want S2", st.Halted.Stage)
	}
	if !strings.Contains(st.Halted.Reason, ":app:lintRelease") {
		t.Errorf("halt reason = %q, want the failing task", st.Halted.Reason)
	}
	if st.Halted.Log == "" {
		t.Error("halt does not record a log path")
	}
	if _, err := os.Stat(st.Halted.Log); err != nil {
		t.Errorf("halt log path is not a real file: %v", err)
	}
	if fx.Play.did("Insert") {
		t.Errorf("a failed build still reached play: %v", fx.Play.calls)
	}

	// Sticky: the same SHA does not rebuild.
	fx.Runner = &runner.Spy{ExitFor: runner.FailOnArg(":app:lintRelease", 1)}
	if code, _, _ := fx.run(false); code != 0 {
		t.Errorf("halted tick exit code = %d, want 0", code)
	}
	if fx.Runner.Count() != 0 {
		t.Errorf("halted tick rebuilt: %v", fx.Runner.ArgLines())
	}
}

func TestRun_HaltsWhenNotesMissing(t *testing.T) {
	fx := newFixture(t)
	// Remove the notes the developer is required to write (spec §7.1).
	git(t, fx.Origin, "rm", "-q", "docs/release/notes/1.0.5.txt")
	git(t, fx.Origin, "commit", "-m", "chore: drop the notes")

	code, _, stderr := fx.run(false)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1 (stderr: %s)", code, stderr)
	}
	st := fx.loadState()
	if !st.IsHalted() || st.Halted.Stage != "S4" {
		t.Fatalf("halt = %+v, want an S4 halt", st.Halted)
	}
	if !strings.Contains(st.Halted.Reason, "1.0.5") {
		t.Errorf("halt reason = %q, want the version named", st.Halted.Reason)
	}
	if fx.Play.did("Insert") {
		t.Errorf("published without notes: %v", fx.Play.calls)
	}
}

func TestRun_NewCommitClearsAHalt(t *testing.T) {
	fx := newFixture(t)
	st := state.State{LastProcessedSHA: fx.headSHA()}
	state.SetHalt(&st, "S2", "unit tests failed", fx.headSHA(), "log.txt", time.Now())
	fx.saveState(st)

	// The fix push is the natural retry signal (spec Q5).
	writeFile(t, filepath.Join(fx.Origin, "app", "src", "fix.kt"), "// fixed\n")
	git(t, fx.Origin, "add", ".")
	git(t, fx.Origin, "commit", "-m", "fix: make the tests pass")

	code, _, stderr := fx.run(false)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, stderr)
	}
	if fx.Runner.Count() == 0 {
		t.Error("a new commit did not clear the halt")
	}
	if fx.loadState().IsHalted() {
		t.Error("state is still halted after a successful run")
	}
}

func TestRun_MissingConfigIsAUsageError(t *testing.T) {
	var out, errOut bytes.Buffer
	code := runCmd([]string{"--config", filepath.Join(t.TempDir(), "nope.yaml")}, &out, &errOut, false, deps{Root: t.TempDir()})
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if !strings.Contains(errOut.String(), "nope.yaml") {
		t.Errorf("stderr = %q, want it to name the missing config", errOut.String())
	}
}

func TestRun_WritesAPerRunLog(t *testing.T) {
	fx := newFixture(t)
	fx.saveState(state.State{LastProcessedSHA: fx.headSHA()})
	if code, _, _ := fx.run(false); code != 0 {
		t.Fatal("run failed")
	}
	entries, err := os.ReadDir(fx.Store.LogsDir())
	if err != nil {
		t.Fatalf("read logs dir: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("no log file written")
	}
	raw, err := os.ReadFile(filepath.Join(fx.Store.LogsDir(), entries[0].Name()))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "no change") {
		t.Errorf("log = %q, want the gate decision recorded", raw)
	}
}

func TestRun_LeavesNoLockBehind(t *testing.T) {
	fx := newFixture(t)
	fx.saveState(state.State{LastProcessedSHA: fx.headSHA()})
	if code, _, _ := fx.run(false); code != 0 {
		t.Fatal("run failed")
	}
	if _, err := os.Stat(filepath.Join(fx.Store.Dir, "lock")); err == nil {
		t.Error("lock file survived the run")
	}
}

func TestRun_StateJSONShapeMatchesTheSpec(t *testing.T) {
	fx := newFixture(t)
	fx.saveState(state.State{LastProcessedSHA: fx.headSHA()})
	if code, _, _ := fx.run(false); code != 0 {
		t.Fatal("run failed")
	}
	raw, err := os.ReadFile(filepath.Join(fx.Store.Dir, "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("state.json is not valid json: %v", err)
	}
	for _, key := range []string{"last_processed_sha", "last_published_version_code", "last_published_version_name", "status", "last_run_at"} {
		if _, ok := got[key]; !ok {
			t.Errorf("state.json is missing %q: %s", key, raw)
		}
	}
}

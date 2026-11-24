package fetcher

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
	"time"
)

// useTempRoot points os.TempDir at a private directory, so the sweep under
// test never sees (or deletes) anything real.
func useTempRoot(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, name := range []string{"TMPDIR", "TMP", "TEMP"} {
		t.Setenv(name, dir)
	}
	return dir
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// makeDeadSession builds a session directory the way a server leaves it when
// it is killed: lock file present but no longer locked, one profile inside.
func makeDeadSession(t *testing.T) (session, profile string) {
	t.Helper()
	root := profileRoot()
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	session, err := os.MkdirTemp(root, sessionPrefix)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(session, sessionLockName), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	profile = filepath.Join(session, "profile-1")
	if err := os.MkdirAll(filepath.Join(profile, "Default"), 0o700); err != nil {
		t.Fatal(err)
	}
	return session, profile
}

// startProcessMentioning starts a long-running process whose command line
// contains path, standing in for a Chrome started with --user-data-dir=path.
func startProcessMentioning(t *testing.T, path string) *exec.Cmd {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("needs a POSIX shell")
	}
	cmd := exec.Command("sh", "-c", "sleep 60; : --user-data-dir="+path)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cmd.Process.Kill()
		cmd.Wait()
	})
	return cmd
}

func age(t *testing.T, path string, by time.Duration) {
	t.Helper()
	old := time.Now().Add(-by)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
}

func TestSweepRemovesDeadSession(t *testing.T) {
	useTempRoot(t)
	session, _ := makeDeadSession(t)

	sweepStaleProfiles()

	if exists(session) {
		t.Errorf("dead session %s was not removed", session)
	}
}

func TestSweepKillsChromeOfDeadSession(t *testing.T) {
	useTempRoot(t)
	session, profile := makeDeadSession(t)
	orphan := startProcessMentioning(t, profile+"/")
	bystander := startProcessMentioning(t, filepath.Join(os.TempDir(), "unrelated")+"/")

	sweepStaleProfiles()

	exited := make(chan struct{})
	go func() {
		orphan.Wait()
		close(exited)
	}()
	select {
	case <-exited:
	case <-time.After(5 * time.Second):
		t.Error("process using the dead session's profile was not killed")
	}
	if exists(session) {
		t.Errorf("dead session %s was not removed", session)
	}
	if err := bystander.Process.Signal(syscall.Signal(0)); err != nil {
		t.Errorf("an unrelated process was killed: %v", err)
	}
}

func TestSweepKeepsLiveSession(t *testing.T) {
	useTempRoot(t)
	session, err := newSessionDir()
	if err != nil {
		t.Fatal(err)
	}
	// However old it looks: only the lock decides.
	age(t, session.path, 24*time.Hour)

	sweepStaleProfiles()

	if !exists(session.path) {
		t.Fatal("session of a live process was removed")
	}

	session.remove()
	if exists(session.path) {
		t.Error("remove left the session directory behind")
	}
}

func TestSweepSessionWithoutLockFile(t *testing.T) {
	useTempRoot(t)
	root := profileRoot()
	fresh := filepath.Join(root, sessionPrefix+"fresh")
	old := filepath.Join(root, sessionPrefix+"old")
	for _, dir := range []string{fresh, old} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	age(t, old, 2*unlockedSessionMinAge)

	sweepStaleProfiles()

	if !exists(fresh) {
		t.Error("a session directory that may still be starting up was removed")
	}
	if exists(old) {
		t.Error("an old session directory without lock file was not removed")
	}
}

func TestSweepLegacyProfiles(t *testing.T) {
	tmp := useTempRoot(t)
	leaked := filepath.Join(tmp, "chromedp-runner111")
	fresh := filepath.Join(tmp, "chromedp-runner222")
	inUse := filepath.Join(tmp, "chromedp-runner333")
	other := filepath.Join(tmp, "something-else")
	for _, dir := range []string{leaked, fresh, inUse, other} {
		if err := os.MkdirAll(filepath.Join(dir, "Default"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	for _, dir := range []string{leaked, inUse, other} {
		age(t, dir, 2*legacyProfileMinAge)
	}
	startProcessMentioning(t, inUse)

	sweepStaleProfiles()

	if exists(leaked) {
		t.Error("leaked legacy profile was not removed")
	}
	if !exists(fresh) {
		t.Error("recently created legacy profile was removed")
	}
	if !exists(inUse) {
		t.Error("legacy profile used by a running process was removed")
	}
	if !exists(other) {
		t.Error("an unrelated directory was removed")
	}
}

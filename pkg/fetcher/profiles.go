package fetcher

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Chrome profile directories and how they get cleaned up.
//
// Every server process that starts Chrome owns one session directory:
//
//	<tmp>/url-fetcher-chrome/session-XXXX/
//	    lock              held (locked) by the owning process while it lives
//	    profile-XXXX/     --user-data-dir of one Chrome instance
//
// A normal shutdown removes the session directory. A process that is killed
// outright (SIGKILL, crash, power loss) cannot do that, and on macOS and
// Windows its Chrome children keep running. The lock is released by the
// kernel when the owner dies, whatever the cause, so the next server to start
// can tell that the session is dead, kill the Chrome processes still using
// it and delete it. That is sweepStaleProfiles.

const (
	profileRootName = "url-fetcher-chrome"
	sessionPrefix   = "session-"
	sessionLockName = "lock"

	// legacyProfilePattern matches the temporary profiles that chromedp
	// creates itself. Versions of this server up to 2026-11 used those and
	// leaked them whenever the server was killed.
	legacyProfilePattern = "chromedp-runner*"

	// A session directory without a lock file belongs to a process that is
	// either between creating the directory and creating the lock, or died
	// there. Only after this long is it taken for dead.
	unlockedSessionMinAge = time.Minute

	// A legacy profile younger than this may belong to a Chrome that is
	// still being started and does not show up in the process list yet.
	legacyProfileMinAge = 10 * time.Minute
)

func profileRoot() string {
	return filepath.Join(os.TempDir(), profileRootName)
}

// sessionDir is the directory that holds this process's Chrome profiles.
type sessionDir struct {
	path string
	lock *os.File
}

// newSessionDir creates a session directory and takes its lock.
func newSessionDir() (*sessionDir, error) {
	root := profileRoot()
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, err
	}

	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		path, err := os.MkdirTemp(root, sessionPrefix)
		if err != nil {
			return nil, err
		}
		lock, err := os.OpenFile(filepath.Join(path, sessionLockName), os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
		if err == nil {
			if err = lockSessionFile(lock); err == nil {
				return &sessionDir{path: path, lock: lock}, nil
			}
			lock.Close()
		}
		// A sweep in another process got in between creating the lock file
		// and locking it and now treats this directory as dead. Leave it to
		// that sweep and start over with a new one.
		lastErr = err
	}
	return nil, fmt.Errorf("could not lock a session directory: %w", lastErr)
}

// remove deletes the session directory with everything in it.
func (s *sessionDir) remove() {
	s.lock.Close() // first: Windows cannot delete an open file
	removeAllRetry(s.path)
}

// removeAllRetry deletes a directory tree, retrying for a moment: Chrome's
// helper processes can still be writing to the profile right after the
// browser process was killed.
func removeAllRetry(path string) error {
	var err error
	for attempt := 0; attempt < 5; attempt++ {
		if err = os.RemoveAll(path); err == nil {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return err
}

// claimDeadSession reports whether the process that owned a session directory
// is gone. If so, the caller owns the directory until it calls release.
func claimDeadSession(path string) (release func(), dead bool) {
	lockPath := filepath.Join(path, sessionLockName)
	if _, err := os.Stat(lockPath); err != nil {
		info, err := os.Stat(path)
		if err != nil || time.Since(info.ModTime()) < unlockedSessionMinAge {
			return nil, false
		}
		return func() {}, true
	}
	return claimSessionLock(lockPath)
}

// sweepStaleProfiles removes what earlier server processes left behind when
// they died without shutting down: their session directories, any Chrome
// still running on them, and profile directories leaked by older versions.
func sweepStaleProfiles() {
	type deadSession struct {
		path    string
		release func()
	}
	var sessions []deadSession

	entries, _ := os.ReadDir(profileRoot())
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), sessionPrefix) {
			continue
		}
		path := filepath.Join(profileRoot(), entry.Name())
		if release, dead := claimDeadSession(path); dead {
			sessions = append(sessions, deadSession{path, release})
		}
	}

	var legacy []string
	matches, _ := filepath.Glob(filepath.Join(os.TempDir(), legacyProfilePattern))
	for _, path := range matches {
		info, err := os.Stat(path)
		if err == nil && info.IsDir() && time.Since(info.ModTime()) >= legacyProfileMinAge {
			legacy = append(legacy, path)
		}
	}

	if len(sessions) == 0 && len(legacy) == 0 {
		return
	}

	commands, err := runningCommands()

	for _, session := range sessions {
		// Chrome and its helpers all carry --user-data-dir=<session>/profile-…
		inSession := session.path + string(filepath.Separator)
		for pid, command := range commands {
			if strings.Contains(command, inSession) {
				killProcess(pid)
			}
		}
		removeAllRetry(session.path)
		session.release()
	}

	if err != nil {
		// Without the process list there is no telling which legacy profiles
		// are still in use (by an older server that is still running, or by
		// another chromedp program). Leave them all alone.
		return
	}
	for _, path := range legacy {
		if !mentionedIn(commands, path) {
			os.RemoveAll(path)
		}
	}
}

func mentionedIn(commands map[int]string, path string) bool {
	for _, command := range commands {
		if strings.Contains(command, path) {
			return true
		}
	}
	return false
}

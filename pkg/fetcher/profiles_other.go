//go:build !(darwin || linux || freebsd || netbsd || openbsd || dragonfly)

package fetcher

import (
	"errors"
	"os"
	"runtime"
)

// lockSessionFile has nothing to do here. On Windows the open handle is the
// lock: a file that another process holds open cannot be deleted.
func lockSessionFile(f *os.File) error {
	return nil
}

// claimSessionLock decides whether a session's owner is dead by trying to
// delete its lock file, which on Windows fails while the owner has it open.
// On any other platform there is no safe test, so nothing is ever claimed.
func claimSessionLock(lockPath string) (release func(), dead bool) {
	if runtime.GOOS != "windows" {
		return nil, false
	}
	if err := os.Remove(lockPath); err != nil && !os.IsNotExist(err) {
		return nil, false
	}
	return func() {}, true
}

// runningCommands is not available here, so the sweep neither kills orphaned
// Chrome processes nor touches legacy profiles on this platform. A dead
// session's directory is still removed as far as Chrome's open files allow.
func runningCommands() (map[int]string, error) {
	return nil, errors.New("process list not available on " + runtime.GOOS)
}

func killProcess(pid int) {}

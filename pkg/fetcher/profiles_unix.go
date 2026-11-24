//go:build darwin || linux || freebsd || netbsd || openbsd || dragonfly

package fetcher

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

// lockSessionFile takes an exclusive flock on the session lock file. The
// kernel drops it when this process exits, however it exits. Chrome does not
// inherit it: Go opens files close-on-exec.
func lockSessionFile(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
}

// claimSessionLock tries to take the lock of another process's session. That
// succeeds only when the owner is dead. The lock is then held until release,
// which keeps other sweeps away from the directory meanwhile.
func claimSessionLock(lockPath string) (release func(), dead bool) {
	f, err := os.OpenFile(lockPath, os.O_RDWR, 0)
	if err != nil {
		return nil, false
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return nil, false
	}
	return func() { f.Close() }, true
}

// runningCommands returns the command line of every running process by pid.
func runningCommands() (map[int]string, error) {
	out, err := exec.Command("ps", "-axww", "-o", "pid=,command=").Output()
	if err != nil {
		return nil, err
	}
	commands := make(map[int]string)
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.SplitN(strings.TrimSpace(line), " ", 2)
		if len(fields) != 2 {
			continue
		}
		pid, err := strconv.Atoi(fields[0])
		if err != nil {
			continue
		}
		commands[pid] = fields[1]
	}
	return commands, nil
}

func killProcess(pid int) {
	syscall.Kill(pid, syscall.SIGKILL)
}

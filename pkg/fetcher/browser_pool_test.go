package fetcher

import (
	"errors"
	"os"
	"testing"
	"time"
)

// These tests start real Chrome processes.
func requireChrome(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("starts Chrome; skipped in short mode")
	}
	if !checkChromeAvailable() {
		t.Skip("Chrome is not installed")
	}
}

func sessionCount(t *testing.T) int {
	t.Helper()
	entries, err := os.ReadDir(profileRoot())
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return len(entries)
}

func waitFor(t *testing.T, what string, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestPoolStartsChromeOnDemandAndCleansUp(t *testing.T) {
	requireChrome(t)
	useTempRoot(t)

	pool := newBrowserPool(2, 0)
	if n := sessionCount(t); n != 0 {
		t.Fatalf("a new pool created %d session directories before any fetch", n)
	}

	first, err := pool.acquire()
	if err != nil {
		t.Fatal(err)
	}
	if !first.alive() || !exists(first.dir) {
		t.Fatal("acquired browser is not running on its profile directory")
	}
	pool.release(first)

	again, err := pool.acquire()
	if err != nil {
		t.Fatal(err)
	}
	if again != first {
		t.Error("an idle browser was not reused")
	}
	pool.release(again)

	pool.Close()
	pool.Close() // must be harmless

	if first.alive() {
		t.Error("Chrome still running after Close")
	}
	if n := sessionCount(t); n != 0 {
		t.Errorf("%d session directories left after Close", n)
	}
	if _, err := pool.acquire(); !errors.Is(err, errPoolClosed) {
		t.Errorf("acquire after Close: got %v, want errPoolClosed", err)
	}
}

func TestPoolCloseWhileBrowserInUse(t *testing.T) {
	requireChrome(t)
	useTempRoot(t)

	pool := newBrowserPool(1, 0)
	b, err := pool.acquire()
	if err != nil {
		t.Fatal(err)
	}

	pool.Close()

	if b.alive() {
		t.Error("Chrome that was in use survived Close")
	}
	if n := sessionCount(t); n != 0 {
		t.Errorf("%d session directories left after Close", n)
	}
	pool.release(b) // the fetch that was using it returns afterwards
}

func TestPoolShutsDownIdleChrome(t *testing.T) {
	requireChrome(t)
	useTempRoot(t)

	pool := newBrowserPool(1, 200*time.Millisecond)
	defer pool.Close()

	first, err := pool.acquire()
	if err != nil {
		t.Fatal(err)
	}
	pool.release(first)

	waitFor(t, "idle Chrome to be shut down", func() bool {
		return !first.alive() && sessionCount(t) == 0
	})

	second, err := pool.acquire()
	if err != nil {
		t.Fatalf("no browser after the idle one was shut down: %v", err)
	}
	if second == first || !second.alive() {
		t.Error("expected a freshly started browser")
	}
	pool.release(second)
}

func TestPoolReplacesChromeThatDied(t *testing.T) {
	requireChrome(t)
	useTempRoot(t)

	pool := newBrowserPool(1, 0)
	defer pool.Close()

	first, err := pool.acquire()
	if err != nil {
		t.Fatal(err)
	}
	pool.release(first)

	first.cancel() // what chromedp does when Chrome crashes or is killed

	second, err := pool.acquire()
	if err != nil {
		t.Fatal(err)
	}
	if second == first || !second.alive() {
		t.Error("a dead browser was handed out again")
	}
	pool.release(second)

	waitFor(t, "the dead browser's profile to be removed", func() bool {
		return !exists(first.dir)
	})
}

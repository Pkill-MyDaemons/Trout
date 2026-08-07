package main

import (
	"path/filepath"
	"testing"
)

// TestAcquireDaemonLockPreventsSecondInstance is a regression test for the
// root cause of duplicate LLM calls: two daemon processes both picking up
// and processing the same task (each running its own full tool-use loop,
// doubling token usage). acquireDaemonLock must let only one instance win.
func TestAcquireDaemonLockPreventsSecondInstance(t *testing.T) {
	orig := daemonLockFile
	daemonLockFile = filepath.Join(t.TempDir(), "daemon.lock")
	t.Cleanup(func() { daemonLockFile = orig })

	if err := acquireDaemonLock(); err != nil {
		t.Fatalf("first acquireDaemonLock: %v", err)
	}
	if err := acquireDaemonLock(); err == nil {
		t.Fatal("expected a second acquireDaemonLock call to fail while the first instance holds the lock")
	}
}

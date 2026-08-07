package main

import (
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func newID() string {
	b := make([]byte, 4)
	rand.Read(b)
	return fmt.Sprintf("%x", b)
}

// dataDir returns ~/.task-agent and ensures it exists.
func dataDir() string {
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".task-agent")
	_ = os.MkdirAll(dir, 0700)
	return dir
}

func dataPath(name string) string {
	return filepath.Join(dataDir(), name)
}

func nowTime() time.Time { return time.Now() }

// storeTickMsg drives periodic reloads of tasks.json in the list and detail
// TUI views, so daemon-side writes (agent replies, live activity) show up
// without the user having to press "R". fast picks a tighter interval while
// a task is actively being processed, and a longer one at idle to avoid
// needless disk polling.
type storeTickMsg time.Time

const (
	tickFastInterval = 1 * time.Second
	tickIdleInterval = 5 * time.Second
)

func storeTick(fast bool) tea.Cmd {
	interval := tickIdleInterval
	if fast {
		interval = tickFastInterval
	}
	return tea.Tick(interval, func(t time.Time) tea.Msg { return storeTickMsg(t) })
}

// truncate shortens s to at most max runes, appending an ellipsis if cut.
func truncate(s string, max int) string {
	r := []rune(s)
	if max <= 0 || len(r) <= max {
		return s
	}
	if max <= 1 {
		return string(r[:max])
	}
	return string(r[:max-1]) + "…"
}

// atomicWriteFile writes data to path by writing to a temp file in the same
// directory and renaming it over the target, so a crash mid-write can never
// leave a truncated/corrupt file at path.
func atomicWriteFile(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath) // no-op once renamed

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpPath, perm); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

func daemonLog(format string, args ...any) {
	f, err := os.OpenFile(dataPath("daemon.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "[%s] %s\n", time.Now().Format(time.RFC3339), fmt.Sprintf(format, args...))
}

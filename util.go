package main

import (
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"time"
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

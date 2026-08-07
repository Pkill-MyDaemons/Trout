package main

import (
	"os"
	"path/filepath"
	"testing"
)

// withTempStore points storeFile/storeLockFile at a fresh temp directory for
// the duration of a test, restoring the originals on cleanup. Store tests
// must never touch the real ~/.task-agent data.
func withTempStore(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	origFile, origLock := storeFile, storeLockFile
	storeFile = filepath.Join(dir, "tasks.json")
	storeLockFile = filepath.Join(dir, "tasks.lock")
	t.Cleanup(func() {
		storeFile, storeLockFile = origFile, origLock
	})
}

func TestAddTaskPersists(t *testing.T) {
	withTempStore(t)
	s := &Store{}
	task := s.addTask("Title", "Desc")
	if task.ID == "" {
		t.Fatal("expected non-empty task ID")
	}

	fresh, err := loadStore()
	if err != nil {
		t.Fatalf("loadStore: %v", err)
	}
	if len(fresh.Tasks) != 1 || fresh.Tasks[0].Title != "Title" {
		t.Fatalf("unexpected tasks on disk: %+v", fresh.Tasks)
	}
}

func TestDeleteTask(t *testing.T) {
	withTempStore(t)
	s := &Store{}
	t1 := s.addTask("A", "")
	s.addTask("B", "")
	s.deleteTask(t1.ID)

	fresh, err := loadStore()
	if err != nil {
		t.Fatalf("loadStore: %v", err)
	}
	if len(fresh.Tasks) != 1 || fresh.Tasks[0].Title != "B" {
		t.Fatalf("expected only task B to remain, got %+v", fresh.Tasks)
	}
}

func TestCycleStatus(t *testing.T) {
	withTempStore(t)
	s := &Store{}
	task := s.addTask("A", "")
	if task.Status != StatusTodo {
		t.Fatalf("expected initial status todo, got %s", task.Status)
	}
	if got := s.cycleStatus(task.ID); got.Status != StatusInProgress {
		t.Fatalf("expected in_progress, got %s", got.Status)
	}
	if got := s.cycleStatus(task.ID); got.Status != StatusDone {
		t.Fatalf("expected done, got %s", got.Status)
	}
	if got := s.cycleStatus(task.ID); got.Status != StatusTodo {
		t.Fatalf("expected todo again, got %s", got.Status)
	}
}

func TestFindTask(t *testing.T) {
	withTempStore(t)
	s := &Store{}
	task := s.addTask("A", "")
	if s.findTask(task.ID) == nil {
		t.Fatal("expected to find task by id")
	}
	if s.findTask("nonexistent") != nil {
		t.Fatal("expected nil for missing id")
	}
}

// TestMutateStoreDoesNotLoseConcurrentWrite is a regression test for the
// TUI/daemon lost-update race: a long-lived process (e.g. the TUI) that
// loaded the store before another process (e.g. the daemon) wrote to it
// must not clobber that write when it later saves its own, unrelated edit.
func TestMutateStoreDoesNotLoseConcurrentWrite(t *testing.T) {
	withTempStore(t)
	base := &Store{}
	t1 := base.addTask("Task 1", "")
	t2 := base.addTask("Task 2", "")

	// The TUI loads its own snapshot first...
	tui, err := loadStore()
	if err != nil {
		t.Fatalf("loadStore: %v", err)
	}

	// ...then the daemon (a separate in-memory Store) posts an agent
	// reply after that snapshot was taken.
	daemon, err := loadStore()
	if err != nil {
		t.Fatalf("loadStore: %v", err)
	}
	daemon.addComment(t2.ID, "agent", "agent reply", nil, nil)

	// The TUI, still holding its pre-daemon-write snapshot, now saves an
	// unrelated edit. A blind whole-file overwrite would wipe out the
	// daemon's comment; the reload-merge fix must not.
	tui.cycleStatus(t1.ID)

	fresh, err := loadStore()
	if err != nil {
		t.Fatalf("loadStore: %v", err)
	}
	got1, got2 := fresh.findTask(t1.ID), fresh.findTask(t2.ID)
	if got1 == nil || got1.Status != StatusInProgress {
		t.Fatalf("task 1's status change was lost: %+v", got1)
	}
	if got2 == nil || len(got2.Comments) != 1 {
		t.Fatalf("task 2's daemon comment was lost: %+v", got2)
	}
}

func TestAtomicWriteFileLeavesNoPartialFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.json")

	if err := atomicWriteFile(path, []byte(`{"ok":true}`), 0644); err != nil {
		t.Fatalf("atomicWriteFile: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != `{"ok":true}` {
		t.Fatalf("unexpected content: %s", data)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected exactly one file in dir (no leftover temp file), got %d: %v", len(entries), entries)
	}
}

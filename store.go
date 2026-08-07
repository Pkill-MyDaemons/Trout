package main

import (
	"encoding/json"
	"os"
	"syscall"
	"time"
)

var storeFile = dataPath("tasks.json")
var storeLockFile = dataPath("tasks.lock")

type Status string

const (
	StatusTodo       Status = "todo"
	StatusInProgress Status = "in_progress"
	StatusDone       Status = "done"
)

type Comment struct {
	ID         string      `json:"id"`
	Author     string      `json:"author"` // "user" or "agent"
	Body       string      `json:"body"`
	Files      []string    `json:"files,omitempty"`
	EmailDraft *EmailDraft `json:"email_draft,omitempty"`
	CreatedAt  time.Time   `json:"created_at"`
}

type Task struct {
	ID              string    `json:"id"`
	Title           string    `json:"title"`
	Description     string    `json:"description"`
	Status          Status    `json:"status"`
	CreatedAt       time.Time `json:"created_at"`
	Comments        []Comment `json:"comments"`
	HasUnread       bool      `json:"has_unread"`
	Tags            []string  `json:"tags,omitempty"`
	Processing      bool      `json:"processing,omitempty"`
	CurrentActivity string    `json:"current_activity,omitempty"`
}

type Store struct {
	Tasks  []*Task `json:"tasks"`
	nextID int
}

func loadStore() (*Store, error) {
	data, err := os.ReadFile(storeFile)
	if err != nil {
		if os.IsNotExist(err) {
			return &Store{}, nil
		}
		return nil, err
	}
	var s Store
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func saveStore(s *Store) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return atomicWriteFile(storeFile, data, 0644)
}

// withStoreLock serializes read-modify-write cycles across processes (the
// TUI and the daemon both read/write tasks.json independently) using an
// OS-level advisory lock, so two writers can never interleave their writes.
func withStoreLock(fn func() error) error {
	f, err := os.OpenFile(storeLockFile, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	return fn()
}

// mutateStore loads the freshest copy of the store from disk (under the
// store lock), applies fn to it, saves it, and returns the fresh store.
// Every mutating Store method goes through this instead of saving the
// caller's own (possibly stale) in-memory snapshot, so a long-lived TUI
// session can never clobber writes made by the daemon (or vice versa)
// since the store was last loaded.
func mutateStore(fn func(fresh *Store)) (*Store, error) {
	var fresh *Store
	err := withStoreLock(func() error {
		var err error
		fresh, err = loadStore()
		if err != nil {
			return err
		}
		fn(fresh)
		return saveStore(fresh)
	})
	return fresh, err
}

// findTask returns the task with the given ID from this store's in-memory
// snapshot, or nil. Callers that hold a *Task across a mutating call (e.g.
// the TUI's open task detail view) should re-fetch via this after any
// mutateStore-backed call, since that call replaces s.Tasks with freshly
// loaded (and therefore different) *Task pointers.
func (s *Store) findTask(id string) *Task {
	for _, t := range s.Tasks {
		if t.ID == id {
			return t
		}
	}
	return nil
}

func (s *Store) addTask(title, description string) *Task {
	var t *Task
	fresh, _ := mutateStore(func(fresh *Store) {
		fresh.nextID++
		t = &Task{
			ID:          newID(),
			Title:       title,
			Description: description,
			Status:      StatusTodo,
			CreatedAt:   time.Now(),
		}
		fresh.Tasks = append(fresh.Tasks, t)
	})
	if fresh != nil {
		s.Tasks = fresh.Tasks
	}
	return t
}

func (s *Store) deleteTask(id string) {
	fresh, _ := mutateStore(func(fresh *Store) {
		for i, t := range fresh.Tasks {
			if t.ID == id {
				fresh.Tasks = append(fresh.Tasks[:i], fresh.Tasks[i+1:]...)
				break
			}
		}
	})
	if fresh != nil {
		s.Tasks = fresh.Tasks
	}
}

// cycleStatus advances a task todo -> in_progress -> done -> todo and
// returns the task's fresh pointer (or nil if the task no longer exists).
func (s *Store) cycleStatus(id string) *Task {
	var target *Task
	fresh, _ := mutateStore(func(fresh *Store) {
		for _, t := range fresh.Tasks {
			if t.ID == id {
				switch t.Status {
				case StatusTodo:
					t.Status = StatusInProgress
				case StatusInProgress:
					t.Status = StatusDone
				case StatusDone:
					t.Status = StatusTodo
				}
				target = t
				break
			}
		}
	})
	if fresh != nil {
		s.Tasks = fresh.Tasks
	}
	return target
}

// setTags replaces a task's tags and returns its fresh pointer.
func (s *Store) setTags(id string, tags []string) *Task {
	var target *Task
	fresh, _ := mutateStore(func(fresh *Store) {
		for _, t := range fresh.Tasks {
			if t.ID == id {
				t.Tags = tags
				target = t
				break
			}
		}
	})
	if fresh != nil {
		s.Tasks = fresh.Tasks
	}
	return target
}

// setProcessing marks whether a task currently has an agent run in flight,
// clearing CurrentActivity whenever it's set back to false.
func (s *Store) setProcessing(id string, on bool) {
	fresh, _ := mutateStore(func(fresh *Store) {
		for _, t := range fresh.Tasks {
			if t.ID == id {
				t.Processing = on
				if !on {
					t.CurrentActivity = ""
				}
				break
			}
		}
	})
	if fresh != nil {
		s.Tasks = fresh.Tasks
	}
}

// setActivity records a short human-readable description of the tool call
// the agent is currently running, for display in the TUI.
func (s *Store) setActivity(id, activity string) {
	fresh, _ := mutateStore(func(fresh *Store) {
		for _, t := range fresh.Tasks {
			if t.ID == id {
				t.CurrentActivity = activity
				break
			}
		}
	})
	if fresh != nil {
		s.Tasks = fresh.Tasks
	}
}

// markRead clears a task's unread flag and returns its fresh pointer.
func (s *Store) markRead(id string) *Task {
	var target *Task
	fresh, _ := mutateStore(func(fresh *Store) {
		for _, t := range fresh.Tasks {
			if t.ID == id {
				t.HasUnread = false
				target = t
				break
			}
		}
	})
	if fresh != nil {
		s.Tasks = fresh.Tasks
	}
	return target
}

// markInProgress transitions a task from todo to in_progress (a no-op for
// any other status) and returns its fresh pointer.
func (s *Store) markInProgress(id string) *Task {
	var target *Task
	fresh, _ := mutateStore(func(fresh *Store) {
		for _, t := range fresh.Tasks {
			if t.ID == id {
				if t.Status == StatusTodo {
					t.Status = StatusInProgress
				}
				target = t
				break
			}
		}
	})
	if fresh != nil {
		s.Tasks = fresh.Tasks
	}
	return target
}

func (s *Store) addComment(taskID, author, body string, files []string, draft *EmailDraft) *Task {
	var target *Task
	fresh, _ := mutateStore(func(fresh *Store) {
		for _, t := range fresh.Tasks {
			if t.ID == taskID {
				t.Comments = append(t.Comments, Comment{
					ID:         newID(),
					Author:     author,
					Body:       body,
					Files:      files,
					EmailDraft: draft,
					CreatedAt:  time.Now(),
				})
				if author == "agent" {
					t.HasUnread = true
				}
				target = t
				break
			}
		}
	})
	if fresh != nil {
		s.Tasks = fresh.Tasks
	}
	return target
}

// updateEmailDraft sets the status (and optionally SentAt) on the first pending draft found.
func (s *Store) updateEmailDraft(taskID, status string) {
	fresh, _ := mutateStore(func(fresh *Store) {
		now := time.Now()
		for _, t := range fresh.Tasks {
			if t.ID != taskID {
				continue
			}
			for i := range t.Comments {
				d := t.Comments[i].EmailDraft
				if d != nil && d.Status == "pending" {
					d.Status = status
					if status == "sent" {
						d.SentAt = &now
					}
					return
				}
			}
		}
	})
	if fresh != nil {
		s.Tasks = fresh.Tasks
	}
}

// pendingDraft returns the first pending email draft on a task, or nil.
func (s *Store) pendingDraft(taskID string) *EmailDraft {
	for _, t := range s.Tasks {
		if t.ID != taskID {
			continue
		}
		for _, c := range t.Comments {
			if c.EmailDraft != nil && c.EmailDraft.Status == "pending" {
				return c.EmailDraft
			}
		}
	}
	return nil
}

package main

import (
	"encoding/json"
	"os"
	"time"
)

var storeFile = dataPath("tasks.json")

type Status string

const (
	StatusTodo       Status = "todo"
	StatusInProgress Status = "in_progress"
	StatusDone       Status = "done"
)

type Comment struct {
	ID        string    `json:"id"`
	Author    string    `json:"author"` // "user" or "agent"
	Body      string    `json:"body"`
	Files     []string  `json:"files,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type Task struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Status      Status    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	Comments    []Comment `json:"comments"`
	HasUnread   bool      `json:"has_unread"`
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
	return os.WriteFile(storeFile, data, 0644)
}

func (s *Store) addTask(title, description string) *Task {
	s.nextID++
	t := &Task{
		ID:          newID(),
		Title:       title,
		Description: description,
		Status:      StatusTodo,
		CreatedAt:   time.Now(),
	}
	s.Tasks = append(s.Tasks, t)
	_ = saveStore(s)
	return t
}

func (s *Store) deleteTask(id string) {
	for i, t := range s.Tasks {
		if t.ID == id {
			s.Tasks = append(s.Tasks[:i], s.Tasks[i+1:]...)
			break
		}
	}
	_ = saveStore(s)
}

func (s *Store) addComment(taskID, author, body string, files []string) {
	for _, t := range s.Tasks {
		if t.ID == taskID {
			t.Comments = append(t.Comments, Comment{
				ID:        newID(),
				Author:    author,
				Body:      body,
				Files:     files,
				CreatedAt: time.Now(),
			})
			if author == "agent" {
				t.HasUnread = true
			}
			break
		}
	}
	_ = saveStore(s)
}

package main

import (
	"fmt"
	"sync"
)

func needsAgentResponse(t *Task) bool {
	if t.Status == StatusDone {
		return false
	}
	if len(t.Comments) == 0 {
		return true
	}
	return t.Comments[len(t.Comments)-1].Author == "user"
}

var (
	inFlightMu  sync.Mutex
	inFlightIDs = map[string]bool{}
)

// tryClaimTask marks a task as being processed and reports whether the
// caller won the claim. The daemon reloads the store from disk on every
// tick, so without this a task whose agent run outlives one tick interval
// (very likely for the 5s "instant" mode) gets picked up again on the next
// tick before the first run has saved its result — spawning duplicate,
// concurrent LLM calls for the same task and burning through rate limits.
func tryClaimTask(id string) bool {
	inFlightMu.Lock()
	defer inFlightMu.Unlock()
	if inFlightIDs[id] {
		return false
	}
	inFlightIDs[id] = true
	return true
}

// releaseTask must be called (via defer) once a claimed task's agent run finishes.
func releaseTask(id string) {
	inFlightMu.Lock()
	defer inFlightMu.Unlock()
	delete(inFlightIDs, id)
}

func processTask(cfg *Config, task *Task, store *Store) {
	if isNoReplySender(task) {
		store.addComment(task.ID, "agent",
			"*Skipped: email is from a no-reply address — automated messages are not processed.*",
			nil, nil)
		return
	}

	workspace := cfg.WorkDir
	if workspace == "" {
		workspace = defaultWorkspace()
	}

	exec, err := newExecutor(workspace)
	if err != nil {
		store.addComment(task.ID, "agent",
			fmt.Sprintf("*Could not set up workspace: %s*", err), nil, nil)
		return
	}
	exec.cfg = cfg

	raw, files, err := callLLMLoop(cfg, task, exec)
	if err != nil {
		store.addComment(task.ID, "agent",
			fmt.Sprintf("*Agent error: %s*", err), nil, nil)
		return
	}

	// Strip file links block (already handled by callLLMLoop / parseAgentResponse)
	body, emailDraft := parseEmailDraft(raw)

	// Mode 3: auto-send immediately
	if emailDraft != nil && cfg.EmailMode == EmailModeAutoSend {
		if sendErr := sendEmail(cfg, emailDraft); sendErr != nil {
			body += fmt.Sprintf("\n\n*Email send failed: %s*", sendErr)
			emailDraft.Status = "rejected"
		} else {
			now := nowTime()
			emailDraft.Status = "sent"
			emailDraft.SentAt = &now
		}
	}

	store.addComment(task.ID, "agent", body, files, emailDraft)

	for _, t := range store.Tasks {
		if t.ID == task.ID && t.Status == StatusTodo {
			t.Status = StatusInProgress
			_ = saveStore(store)
			break
		}
	}
}

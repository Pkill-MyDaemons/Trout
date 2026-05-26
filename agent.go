package main

import "fmt"

func needsAgentResponse(t *Task) bool {
	if t.Status == StatusDone {
		return false
	}
	if len(t.Comments) == 0 {
		return true
	}
	return t.Comments[len(t.Comments)-1].Author == "user"
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

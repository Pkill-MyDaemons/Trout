package main

import "fmt"

// needsAgentResponse returns true if the task needs a response from the agent.
// responsive mode (nightly=false): only tasks where last comment is from user.
// nightly mode: also picks up uncommented non-done tasks.
func needsAgentResponse(t *Task, nightly bool) bool {
	if t.Status == StatusDone {
		return false
	}
	if len(t.Comments) == 0 {
		return nightly
	}
	return t.Comments[len(t.Comments)-1].Author == "user"
}

// processTask runs the agentic loop for a single task and stores the response.
func processTask(cfg *Config, task *Task, store *Store) {
	workspace := cfg.WorkDir
	if workspace == "" {
		workspace = defaultWorkspace()
	}

	exec, err := newExecutor(workspace)
	if err != nil {
		store.addComment(task.ID, "agent",
			fmt.Sprintf("*Could not set up workspace: %s*", err), nil)
		return
	}

	body, files, err := callLLMLoop(cfg, task, exec)
	if err != nil {
		store.addComment(task.ID, "agent",
			fmt.Sprintf("*Agent error: %s*", err), nil)
		return
	}
	store.addComment(task.ID, "agent", body, files)
}

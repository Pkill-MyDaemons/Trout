package main

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// parseRunTitleArgs parses `--run-title <title> [--cwd <dir>] --out <path>`
// (order-independent after the title) into its three components. --out is
// required; --cwd is optional and falls back to cfg.WorkDir / the default
// workspace when empty.
func parseRunTitleArgs(args []string) (title, workDir, outPath string, err error) {
	if len(args) == 0 {
		return "", "", "", fmt.Errorf("usage: trout --run-title <title> --out <path> [--cwd <dir>]")
	}
	title = args[0]
	rest := args[1:]
	for i := 0; i < len(rest); i++ {
		switch rest[i] {
		case "--out":
			if i+1 >= len(rest) {
				return "", "", "", fmt.Errorf("--out requires a path argument")
			}
			outPath = rest[i+1]
			i++
		case "--cwd":
			if i+1 >= len(rest) {
				return "", "", "", fmt.Errorf("--cwd requires a directory argument")
			}
			workDir = rest[i+1]
			i++
		default:
			return "", "", "", fmt.Errorf("unrecognized argument %q", rest[i])
		}
	}
	if outPath == "" {
		return "", "", "", fmt.Errorf("--out <path> is required")
	}
	return title, workDir, outPath, nil
}

// runEphemeralTask runs the same core agent loop processTask uses, but against
// a one-off in-memory Task with no Store involved: no comment thread, no
// status transitions, no no-reply/auto-send bookkeeping. The final markdown
// result is written to outPath instead of being appended to a task's
// comments. Used by `--run-title` for headless/scripted invocations (e.g.
// another tool dispatching work to trout as a backend).
func runEphemeralTask(cfg *Config, title, workDir, outPath string) error {
	task := &Task{
		Title:     title,
		Status:    StatusTodo,
		CreatedAt: time.Now(),
	}

	workspace := workDir
	if workspace == "" {
		workspace = cfg.WorkDir
	}
	if workspace == "" {
		workspace = defaultWorkspace()
	}

	exec, err := newExecutor(workspace)
	if err != nil {
		return fmt.Errorf("could not set up workspace: %w", err)
	}
	exec.cfg = cfg

	raw, files, err := callLLMLoop(cfg, task, exec)
	if err != nil {
		return fmt.Errorf("agent error: %w", err)
	}

	body, emailDraft := parseEmailDraft(raw)

	var out strings.Builder
	out.WriteString(body)
	out.WriteString("\n")

	if len(files) > 0 {
		out.WriteString("\n## Files written\n\n")
		for _, f := range files {
			out.WriteString(fmt.Sprintf("- %s\n", f))
		}
	}

	if emailDraft != nil {
		out.WriteString("\n## Draft email\n\n")
		out.WriteString(fmt.Sprintf("**To:** %s\n\n**Subject:** %s\n\n%s\n", emailDraft.To, emailDraft.Subject, emailDraft.Body))
	}

	if err := os.WriteFile(outPath, []byte(out.String()), 0644); err != nil {
		return fmt.Errorf("could not write output: %w", err)
	}
	return nil
}

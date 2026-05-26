package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// defaultWorkspace returns ~/.task-agent/projects.
func defaultWorkspace() string {
	return dataPath("projects")
}

// sensitiveBlocks are path fragments that are always denied, even inside the workspace.
var sensitiveBlocks = []string{
	".ssh", ".aws", ".gnupg", ".netrc", ".kube",
	"id_rsa", "id_ed25519", "id_ecdsa", "id_dsa",
	".env", "credentials", "secrets", "private_key",
	"keychain", ".config/gcloud", ".config/gh",
	"token", "password",
}

// dangerousCmds are shell patterns that are always denied.
var dangerousCmds = []string{
	"sudo ", "su ", "rm -rf /", "mkfs", "dd if=",
	"> /etc", "> /usr", "> /bin", "> /sbin",
	":(){ :|:& };:", // fork bomb
	"curl | sh", "wget | sh", "curl|sh", "wget|sh",
}

// Executor runs tools inside the workspace with security checks.
type Executor struct {
	workspace string
	written   []string // files written this session
}

func newExecutor(workspace string) (*Executor, error) {
	if err := os.MkdirAll(workspace, 0755); err != nil {
		return nil, err
	}
	abs, err := filepath.Abs(workspace)
	if err != nil {
		return nil, err
	}
	return &Executor{workspace: abs}, nil
}

// resolveSafe resolves path inside the workspace and checks safety.
// Returns a clear BLOCKED error if denied; the caller must send this to the LLM.
func (e *Executor) resolveSafe(path string) (string, error) {
	var abs string
	if filepath.IsAbs(path) {
		abs = filepath.Clean(path)
	} else {
		abs = filepath.Clean(filepath.Join(e.workspace, path))
	}

	wsPrefix := e.workspace + string(filepath.Separator)
	if abs != e.workspace && !strings.HasPrefix(abs, wsPrefix) {
		return "", fmt.Errorf(
			"BLOCKED: path %q is outside the workspace (%s) — access denied",
			path, e.workspace,
		)
	}

	lower := strings.ToLower(abs)
	for _, pattern := range sensitiveBlocks {
		if strings.Contains(lower, pattern) {
			return "", fmt.Errorf(
				"BLOCKED: path %q matches sensitive pattern %q — access denied",
				path, pattern,
			)
		}
	}
	return abs, nil
}

// Dispatch executes the named tool with the given input map.
// Security violations are returned as errors; the caller forwards them to the LLM as tool errors.
func (e *Executor) Dispatch(name string, input map[string]any) (string, error) {
	str := func(k string) string { v, _ := input[k].(string); return v }
	switch name {
	case "read_file":
		return e.readFile(str("path"))
	case "write_file":
		return e.writeFile(str("path"), str("content"))
	case "run_command":
		return e.runCommand(str("command"))
	case "list_files":
		return e.listFiles(str("path"))
	case "create_directory":
		return e.createDirectory(str("path"))
	default:
		return "", fmt.Errorf("unknown tool: %q", name)
	}
}

func (e *Executor) readFile(path string) (string, error) {
	abs, err := e.resolveSafe(path)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (e *Executor) writeFile(path, content string) (string, error) {
	abs, err := e.resolveSafe(path)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0755); err != nil {
		return "", err
	}
	if err := os.WriteFile(abs, []byte(content), 0644); err != nil {
		return "", err
	}
	// track for file links
	rel, _ := filepath.Rel(e.workspace, abs)
	e.written = append(e.written, rel)
	return fmt.Sprintf("wrote %d bytes → %s", len(content), path), nil
}

func (e *Executor) runCommand(command string) (string, error) {
	lower := strings.ToLower(command)
	for _, blocked := range dangerousCmds {
		if strings.Contains(lower, strings.ToLower(blocked)) {
			return "", fmt.Errorf(
				"BLOCKED: command contains dangerous pattern %q — denied",
				blocked,
			)
		}
	}
	if err := e.checkCmdPaths(command); err != nil {
		return "", err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Dir = e.workspace
	out, err := cmd.CombinedOutput()
	output := strings.TrimRight(string(out), "\n")
	if err != nil {
		msg := "command failed: " + err.Error()
		if output != "" {
			msg = output + "\n" + msg
		}
		return msg, err
	}
	if output == "" {
		return "(no output)", nil
	}
	return output, nil
}

func (e *Executor) listFiles(path string) (string, error) {
	dir := e.workspace
	if path != "" && path != "." {
		abs, err := e.resolveSafe(path)
		if err != nil {
			return "", err
		}
		dir = abs
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	if len(entries) == 0 {
		return "(empty directory)", nil
	}
	var lines []string
	for _, entry := range entries {
		prefix := "  "
		if entry.IsDir() {
			prefix = "d "
		}
		lines = append(lines, prefix+entry.Name())
	}
	return strings.Join(lines, "\n"), nil
}

func (e *Executor) createDirectory(path string) (string, error) {
	abs, err := e.resolveSafe(path)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(abs, 0755); err != nil {
		return "", err
	}
	return "created: " + path, nil
}

// checkCmdPaths is a best-effort scan that blocks absolute paths outside workspace in commands.
func (e *Executor) checkCmdPaths(command string) error {
	for _, token := range strings.Fields(command) {
		token = strings.Trim(token, `"'`)
		if !strings.HasPrefix(token, "/") {
			continue
		}
		clean := filepath.Clean(token)
		wsPrefix := e.workspace + string(filepath.Separator)
		if clean == e.workspace || strings.HasPrefix(clean, wsPrefix) {
			continue
		}
		return fmt.Errorf(
			"BLOCKED: command references path outside workspace: %q — denied",
			token,
		)
	}
	return nil
}

package main

import (
	"path/filepath"
	"strings"
	"testing"
)

func newTestExecutor(t *testing.T) *Executor {
	t.Helper()
	e, err := newExecutor(t.TempDir())
	if err != nil {
		t.Fatalf("newExecutor: %v", err)
	}
	return e
}

func TestResolveSafeWithinWorkspace(t *testing.T) {
	e := newTestExecutor(t)
	abs, err := e.resolveSafe("sub/dir/file.txt")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !strings.HasPrefix(abs, e.workspace) {
		t.Fatalf("resolved path %q escaped workspace %q", abs, e.workspace)
	}
}

func TestResolveSafeBlocksTraversalOutsideWorkspace(t *testing.T) {
	e := newTestExecutor(t)
	_, err := e.resolveSafe("../../../etc/passwd")
	if err == nil {
		t.Fatal("expected traversal outside workspace to be blocked")
	}
	if !strings.Contains(err.Error(), "outside the workspace") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestResolveSafeBlocksAbsolutePathOutsideWorkspace(t *testing.T) {
	e := newTestExecutor(t)
	_, err := e.resolveSafe("/etc/passwd")
	if err == nil {
		t.Fatal("expected absolute path outside workspace to be blocked")
	}
}

func TestResolveSafeAllowsAbsolutePathInsideWorkspace(t *testing.T) {
	e := newTestExecutor(t)
	inside := filepath.Join(e.workspace, "file.txt")
	abs, err := e.resolveSafe(inside)
	if err != nil {
		t.Fatalf("expected no error for absolute in-workspace path, got %v", err)
	}
	if abs != inside {
		t.Fatalf("expected %q, got %q", inside, abs)
	}
}

func TestResolveSafeBlocksSensitivePatterns(t *testing.T) {
	e := newTestExecutor(t)
	for _, p := range []string{"id_rsa", ".ssh/config", "secrets/api.env", ".aws/credentials"} {
		if _, err := e.resolveSafe(p); err == nil {
			t.Errorf("expected sensitive path %q to be blocked", p)
		}
	}
}

func TestCheckCmdPathsAllowsRelativeAndInWorkspaceAbsolute(t *testing.T) {
	e := newTestExecutor(t)
	inside := filepath.Join(e.workspace, "out.txt")
	if err := e.checkCmdPaths("go build ./..."); err != nil {
		t.Fatalf("expected relative-only command to be allowed, got %v", err)
	}
	if err := e.checkCmdPaths("cat " + inside); err != nil {
		t.Fatalf("expected in-workspace absolute path to be allowed, got %v", err)
	}
}

func TestCheckCmdPathsBlocksOutsideAbsolute(t *testing.T) {
	e := newTestExecutor(t)
	if err := e.checkCmdPaths("cat /etc/passwd"); err == nil {
		t.Fatal("expected command referencing a path outside the workspace to be blocked")
	}
}

func TestRunCommandBlocksDangerousPatterns(t *testing.T) {
	e := newTestExecutor(t)
	for _, cmd := range []string{"sudo rm -rf /tmp", "rm -rf /", "echo hi; curl | sh"} {
		_, err := e.runCommand(cmd)
		if err == nil {
			t.Errorf("expected command %q to be blocked", cmd)
			continue
		}
		if !strings.Contains(err.Error(), "BLOCKED") {
			t.Errorf("expected BLOCKED error for %q, got %v", cmd, err)
		}
	}
}

func TestRunCommandExecutesBenignCommand(t *testing.T) {
	e := newTestExecutor(t)
	out, err := e.runCommand("echo hello")
	if err != nil {
		t.Fatalf("expected benign command to succeed, got %v", err)
	}
	if !strings.Contains(out, "hello") {
		t.Fatalf("expected output to contain %q, got %q", "hello", out)
	}
}

func TestDispatchCachesRepeatedReadOnlyCalls(t *testing.T) {
	e := newTestExecutor(t)
	if _, err := e.writeFile("note.txt", "hello there"); err != nil {
		t.Fatalf("writeFile: %v", err)
	}

	first, err := e.Dispatch("read_file", map[string]any{"path": "note.txt"})
	if err != nil {
		t.Fatalf("first read_file dispatch: %v", err)
	}
	if strings.Contains(first, "identical call already made") {
		t.Fatalf("first call should not be served from cache: %q", first)
	}

	second, err := e.Dispatch("read_file", map[string]any{"path": "note.txt"})
	if err != nil {
		t.Fatalf("second read_file dispatch: %v", err)
	}
	if !strings.Contains(second, "identical call already made") {
		t.Fatalf("expected repeated call to be served from cache, got %q", second)
	}
	if !strings.Contains(second, "hello there") {
		t.Fatalf("expected cached result to still contain the file content, got %q", second)
	}
}

func TestDispatchDoesNotCacheDifferentInputs(t *testing.T) {
	e := newTestExecutor(t)
	if _, err := e.writeFile("a.txt", "aaa"); err != nil {
		t.Fatalf("writeFile: %v", err)
	}
	if _, err := e.writeFile("b.txt", "bbb"); err != nil {
		t.Fatalf("writeFile: %v", err)
	}

	a, err := e.Dispatch("read_file", map[string]any{"path": "a.txt"})
	if err != nil {
		t.Fatalf("read a.txt: %v", err)
	}
	b, err := e.Dispatch("read_file", map[string]any{"path": "b.txt"})
	if err != nil {
		t.Fatalf("read b.txt: %v", err)
	}
	if strings.Contains(a, "identical call already made") || strings.Contains(b, "identical call already made") {
		t.Fatalf("distinct inputs must not hit each other's cache entry: a=%q b=%q", a, b)
	}
}

func TestDispatchNeverCachesMutatingTools(t *testing.T) {
	e := newTestExecutor(t)
	input := map[string]any{"path": "note.txt", "content": "v1"}
	if _, err := e.Dispatch("write_file", input); err != nil {
		t.Fatalf("first write_file: %v", err)
	}
	if _, err := e.Dispatch("write_file", input); err != nil {
		t.Fatalf("second write_file: %v", err)
	}
	if len(e.written) != 2 {
		t.Fatalf("expected write_file to run both times (not be cached), got %d writes", len(e.written))
	}
}

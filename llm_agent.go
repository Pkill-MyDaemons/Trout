package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

const maxToolRounds = 12

// ── tool schemas ──────────────────────────────────────────────────────────────

// toolSchema is the JSON Schema for a single tool parameter object.
// Both Claude (input_schema) and OpenAI (parameters) use the same schema structure.
var toolSchemas = []struct {
	Name        string
	Description string
	Properties  map[string]any
	Required    []string
}{
	{
		Name:        "read_file",
		Description: "Read a file from the workspace.",
		Properties: map[string]any{
			"path": map[string]any{"type": "string", "description": "Path relative to workspace root."},
		},
		Required: []string{"path"},
	},
	{
		Name:        "write_file",
		Description: "Write content to a file in the workspace. Creates parent directories as needed.",
		Properties: map[string]any{
			"path":    map[string]any{"type": "string", "description": "Path relative to workspace root. Use projects/<name>/<file> for code."},
			"content": map[string]any{"type": "string", "description": "Full file content to write."},
		},
		Required: []string{"path", "content"},
	},
	{
		Name:        "run_command",
		Description: "Run a shell command inside the workspace directory. Timeout 30 s.",
		Properties: map[string]any{
			"command": map[string]any{"type": "string", "description": "Shell command to execute."},
		},
		Required: []string{"command"},
	},
	{
		Name:        "list_files",
		Description: "List files and directories at a path inside the workspace.",
		Properties: map[string]any{
			"path": map[string]any{"type": "string", "description": "Relative path, empty for workspace root."},
		},
		Required: []string{},
	},
	{
		Name:        "create_directory",
		Description: "Create a directory (and parents) in the workspace.",
		Properties: map[string]any{
			"path": map[string]any{"type": "string", "description": "Relative path to create."},
		},
		Required: []string{"path"},
	},
}

// ── system prompt ─────────────────────────────────────────────────────────────

func buildAgentSystemPrompt(cfg *Config, task *Task, workspace string) string {
	var b strings.Builder
	b.WriteString("You are a task agent helping a software developer.\n\n")
	fmt.Fprintf(&b, "You have a sandboxed workspace at: %s\n", workspace)
	b.WriteString("You may freely read/write files and run commands there.\n\n")
	b.WriteString("**Rules:**\n")
	b.WriteString("- When writing code, first create a project folder: create_directory(\"projects/<project-name>\")\n")
	b.WriteString("- Keep all project files inside that folder.\n")
	b.WriteString("- Paths outside the workspace are blocked; if blocked, adjust and continue.\n\n")
	b.WriteString("**Current task:**\n")
	fmt.Fprintf(&b, "Title: %s\n", task.Title)
	if task.Description != "" {
		fmt.Fprintf(&b, "Description: %s\n", task.Description)
	}
	fmt.Fprintf(&b, "Status: %s\n\n", statusLabel(task.Status))
	b.WriteString("Use tools to complete the task, then write a clear summary.\n")
	b.WriteString("At the end of your final response, list any files you wrote:\n\n")
	b.WriteString("<!-- task-agent-files\n")
	b.WriteString("projects/my-project/main.go\n")
	b.WriteString("-->\n")

	if cfg.EmailMode != EmailModeSummaryOnly {
		b.WriteString("\n**Email replies:**\n")
		b.WriteString("If this task involves an email that needs a reply, append a draft using:\n\n")
		b.WriteString("<!-- task-agent-email\n")
		b.WriteString("to: sender@example.com\n")
		b.WriteString("subject: Re: Original Subject\n")
		b.WriteString("---\n")
		b.WriteString("Reply body here.\n")
		b.WriteString("-->\n\n")
		if cfg.EmailMode == EmailModeAutoSend {
			b.WriteString("The draft will be sent automatically — write it ready to send.\n")
		} else {
			b.WriteString("The user will review the draft before it is sent.\n")
		}
	}
	return b.String()
}

// ── main dispatcher ───────────────────────────────────────────────────────────

// callLLMLoop runs the full agentic loop: tool calls → execute → results → repeat.
// Returns the final text response and any files the agent wrote.
func callLLMLoop(cfg *Config, task *Task, exec *Executor) (string, []string, error) {
	switch cfg.Provider {
	case "claude":
		return claudeAgentLoop(cfg, task, exec)
	default:
		return openaiAgentLoop(cfg, task, exec)
	}
}

// ── Claude agent loop ─────────────────────────────────────────────────────────

// Claude content block types
type claudeTextBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}
type claudeToolUseBlock struct {
	Type  string          `json:"type"`
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}
type claudeToolResultBlock struct {
	Type      string `json:"type"`
	ToolUseID string `json:"tool_use_id"`
	Content   string `json:"content"`
	IsError   bool   `json:"is_error,omitempty"`
}

type claudeMsg struct {
	Role    string `json:"role"`
	Content any    `json:"content"` // string | []any
}

func claudeAgentLoop(cfg *Config, task *Task, exec *Executor) (string, []string, error) {
	system := buildAgentSystemPrompt(cfg, task, exec.workspace)

	// seed conversation from prior comments
	var messages []claudeMsg
	for _, m := range buildMessages(task) {
		messages = append(messages, claudeMsg{Role: m.Role, Content: m.Content})
	}

	// build tool definitions for Claude
	var tools []any
	for _, s := range toolSchemas {
		tools = append(tools, map[string]any{
			"name":         s.Name,
			"description":  s.Description,
			"input_schema": map[string]any{"type": "object", "properties": s.Properties, "required": s.Required},
		})
	}

	for round := 0; round < maxToolRounds; round++ {
		reqBody := map[string]any{
			"model":      cfg.Model,
			"max_tokens": 4096,
			"system":     system,
			"tools":      tools,
			"messages":   messages,
		}
		body, _ := json.Marshal(reqBody)

		raw, err := httpPost("https://api.anthropic.com/v1/messages", body, map[string]string{
			"x-api-key":         cfg.APIKey,
			"anthropic-version": "2023-06-01",
			"content-type":      "application/json",
		})
		if err != nil {
			return "", nil, err
		}

		var resp struct {
			StopReason string            `json:"stop_reason"`
			Content    []json.RawMessage `json:"content"`
			Error      *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal(raw, &resp); err != nil {
			return "", nil, fmt.Errorf("claude parse: %w\nraw: %s", err, raw)
		}
		if resp.Error != nil {
			return "", nil, fmt.Errorf("claude: %s", resp.Error.Message)
		}

		// Decode content blocks
		var textParts []string
		var toolCalls []claudeToolUseBlock
		var assistantBlocks []any

		for _, block := range resp.Content {
			var base struct {
				Type string `json:"type"`
			}
			json.Unmarshal(block, &base)
			switch base.Type {
			case "text":
				var tb claudeTextBlock
				json.Unmarshal(block, &tb)
				textParts = append(textParts, tb.Text)
				assistantBlocks = append(assistantBlocks, tb)
			case "tool_use":
				var tu claudeToolUseBlock
				json.Unmarshal(block, &tu)
				toolCalls = append(toolCalls, tu)
				assistantBlocks = append(assistantBlocks, tu)
			}
		}

		if resp.StopReason == "end_turn" || len(toolCalls) == 0 {
			text := strings.TrimSpace(strings.Join(textParts, "\n"))
			body, files := parseAgentResponse(text)
			// Also include files the executor tracked (written via write_file)
			files = mergeFiles(files, exec.written)
			return body, files, nil
		}

		// Append assistant turn with tool use blocks
		messages = append(messages, claudeMsg{Role: "assistant", Content: assistantBlocks})

		// Execute each tool call and collect results
		var resultBlocks []any
		for _, tc := range toolCalls {
			var inputMap map[string]any
			json.Unmarshal(tc.Input, &inputMap)

			result, err := exec.Dispatch(tc.Name, inputMap)
			isErr := err != nil
			content := result
			if isErr {
				content = err.Error()
			}
			resultBlocks = append(resultBlocks, claudeToolResultBlock{
				Type:      "tool_result",
				ToolUseID: tc.ID,
				Content:   content,
				IsError:   isErr,
			})
		}
		messages = append(messages, claudeMsg{Role: "user", Content: resultBlocks})
	}

	return "Agent reached maximum tool-use rounds without finishing.", nil, nil
}

// ── OpenAI-compatible agent loop ─────────────────────────────────────────────

type openAIMsg struct {
	Role       string         `json:"role"`
	Content    any            `json:"content"` // string or null
	ToolCalls  []oaiToolCall  `json:"tool_calls,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
	Name       string         `json:"name,omitempty"`
}

type oaiToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

func openaiAgentLoop(cfg *Config, task *Task, exec *Executor) (string, []string, error) {
	baseURL := openaiBaseURL(cfg)
	system := buildAgentSystemPrompt(cfg, task, exec.workspace)

	// seed with system + conversation history
	messages := []openAIMsg{{Role: "system", Content: system}}
	for _, m := range buildMessages(task) {
		messages = append(messages, openAIMsg{Role: m.Role, Content: m.Content})
	}

	// build OpenAI-format tools
	var tools []any
	for _, s := range toolSchemas {
		tools = append(tools, map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        s.Name,
				"description": s.Description,
				"parameters":  map[string]any{"type": "object", "properties": s.Properties, "required": s.Required},
			},
		})
	}

	for round := 0; round < maxToolRounds; round++ {
		reqBody := map[string]any{
			"model":    cfg.Model,
			"tools":    tools,
			"messages": messages,
		}
		body, _ := json.Marshal(reqBody)

		raw, err := httpPost(baseURL+"/chat/completions", body, map[string]string{
			"Authorization": "Bearer " + cfg.APIKey,
			"content-type":  "application/json",
		})
		if err != nil {
			return "", nil, err
		}

		var resp struct {
			Choices []struct {
				Message      openAIMsg `json:"message"`
				FinishReason string    `json:"finish_reason"`
			} `json:"choices"`
			Error *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal(raw, &resp); err != nil {
			return "", nil, fmt.Errorf("openai parse: %w\nraw: %s", err, raw)
		}
		if resp.Error != nil {
			return "", nil, fmt.Errorf("%s: %s", cfg.Provider, resp.Error.Message)
		}
		if len(resp.Choices) == 0 {
			return "", nil, fmt.Errorf("%s: empty response", cfg.Provider)
		}

		choice := resp.Choices[0]
		messages = append(messages, choice.Message)

		if choice.FinishReason != "tool_calls" || len(choice.Message.ToolCalls) == 0 {
			text := ""
			if s, ok := choice.Message.Content.(string); ok {
				text = strings.TrimSpace(s)
			}
			body, files := parseAgentResponse(text)
			files = mergeFiles(files, exec.written)
			return body, files, nil
		}

		// Execute each tool call
		for _, tc := range choice.Message.ToolCalls {
			var inputMap map[string]any
			json.Unmarshal([]byte(tc.Function.Arguments), &inputMap)

			result, err := exec.Dispatch(tc.Function.Name, inputMap)
			content := result
			if err != nil {
				content = err.Error()
			}
			messages = append(messages, openAIMsg{
				Role:       "tool",
				ToolCallID: tc.ID,
				Name:       tc.Function.Name,
				Content:    content,
			})
		}
	}

	return "Agent reached maximum tool-use rounds without finishing.", nil, nil
}

func openaiBaseURL(cfg *Config) string {
	switch cfg.Provider {
	case "gemini":
		return "https://generativelanguage.googleapis.com/v1beta/openai"
	case "groq":
		return "https://api.groq.com/openai/v1"
	default: // local
		base := strings.TrimRight(cfg.LocalURL, "/")
		if !strings.HasSuffix(base, "/v1") {
			base += "/v1"
		}
		return base
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

// httpPost sends a JSON POST with the given headers, using the shared httpDo from llm.go.
func httpPost(url string, body []byte, headers map[string]string) ([]byte, error) {
	req, err := newHTTPReq("POST", url, body)
	if err != nil {
		return nil, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return httpDo(req)
}

// mergeFiles deduplicates two file slices, keeping order.
func mergeFiles(a, b []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, f := range append(a, b...) {
		if f != "" && !seen[f] {
			seen[f] = true
			out = append(out, f)
		}
	}
	return out
}

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type llmMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// callLLM routes to the correct provider implementation.
func callLLM(cfg *Config, task *Task) (string, error) {
	if cfg.APIKey == "" && cfg.Provider != "local" {
		return "", fmt.Errorf("no API key set for provider %q — open config with 'c'", cfg.Provider)
	}
	system := buildSystemPrompt(task)
	messages := buildMessages(task)

	switch cfg.Provider {
	case "claude":
		return callClaude(cfg, system, messages)
	case "gemini":
		return callOpenAICompat(cfg, system, messages, "https://generativelanguage.googleapis.com/v1beta/openai")
	case "groq":
		return callOpenAICompat(cfg, system, messages, "https://api.groq.com/openai/v1")
	case "local":
		base := strings.TrimRight(cfg.LocalURL, "/")
		if !strings.HasSuffix(base, "/v1") {
			base += "/v1"
		}
		return callOpenAICompat(cfg, system, messages, base)
	default:
		return "", fmt.Errorf("unknown provider: %s", cfg.Provider)
	}
}

func buildSystemPrompt(task *Task) string {
	var b strings.Builder
	b.WriteString("You are a task agent helping a software developer. ")
	b.WriteString("Respond to the task clearly and actionably.\n\n")
	b.WriteString("Task title: " + task.Title + "\n")
	if task.Description != "" {
		b.WriteString("Description: " + task.Description + "\n")
	}
	b.WriteString("Status: " + statusLabel(task.Status) + "\n\n")
	b.WriteString("Be concise. Use markdown. Include code blocks when relevant.\n\n")
	b.WriteString("If you create or modify any files, list their paths at the very end of your\n")
	b.WriteString("response using this exact block (outside any code fence, one path per line):\n\n")
	b.WriteString("<!-- task-agent-files\n")
	b.WriteString("path/to/file1.go\n")
	b.WriteString("path/to/file2.go\n")
	b.WriteString("-->")
	return b.String()
}

// parseAgentResponse splits the LLM reply into the visible body and any file paths
// listed in the <!-- task-agent-files ... --> trailer block.
func parseAgentResponse(raw string) (body string, files []string) {
	const start = "<!-- task-agent-files"
	const end = "-->"
	idx := strings.LastIndex(raw, start)
	if idx == -1 {
		return strings.TrimSpace(raw), nil
	}
	body = strings.TrimSpace(raw[:idx])
	rest := raw[idx+len(start):]
	closeIdx := strings.Index(rest, end)
	if closeIdx == -1 {
		return body, nil
	}
	section := strings.TrimSpace(rest[:closeIdx])
	for _, line := range strings.Split(section, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			files = append(files, line)
		}
	}
	return
}

// buildMessages converts the comment thread into a message slice for the LLM.
// Consecutive same-role messages are merged. Must end with a user-role message.
func buildMessages(task *Task) []llmMessage {
	if len(task.Comments) == 0 {
		return []llmMessage{{
			Role:    "user",
			Content: "Please analyse this task and give your initial response.",
		}}
	}

	var msgs []llmMessage
	for _, c := range task.Comments {
		role := "assistant"
		if c.Author == "user" {
			role = "user"
		}
		if len(msgs) > 0 && msgs[len(msgs)-1].Role == role {
			msgs[len(msgs)-1].Content += "\n\n" + c.Body
		} else {
			msgs = append(msgs, llmMessage{Role: role, Content: c.Body})
		}
	}

	if msgs[len(msgs)-1].Role != "user" {
		msgs = append(msgs, llmMessage{Role: "user", Content: "Please continue."})
	}
	return msgs
}

// ── Claude (Anthropic Messages API) ──────────────────────────────────────────

func callClaude(cfg *Config, system string, messages []llmMessage) (string, error) {
	type msg struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	type reqBody struct {
		Model     string `json:"model"`
		MaxTokens int    `json:"max_tokens"`
		System    string `json:"system"`
		Messages  []msg  `json:"messages"`
	}
	type contentBlock struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	type respBody struct {
		Content []contentBlock `json:"content"`
		Error   *struct {
			Message string `json:"message"`
		} `json:"error"`
	}

	msgs := make([]msg, len(messages))
	for i, m := range messages {
		msgs[i] = msg{Role: m.Role, Content: m.Content}
	}

	body, _ := json.Marshal(reqBody{
		Model:     cfg.Model,
		MaxTokens: 4096,
		System:    system,
		Messages:  msgs,
	})

	req, err := http.NewRequest("POST", "https://api.anthropic.com/v1/messages", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("x-api-key", cfg.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("content-type", "application/json")

	resp, err := httpDo(req)
	if err != nil {
		return "", err
	}
	var r respBody
	if err := json.Unmarshal(resp, &r); err != nil {
		return "", fmt.Errorf("claude parse: %w\nraw: %s", err, resp)
	}
	if r.Error != nil {
		return "", fmt.Errorf("claude: %s", r.Error.Message)
	}
	if len(r.Content) == 0 {
		return "", fmt.Errorf("claude: empty response")
	}
	return r.Content[0].Text, nil
}

// ── OpenAI-compatible (Gemini, Groq, Ollama) ─────────────────────────────────

func callOpenAICompat(cfg *Config, system string, messages []llmMessage, baseURL string) (string, error) {
	type reqBody struct {
		Model    string       `json:"model"`
		Messages []llmMessage `json:"messages"`
	}
	type respBody struct {
		Choices []struct {
			Message llmMessage `json:"message"`
		} `json:"choices"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}

	allMsgs := append([]llmMessage{{Role: "system", Content: system}}, messages...)
	body, _ := json.Marshal(reqBody{Model: cfg.Model, Messages: allMsgs})

	req, err := http.NewRequest("POST", baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	req.Header.Set("content-type", "application/json")

	raw, err := httpDo(req)
	if err != nil {
		return "", err
	}
	var r respBody
	if err := json.Unmarshal(raw, &r); err != nil {
		return "", fmt.Errorf("parse: %w\nraw: %s", err, raw)
	}
	if r.Error != nil {
		return "", fmt.Errorf("%s: %s", cfg.Provider, r.Error.Message)
	}
	if len(r.Choices) == 0 {
		return "", fmt.Errorf("%s: empty response", cfg.Provider)
	}
	return r.Choices[0].Message.Content, nil
}

// ── shared HTTP helpers ───────────────────────────────────────────────────────

func newHTTPReq(method, url string, body []byte) (*http.Request, error) {
	return http.NewRequest(method, url, bytes.NewReader(body))
}

func httpDo(req *http.Request) ([]byte, error) {
	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		// try to surface the error message from the body
		var e struct {
			Error struct{ Message string } `json:"error"`
		}
		if json.Unmarshal(data, &e) == nil && e.Error.Message != "" {
			return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, e.Error.Message)
		}
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, data)
	}
	return data, nil
}

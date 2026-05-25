# task-agent

A terminal task manager with an AI agent backend. Create tasks, chat with an AI agent that can write code and run commands, and manage email replies — all from your terminal.

![Go](https://img.shields.io/badge/Go-1.21+-00ADD8?logo=go)

## Features

- **Task list** — add, delete, cycle status (todo → in progress → done)
- **Chat threads** — each task has a comment thread; reply to the agent and it responds
- **Unread dot** — `●` indicator on tasks with new agent responses
- **AI agent** — reads your task, uses tools (read/write files, run commands), posts a summary
- **File viewer** — agent-written files are linked in the thread; browse and view them in-TUI
- **Background daemon** — two modes: nightly at 23:00, or responsive (every 5 min)
- **Email integration** — three modes: summary only / draft for approval / auto-send
- **No-reply filter** — automatically skips emails from automated senders
- **Multi-provider** — Claude, Gemini, Groq, or any local model (Ollama, LM Studio, etc.)

## Installation

```bash
git clone https://github.com/Pkill-MyDaemons/task-agent
cd task-agent
go build -o task-agent .
./task-agent
```

Requires Go 1.21+.

## Usage

### TUI keybindings

**Task list:**

| Key | Action |
|-----|--------|
| `n` | New task |
| `enter` | Open task thread |
| `s` | Cycle task status |
| `d` | Delete task |
| `c` | Open config |
| `q` | Quit |

**Task thread:**

| Key | Action |
|-----|--------|
| `r` | Reply / add comment |
| `j` / `k` | Scroll |
| `f` | Browse files the agent wrote |
| `a` | Approve & send pending email draft |
| `x` | Reject pending email draft |
| `esc` | Back to task list |

**File viewer:**

| Key | Action |
|-----|--------|
| `enter` | Open selected file |
| `j` / `k` | Navigate / scroll |
| `esc` | Back to file list |
| `b` | Back to thread |

### Config (`c` from task list)

- **Daemon mode** — `nightly` (default, runs at 23:00) or `responsive` (every 5 min on new replies)
- **Run at** — time for nightly mode, format `HH:MM`
- **Provider** — `claude`, `gemini`, `groq`, `local`
- **Model** — model name for the selected provider
- **API key** — provider API key
- **Work dir** — sandbox directory the agent works in (default `~/task-agent-workspace/`)
- **Local URL** — base URL for local models (default `http://localhost:11434`)
- **Email** — SMTP settings and email mode

### Daemon

Start/stop the background daemon from the config screen, or manually:

```bash
./task-agent --daemon   # run daemon in foreground (for testing)
```

The daemon re-execs the binary in the background and saves its PID to `~/.task-agent/daemon.pid`.

## AI Agent

The agent runs an agentic loop (up to 12 tool-call rounds) with these tools:

| Tool | Description |
|------|-------------|
| `read_file` | Read a file from the workspace |
| `write_file` | Write a file (creates parent dirs) |
| `run_command` | Run a shell command (30s timeout) |
| `list_files` | List files at a path |
| `create_directory` | Create a directory |

**Workspace sandbox:** All file operations are confined to the work dir. Sensitive paths (`.ssh`, `.aws`, credentials, etc.) and dangerous commands (`sudo`, `rm -rf /`, etc.) are blocked — the agent is told it was blocked and adjusts.

**Project convention:** The agent creates `projects/<name>/` inside the workspace for code.

## Email integration

Set **Email mode** in config:

- **Summary only** — agent posts a summary comment, no draft generated
- **Draft + wait** — agent drafts a reply; use `a` to approve/send or `x` to reject
- **Auto send** — agent drafts and sends immediately via SMTP

Configure SMTP host, port, from address, and credentials in the Email section of config. Port 465 uses implicit TLS (SMTPS); all other ports use STARTTLS.

## Data storage

All data lives in `~/.task-agent/`:

| File | Contents |
|------|----------|
| `tasks.json` | All tasks and comment threads |
| `config.json` | Settings (API keys, SMTP, etc.) — mode `0600` |
| `daemon.pid` | PID of the running daemon |

## Providers

| Provider | Default model | Notes |
|----------|--------------|-------|
| `claude` | `claude-opus-4-7` | Anthropic API |
| `gemini` | `gemini-2.5-pro` | Google via OpenAI-compat endpoint |
| `groq` | `llama-3.3-70b-versatile` | Groq Cloud |
| `local` | `llama3` | Ollama / LM Studio at configured URL |

## Dependencies

- [Bubble Tea](https://github.com/charmbracelet/bubbletea) — TUI framework
- [Lip Gloss](https://github.com/charmbracelet/lipgloss) — terminal styling
- [Bubbles](https://github.com/charmbracelet/bubbles) — textinput, textarea, viewport
- [Glamour](https://github.com/charmbracelet/glamour) — markdown rendering in terminal

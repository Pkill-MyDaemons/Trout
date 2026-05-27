# task-agent

A terminal task manager with an AI agent backend. Create tasks, chat with an AI agent that can write code, run commands, search the web, and manage your calendar — all from your terminal.

![Go](https://img.shields.io/badge/Go-1.21+-00ADD8?logo=go)

## Features

- **Task list** — add, delete, cycle status (todo → in progress → done)
- **Chat threads** — each task has a comment thread; reply to the agent and it responds
- **Unread dot** — `●` indicator on tasks with new agent responses
- **AI agent** — reads your task, uses tools, posts a summary
- **File viewer** — agent-written files are linked in the thread; browse and view them in-TUI
- **Background daemon** — two modes: nightly at 23:00, or responsive (every 5 min)
- **Gmail polling** — inbox is polled for new emails; each one becomes a task automatically
- **Email replies** — three modes: summary only / draft for approval / auto-send
- **Google Calendar** — agent can list and create calendar events
- **Web search** — agent searches Google and reads full page content (via [local-search](https://github.com/Pkill-MyDaemons/local-search))
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
| `R` | Reload tasks from disk |
| `c` | Open config |
| `q` | Quit |

**Task thread:**

| Key | Action |
|-----|--------|
| `r` | Reply / add comment |
| `j` / `k` | Scroll |
| `f` | Browse files the agent wrote |
| `y` | Approve & send pending email draft |
| `n` | Edit pending email draft before sending |
| `esc` | Back to task list |

**File viewer:**

| Key | Action |
|-----|--------|
| `enter` | Open selected file |
| `j` / `k` | Navigate / scroll |
| `esc` | Back to file list |
| `b` | Back to thread |

### Config (`c` from task list)

- **Daemon mode** — `nightly` (runs at set time) or `responsive` (every 5 min on new replies)
- **Run at** — time for nightly mode, format `HH:MM`
- **Provider** — `claude`, `gemini`, `groq`, `local`
- **Model** — model name for the selected provider
- **API key** — provider API key
- **Work dir** — sandbox directory the agent works in (default `~/.task-agent/projects/`)
- **Local URL** — base URL for local models (default `http://localhost:11434`)
- **Email** — mode, provider (SMTP or Gmail OAuth), and credentials

### Daemon

Start/stop the background daemon from the config screen, or manually:

```bash
./task-agent --daemon   # run daemon in foreground (for testing)
```

The daemon re-execs the binary in the background and saves its PID to `~/.task-agent/daemon.pid`. Logs are written to `~/.task-agent/daemon.log`.

## AI Agent

The agent runs an agentic loop (up to 12 tool-call rounds) with these tools:

| Tool | Description |
|------|-------------|
| `read_file` | Read a file from the workspace |
| `write_file` | Write a file (creates parent dirs) |
| `run_command` | Run a shell command (30s timeout) |
| `list_files` | List files at a path |
| `create_directory` | Create a directory |
| `web_search` | Search Google and return titles, URLs, and snippets |
| `fetch_page` | Fetch any URL and return its text content |
| `list_calendar_events` | List upcoming Google Calendar events |
| `create_calendar_event` | Create a new Google Calendar event |

`web_search` and `fetch_page` require the [local-search](https://github.com/Pkill-MyDaemons/local-search) server to be running on `localhost:8000`.

**Workspace sandbox:** All file operations are confined to the work dir. Sensitive paths (`.ssh`, `.aws`, credentials, etc.) and dangerous commands (`sudo`, `rm -rf /`, fork bombs, etc.) are blocked.

**Project convention:** The agent creates `projects/<name>/` inside the workspace for code.

## Email integration

Set **Email mode** in config:

- **Summary only** — agent posts a summary comment, no draft generated
- **Draft + wait** — agent drafts a reply; press `y` to send or `n` to edit before sending
- **Auto send** — agent drafts and sends immediately

### Gmail OAuth

Set **Email provider** to `gmail`, enter your OAuth client ID and secret, then authenticate from the config screen. The agent will poll your primary inbox and create tasks from incoming emails, skipping no-reply senders, bulk mail, and anything with a `List-Unsubscribe` header.

### SMTP

Configure host, port, from address, and credentials. Port 465 uses implicit TLS; all other ports use STARTTLS.

## Google Calendar

Requires Gmail OAuth to be set up (the Calendar scope is requested at the same time). Once authenticated, the agent can read your upcoming events and create new ones.

## Data storage

All data lives in `~/.task-agent/`:

| Path | Contents |
|------|----------|
| `tasks.json` | All tasks and comment threads |
| `config.json` | Settings (API keys, SMTP, etc.) — mode `0600` |
| `daemon.pid` | PID of the running daemon |
| `daemon.log` | Daemon activity log |
| `gmail_seen.json` | Gmail message IDs already processed |
| `projects/` | Default agent workspace |

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

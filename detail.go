package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
)

type sv int

const (
	svThread    sv = iota
	svReply
	svFilePick
	svFile
	svEditDraft
)

type detailModel struct {
	store      *Store
	task       *Task
	vp         viewport.Model
	reply      textarea.Model
	view       sv
	fileCursor int
	viewFile   string
	statusMsg  string
	width      int
	height     int
}

func newDetailModel(store *Store, task *Task, w, h int) detailModel {
	vp := viewport.New(w-4, h-10)
	vp.SetContent(renderThread(task, w-4))

	ta := textarea.New()
	ta.Placeholder = "Write a comment..."
	ta.CharLimit = 2000
	ta.SetWidth(w - 4)
	ta.SetHeight(4)
	ta.ShowLineNumbers = false

	return detailModel{
		store:  store,
		task:   task,
		vp:     vp,
		reply:  ta,
		width:  w,
		height: h,
	}
}

func (m detailModel) Init() tea.Cmd { return nil }

func (m detailModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.vp.Width = msg.Width - 4
		m.vp.Height = msg.Height - 10
		m.reply.SetWidth(msg.Width - 4)
		m = m.refreshViewport()

	case tea.KeyMsg:
		switch m.view {
		case svThread:
			return m.keyThread(msg)
		case svReply:
			return m.keyReply(msg)
		case svFilePick:
			return m.keyFilePick(msg)
		case svFile:
			return m.keyFile(msg)
		case svEditDraft:
			return m.keyEditDraft(msg)
		}
	}

	if m.view == svThread || m.view == svFile {
		var cmd tea.Cmd
		m.vp, cmd = m.vp.Update(msg)
		return m, cmd
	}
	if m.view == svReply || m.view == svEditDraft {
		var cmd tea.Cmd
		m.reply, cmd = m.reply.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m detailModel) keyThread(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "esc":
		lm := newListModel(m.store)
		lm.width, lm.height = m.width, m.height
		return lm, nil
	case "ctrl+c":
		return m, tea.Quit
	case "r":
		m.view = svReply
		m.reply.SetValue("")
		m.reply.Focus()
		return m, textarea.Blink
	case "f":
		if len(m.allFiles()) > 0 {
			m.view = svFilePick
			m.fileCursor = 0
		}
	case "y":
		if draft := m.store.pendingDraft(m.task.ID); draft != nil {
			cfg, _ := loadConfig()
			if err := sendEmail(cfg, draft); err != nil {
				m.statusMsg = "send failed: " + err.Error()
			} else {
				m.store.updateEmailDraft(m.task.ID, "sent")
				m.statusMsg = "Email sent to " + draft.To
				m = m.refreshViewport()
			}
		}
	case "n":
		if draft := m.store.pendingDraft(m.task.ID); draft != nil {
			m.reply.SetValue(draft.Body)
			m.reply.Focus()
			m.view = svEditDraft
			return m, textarea.Blink
		}
	case "up", "k":
		m.vp.LineUp(3)
	case "down", "j":
		m.vp.LineDown(3)
	}
	var cmd tea.Cmd
	m.vp, cmd = m.vp.Update(msg)
	return m, cmd
}

func (m detailModel) keyReply(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.view = svThread
		return m, nil
	case "ctrl+c":
		return m, tea.Quit
	case "ctrl+s":
		body := strings.TrimSpace(m.reply.Value())
		if body != "" {
			m.store.addComment(m.task.ID, "user", body, nil, nil)
			m = m.refreshViewport()
			m.vp.GotoBottom()
		}
		m.view = svThread
		return m, nil
	}
	var cmd tea.Cmd
	m.reply, cmd = m.reply.Update(msg)
	return m, cmd
}

func (m detailModel) keyFilePick(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	files := m.allFiles()
	switch msg.String() {
	case "esc", "q":
		m.view = svThread
	case "ctrl+c":
		return m, tea.Quit
	case "up", "k":
		if m.fileCursor > 0 {
			m.fileCursor--
		}
	case "down", "j":
		if m.fileCursor < len(files)-1 {
			m.fileCursor++
		}
	case "enter":
		if len(files) > 0 {
			m.viewFile = files[m.fileCursor]
			m.vp.SetContent(m.renderFile(m.viewFile))
			m.vp.GotoTop()
			m.view = svFile
		}
	}
	return m, nil
}

func (m detailModel) keyFile(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		m.view = svFilePick
		return m, nil
	case "ctrl+c":
		return m, tea.Quit
	case "b":
		m.view = svThread
		m = m.refreshViewport()
		return m, nil
	}
	var cmd tea.Cmd
	m.vp, cmd = m.vp.Update(msg)
	return m, cmd
}

func (m detailModel) keyEditDraft(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.view = svThread
		m.reply.Blur()
		return m, nil
	case "ctrl+c":
		return m, tea.Quit
	case "ctrl+s":
		draft := m.store.pendingDraft(m.task.ID)
		if draft != nil {
			draft.Body = m.reply.Value()
			cfg, _ := loadConfig()
			if err := sendEmail(cfg, draft); err != nil {
				m.statusMsg = "send failed: " + err.Error()
			} else {
				m.store.updateEmailDraft(m.task.ID, "sent")
				m.statusMsg = "Email sent to " + draft.To
				m = m.refreshViewport()
			}
		}
		m.view = svThread
		m.reply.Blur()
		return m, nil
	}
	var cmd tea.Cmd
	m.reply, cmd = m.reply.Update(msg)
	return m, cmd
}

func (m detailModel) refreshViewport() detailModel {
	m.vp.SetContent(renderThread(m.task, m.width-4))
	return m
}

func (m detailModel) allFiles() []string {
	var files []string
	seen := map[string]bool{}
	for _, c := range m.task.Comments {
		for _, f := range c.Files {
			if !seen[f] {
				files = append(files, f)
				seen[f] = true
			}
		}
	}
	return files
}

func (m detailModel) renderFile(path string) string {
	cfg, _ := loadConfig()
	workDir := "."
	if cfg != nil {
		workDir = cfg.WorkDir
	}

	content, err := readFile(path, workDir)
	if err != nil {
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#EF4444")).
			Render(fmt.Sprintf("Cannot read file: %s\n%s", path, err))
	}

	ext := strings.TrimPrefix(filepath.Ext(path), ".")
	if ext == "" {
		ext = "text"
	}
	md := fmt.Sprintf("**`%s`**\n\n```%s\n%s\n```", path, ext, content)
	r, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle("dark"),
		glamour.WithWordWrap(m.width-6),
	)
	if err != nil {
		return content
	}
	rendered, err := r.Render(md)
	if err != nil {
		return content
	}
	return rendered
}

func (m detailModel) View() string {
	switch m.view {
	case svFilePick:
		return m.viewFilePicker()
	case svFile:
		return m.viewFileContent()
	case svReply:
		return m.viewWithReply()
	case svEditDraft:
		return m.viewEditDraft()
	default:
		return m.viewThread()
	}
}

func (m detailModel) viewThread() string {
	files := m.allFiles()
	fileHint := ""
	if len(files) > 0 {
		fileHint = "  •  f files(" + fmt.Sprintf("%d", len(files)) + ")"
	}
	emailHint := ""
	if m.store.pendingDraft(m.task.ID) != nil {
		emailHint = "  •  " + lipgloss.NewStyle().Foreground(colorUnread).Bold(true).Render("y send  n edit")
	}
	help := styleHelp.Render("r reply  •  j/k scroll"+fileHint) + emailHint + styleHelp.Render("  •  esc back")

	var extra string
	if m.statusMsg != "" {
		extra = "\n" + lipgloss.NewStyle().Foreground(colorDone).Render(m.statusMsg)
	}
	return lipgloss.NewStyle().Padding(1, 2).Render(
		m.taskHeader() + "\n" + m.divider() + "\n" + m.vp.View() + "\n" + help + extra,
	)
}

func (m detailModel) viewWithReply() string {
	help := styleHelp.Render("ctrl+s send  •  esc cancel")
	return lipgloss.NewStyle().Padding(1, 2).Render(
		m.taskHeader() + "\n" + m.divider() + "\n" + m.vp.View() +
			"\n" + m.reply.View() + "\n" + help,
	)
}

func (m detailModel) viewEditDraft() string {
	draft := m.store.pendingDraft(m.task.ID)
	header := ""
	if draft != nil {
		header = styleMuted.Render("To: ") + draft.To +
			"  " + styleMuted.Render("Subject: ") + draft.Subject + "\n"
	}
	help := styleHelp.Render("ctrl+s send  •  esc cancel")
	return lipgloss.NewStyle().Padding(1, 2).Render(
		m.taskHeader() + "\n" + m.divider() + "\n" +
			header + m.reply.View() + "\n" + help,
	)
}

func (m detailModel) viewFilePicker() string {
	files := m.allFiles()
	var rows []string
	for i, f := range files {
		line := "  " + styleMuted.Render("↗") + " " + f
		if i == m.fileCursor {
			line = lipgloss.NewStyle().
				Background(colorSelected).Foreground(colorText).
				Padding(0, 1).
				Render("↗ " + f)
		}
		rows = append(rows, line)
	}
	help := styleHelp.Render("enter open  •  j/k navigate  •  esc back")
	content := styleTitle.Render("Files") + "\n\n" +
		strings.Join(rows, "\n") + "\n\n" + help
	return lipgloss.NewStyle().Padding(1, 2).Render(content)
}

func (m detailModel) viewFileContent() string {
	header := styleTitle.Render(m.viewFile)
	help := styleHelp.Render("j/k scroll  •  esc file list  •  b thread")
	return lipgloss.NewStyle().Padding(1, 2).Render(
		header + "\n" + m.divider() + "\n" + m.vp.View() + "\n" + help,
	)
}

func (m detailModel) taskHeader() string {
	h := styleTitle.Render(m.task.Title) + "\n" +
		statusStyle(m.task.Status).Render(statusLabel(m.task.Status)) +
		styleMuted.Render("  •  "+m.task.CreatedAt.Format("Jan 2, 2006"))
	if m.task.Description != "" {
		h += "\n" + lipgloss.NewStyle().Foreground(colorSubtext).Render(m.task.Description)
	}
	return h
}

func (m detailModel) divider() string {
	n := m.width - 4
	if n < 0 {
		n = 0
	}
	return styleMuted.Render(strings.Repeat("─", n))
}

// ── thread rendering ──────────────────────────────────────────────────────────

func renderThread(task *Task, width int) string {
	if len(task.Comments) == 0 {
		return styleMuted.Render("No comments yet. Press r to reply, or start the daemon to process this task.")
	}
	var parts []string
	for _, c := range task.Comments {
		parts = append(parts, renderComment(c, width))
	}
	return strings.Join(parts, "\n\n")
}

func renderComment(c Comment, width int) string {
	isAgent := c.Author == "agent"
	var nameStyle lipgloss.Style
	var label string
	if isAgent {
		nameStyle = styleAgentName
		label = "agent"
	} else {
		nameStyle = styleUserName
		label = "you"
	}

	header := nameStyle.Render(label) + styleMuted.Render("  "+c.CreatedAt.Format(time.Kitchen))

	// render markdown for agent comments
	body := c.Body
	if isAgent {
		r, err := glamour.NewTermRenderer(
			glamour.WithStandardStyle("dark"),
			glamour.WithWordWrap(width-6),
		)
		if err == nil {
			if rendered, err := r.Render(body); err == nil {
				body = strings.TrimRight(rendered, "\n")
			}
		}
	} else {
		body = lipgloss.NewStyle().Foreground(colorText).Width(width - 4).Render(body)
	}

	// file links
	var fileSection string
	if len(c.Files) > 0 {
		links := make([]string, len(c.Files))
		for i, f := range c.Files {
			links[i] = lipgloss.NewStyle().Foreground(colorPrimary).Render("↗ " + f)
		}
		fileSection = "\n" + styleMuted.Render("files: ") + strings.Join(links, "  ")
	}

	// email draft
	emailSection := renderEmailDraft(c.EmailDraft)

	borderColor := colorAgent
	if !isAgent {
		borderColor = colorUser
	}

	return lipgloss.NewStyle().
		BorderLeft(true).
		BorderStyle(lipgloss.ThickBorder()).
		BorderForeground(borderColor).
		PaddingLeft(1).
		Render(header + "\n" + body + fileSection + emailSection)
}

func renderEmailDraft(d *EmailDraft) string {
	if d == nil {
		return ""
	}

	envelope := lipgloss.NewStyle().Foreground(colorUnread).Render("✉")

	switch d.Status {
	case "sent":
		ts := ""
		if d.SentAt != nil {
			ts = "  " + styleMuted.Render(d.SentAt.Format(time.Kitchen))
		}
		return "\n" + envelope + " " +
			lipgloss.NewStyle().Foreground(colorDone).Render("Sent → "+d.To) + ts

	case "rejected":
		return "\n" + envelope + " " + styleMuted.Render("Draft rejected")
	}

	// pending
	subj := styleMuted.Render("Subject: ") + d.Subject
	to := styleMuted.Render("To:      ") + d.To

	// preview first 3 lines of body
	bodyLines := strings.Split(strings.TrimSpace(d.Body), "\n")
	if len(bodyLines) > 3 {
		bodyLines = append(bodyLines[:3], styleMuted.Render("…"))
	}
	preview := strings.Join(bodyLines, "\n")

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorUnread).
		Padding(0, 1).
		MarginTop(1).
		Render(
			envelope + " " + lipgloss.NewStyle().Foreground(colorUnread).Bold(true).Render("Draft ready") +
				"\n" + to + "\n" + subj +
				"\n" + styleMuted.Render(strings.Repeat("─", 30)) +
				"\n" + lipgloss.NewStyle().Foreground(colorSubtext).Render(preview),
		)
	return "\n" + box
}

// ── file loading ──────────────────────────────────────────────────────────────

func readFile(path, workDir string) (string, error) {
	// try as-is (absolute path)
	if data, err := os.ReadFile(path); err == nil {
		return string(data), nil
	}
	// try relative to workDir
	joined := filepath.Join(workDir, path)
	if data, err := os.ReadFile(joined); err == nil {
		return string(data), nil
	}
	// workDir may already end with "projects/" while the stored path starts
	// with "projects/", producing a doubled segment — strip it and retry.
	base := filepath.Base(filepath.Clean(workDir))
	stripped := path
	if base == "projects" && strings.HasPrefix(path, "projects/") {
		stripped = path[len("projects/"):]
	}
	if stripped != path {
		if data, err := os.ReadFile(filepath.Join(workDir, stripped)); err == nil {
			return string(data), nil
		}
	}
	return "", fmt.Errorf("%s: no such file", joined)
}

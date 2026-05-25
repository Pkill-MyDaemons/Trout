package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type cfgField int

const (
	cfgDaemonMode cfgField = iota
	cfgNightlyTime
	cfgProvider
	cfgModel
	cfgAPIKey
	cfgWorkDir
	cfgLocalURL
	cfgDaemonBtn
	cfgFieldCount
)

type configModel struct {
	store         *Store
	cfg           *Config
	cursor        cfgField
	editing       bool
	input         textinput.Model
	daemonRunning bool
	daemonPID     int
	statusMsg     string
	width, height int
}

func newConfigModel(store *Store, cfg *Config, w, h int) configModel {
	ti := textinput.New()
	ti.CharLimit = 256

	running, pid := isDaemonRunning()
	return configModel{
		store:         store,
		cfg:           cfg,
		input:         ti,
		daemonRunning: running,
		daemonPID:     pid,
		width:         w,
		height:        h,
	}
}

func (m configModel) Init() tea.Cmd { return nil }

func (m configModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case tea.KeyMsg:
		if m.editing {
			return m.updateEdit(msg)
		}
		return m.updateNav(msg)
	}
	if m.editing {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m configModel) updateNav(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	maxField := int(cfgFieldCount) - 1
	// hide local URL field unless provider is local
	if m.cfg.Provider != "local" && m.cursor == cfgLocalURL {
		m.cursor = cfgModel
	}

	switch msg.String() {
	case "esc", "q":
		return newListModel(m.store), nil
	case "ctrl+c":
		return m, tea.Quit

	case "up", "k":
		m.cursor--
		if m.cursor < 0 {
			m.cursor = cfgField(maxField)
		}
		m = m.skipHiddenUp()

	case "down", "j", "tab":
		m.cursor++
		if int(m.cursor) > maxField {
			m.cursor = 0
		}
		m = m.skipHiddenDown()

	case "enter", " ":
		return m.activate()

	case "left", "right", "h", "l":
		// cycle toggle fields inline
		switch m.cursor {
		case cfgDaemonMode:
			if m.cfg.DaemonMode == DaemonModeNightly {
				m.cfg.DaemonMode = DaemonModeResponsive
			} else {
				m.cfg.DaemonMode = DaemonModeNightly
			}
			_ = saveConfig(m.cfg)
		case cfgProvider:
			m.cfg.Provider = cycleProvider(m.cfg.Provider, msg.String() == "right" || msg.String() == "l")
			m.cfg.Model = defaultModels[m.cfg.Provider]
			_ = saveConfig(m.cfg)
		}
	}
	return m, nil
}

// skipHiddenDown skips cfgLocalURL when provider != local.
func (m configModel) skipHiddenDown() configModel {
	if m.cfg.Provider != "local" && m.cursor == cfgLocalURL {
		m.cursor++
		if int(m.cursor) >= int(cfgFieldCount) {
			m.cursor = 0
		}
	}
	return m
}

func (m configModel) skipHiddenUp() configModel {
	if m.cfg.Provider != "local" && m.cursor == cfgLocalURL {
		m.cursor--
		if m.cursor < 0 {
			m.cursor = cfgField(int(cfgFieldCount) - 1)
		}
	}
	return m
}

func (m configModel) activate() (tea.Model, tea.Cmd) {
	switch m.cursor {
	case cfgDaemonMode:
		if m.cfg.DaemonMode == DaemonModeNightly {
			m.cfg.DaemonMode = DaemonModeResponsive
		} else {
			m.cfg.DaemonMode = DaemonModeNightly
		}
		_ = saveConfig(m.cfg)

	case cfgProvider:
		m.cfg.Provider = cycleProvider(m.cfg.Provider, true)
		m.cfg.Model = defaultModels[m.cfg.Provider]
		_ = saveConfig(m.cfg)

	case cfgDaemonBtn:
		if m.daemonRunning {
			if err := stopDaemon(); err != nil {
				m.statusMsg = "stop failed: " + err.Error()
			} else {
				m.statusMsg = "Daemon stopped."
				m.daemonRunning = false
				m.daemonPID = 0
			}
		} else {
			if err := startDaemon(); err != nil {
				m.statusMsg = "start failed: " + err.Error()
			} else {
				m.daemonRunning, m.daemonPID = isDaemonRunning()
				m.statusMsg = fmt.Sprintf("Daemon started (pid %d).", m.daemonPID)
			}
		}

	default:
		// text field → enter edit mode
		m.editing = true
		m.input.SetValue(m.textFieldValue())
		m.input.SetCursor(len(m.input.Value()))
		if m.cursor == cfgAPIKey {
			m.input.EchoMode = textinput.EchoNormal
		}
		m.input.Focus()
		return m, textinput.Blink
	}
	return m, nil
}

func (m configModel) updateEdit(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.editing = false
		m.input.Blur()
		return m, nil
	case "enter":
		m.applyEdit(m.input.Value())
		_ = saveConfig(m.cfg)
		m.editing = false
		m.input.Blur()
		return m, nil
	case "ctrl+c":
		return m, tea.Quit
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m *configModel) textFieldValue() string {
	switch m.cursor {
	case cfgNightlyTime:
		return m.cfg.NightlyTime
	case cfgModel:
		return m.cfg.Model
	case cfgAPIKey:
		return m.cfg.APIKey
	case cfgWorkDir:
		return m.cfg.WorkDir
	case cfgLocalURL:
		return m.cfg.LocalURL
	}
	return ""
}

func (m *configModel) applyEdit(val string) {
	switch m.cursor {
	case cfgNightlyTime:
		m.cfg.NightlyTime = val
	case cfgModel:
		m.cfg.Model = val
	case cfgAPIKey:
		m.cfg.APIKey = val
	case cfgWorkDir:
		m.cfg.WorkDir = val
	case cfgLocalURL:
		m.cfg.LocalURL = val
	}
}

func cycleProvider(current string, forward bool) string {
	for i, p := range providers {
		if p == current {
			if forward {
				return providers[(i+1)%len(providers)]
			}
			return providers[(i+len(providers)-1)%len(providers)]
		}
	}
	return providers[0]
}

func (m configModel) View() string {
	rows := m.buildRows()
	content := styleTitle.Render("Config") + "\n\n" +
		strings.Join(rows, "\n") +
		"\n\n" + styleHelp.Render("↑↓ navigate  •  enter/space select  •  ←→ cycle toggles  •  esc back")

	if m.statusMsg != "" {
		content += "\n" + lipgloss.NewStyle().Foreground(colorDone).Render(m.statusMsg)
	}

	return lipgloss.NewStyle().Padding(1, 2).Render(content)
}

func (m configModel) buildRows() []string {
	label := func(s string) string {
		return lipgloss.NewStyle().Foreground(colorSubtext).Width(18).Render(s)
	}
	toggle := func(val string) string {
		return lipgloss.NewStyle().Foreground(colorPrimary).Bold(true).Render("[" + val + "]")
	}
	textVal := func(val string) string {
		if val == "" {
			return styleMuted.Render("(not set)")
		}
		return lipgloss.NewStyle().Foreground(colorText).Render(val)
	}

	rows := []struct {
		field cfgField
		label string
		value string
	}{
		{cfgDaemonMode, "Daemon mode", toggle(string(m.cfg.DaemonMode))},
		{cfgNightlyTime, "Run at", textVal(m.cfg.NightlyTime)},
		{cfgProvider, "Provider", toggle(m.cfg.Provider)},
		{cfgModel, "Model", textVal(m.cfg.Model)},
		{cfgAPIKey, "API key", textVal(maskKey(m.cfg.APIKey))},
		{cfgWorkDir, "Work dir", textVal(m.cfg.WorkDir)},
	}

	var lines []string
	for _, r := range rows {
		if r.field == cfgLocalURL {
			continue // handled separately
		}
		val := r.value
		// replace with live input when editing
		if m.editing && m.cursor == r.field {
			val = m.input.View()
		}
		line := label(r.label) + val
		if m.cursor == r.field && !m.editing {
			line = lipgloss.NewStyle().
				Background(colorSelected).
				Foreground(colorText).
				Padding(0, 1).
				Render(fmt.Sprintf("%-*s%s", 18, r.label, val))
		}
		lines = append(lines, "  "+line)
	}

	// local URL only when provider = local
	if m.cfg.Provider == "local" {
		urlVal := textVal(m.cfg.LocalURL)
		if m.editing && m.cursor == cfgLocalURL {
			urlVal = m.input.View()
		}
		line := label("Local URL") + urlVal
		if m.cursor == cfgLocalURL && !m.editing {
			line = lipgloss.NewStyle().
				Background(colorSelected).
				Foreground(colorText).
				Padding(0, 1).
				Render(fmt.Sprintf("%-18s%s", "Local URL", urlVal))
		}
		lines = append(lines, "  "+line)
	}

	// daemon status + button
	lines = append(lines, "")
	statusStr, btnStr := m.daemonStatusRow()
	btnLine := label("Daemon") + statusStr + "  " + btnStr
	if m.cursor == cfgDaemonBtn {
		btnLine = lipgloss.NewStyle().
			Background(colorSelected).
			Foreground(colorText).
			Padding(0, 1).
			Render(fmt.Sprintf("%-18s%s  %s", "Daemon", statusStr, btnStr))
	}
	lines = append(lines, "  "+btnLine)

	return lines
}

func (m configModel) daemonStatusRow() (status, btn string) {
	if m.daemonRunning {
		status = styleUnreadDot.Render("● ") +
			lipgloss.NewStyle().Foreground(colorDone).Render(fmt.Sprintf("running (pid %d)", m.daemonPID))
		btn = lipgloss.NewStyle().Foreground(lipgloss.Color("#EF4444")).Bold(true).Render("[Stop]")
	} else {
		status = styleMuted.Render("○ stopped")
		btn = lipgloss.NewStyle().Foreground(colorDone).Bold(true).Render("[Start]")
	}
	return
}

func maskKey(k string) string {
	if len(k) <= 8 {
		return strings.Repeat("*", len(k))
	}
	return k[:4] + strings.Repeat("*", len(k)-8) + k[len(k)-4:]
}

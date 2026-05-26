package main

import (
	"fmt"
	"strconv"
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
	// email section
	cfgEmailMode
	cfgEmailProvider
	cfgFromAddr
	// gmail fields
	cfgGmailClientID
	cfgGmailClientSecret
	cfgGmailAuthBtn
	cfgGmailPollEnabled
	// smtp fields
	cfgSMTPHost
	cfgSMTPPort
	cfgSMTPUser
	cfgSMTPPass
	// daemon button
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
	case oauthDoneMsg:
		if msg.err != nil {
			m.statusMsg = "Auth failed: " + msg.err.Error()
		} else {
			m.statusMsg = "Gmail authenticated!"
			if msg.email != "" {
				m.cfg.FromAddr = msg.email
				_ = saveConfig(m.cfg)
			}
		}
		return m, nil
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
	max := int(cfgFieldCount) - 1
	switch msg.String() {
	case "esc", "q":
		return newListModel(m.store), nil
	case "ctrl+c":
		return m, tea.Quit
	case "up", "k":
		m.cursor--
		if m.cursor < 0 {
			m.cursor = cfgField(max)
		}
		m = m.skipHidden(-1)
	case "down", "j", "tab":
		m.cursor++
		if int(m.cursor) > max {
			m.cursor = 0
		}
		m = m.skipHidden(1)
	case "enter", " ":
		return m.activate()
	case "left", "right", "h", "l":
		fwd := msg.String() == "right" || msg.String() == "l"
		switch m.cursor {
		case cfgDaemonMode:
			if fwd {
				if m.cfg.DaemonMode == DaemonModeNightly {
					m.cfg.DaemonMode = DaemonModeResponsive
				} else {
					m.cfg.DaemonMode = DaemonModeNightly
				}
			} else {
				if m.cfg.DaemonMode == DaemonModeResponsive {
					m.cfg.DaemonMode = DaemonModeNightly
				} else {
					m.cfg.DaemonMode = DaemonModeResponsive
				}
			}
			_ = saveConfig(m.cfg)
		case cfgProvider:
			m.cfg.Provider = cycleProvider(m.cfg.Provider, fwd)
			m.cfg.Model = defaultModels[m.cfg.Provider]
			_ = saveConfig(m.cfg)
		case cfgEmailMode:
			m.cfg.EmailMode = cycleEmailMode(m.cfg.EmailMode, fwd)
			_ = saveConfig(m.cfg)
		case cfgEmailProvider:
			m.cfg.EmailProvider = cycleEmailProvider(m.cfg.EmailProvider, fwd)
			_ = saveConfig(m.cfg)
			m = m.skipHidden(1)
		case cfgGmailPollEnabled:
			m.cfg.GmailPollEnabled = !m.cfg.GmailPollEnabled
			_ = saveConfig(m.cfg)
		}
	}
	return m, nil
}

// skipHidden advances the cursor past fields that are hidden based on current config.
func (m configModel) skipHidden(dir int) configModel {
	max := int(cfgFieldCount) - 1
	for {
		var skip bool
		switch m.cursor {
		case cfgLocalURL:
			skip = m.cfg.Provider != "local"
		case cfgGmailClientID, cfgGmailClientSecret, cfgGmailAuthBtn, cfgGmailPollEnabled:
			skip = m.cfg.EmailProvider != "gmail"
		case cfgSMTPHost, cfgSMTPPort, cfgSMTPUser, cfgSMTPPass:
			skip = m.cfg.EmailProvider != "smtp"
		}
		if !skip {
			break
		}
		m.cursor = cfgField(int(m.cursor) + dir)
		if int(m.cursor) > max {
			m.cursor = 0
		}
		if m.cursor < 0 {
			m.cursor = cfgField(max)
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
	case cfgEmailMode:
		m.cfg.EmailMode = cycleEmailMode(m.cfg.EmailMode, true)
		_ = saveConfig(m.cfg)
	case cfgEmailProvider:
		m.cfg.EmailProvider = cycleEmailProvider(m.cfg.EmailProvider, true)
		_ = saveConfig(m.cfg)
	case cfgGmailPollEnabled:
		m.cfg.GmailPollEnabled = !m.cfg.GmailPollEnabled
		_ = saveConfig(m.cfg)
	case cfgGmailAuthBtn:
		if m.cfg.GmailClientID == "" || m.cfg.GmailClientSecret == "" {
			m.statusMsg = "Set Client ID and Client Secret first."
			return m, nil
		}
		m.statusMsg = "Opening browser for Gmail sign-in…"
		return m, startOAuthFlowCmd(m.cfg.GmailClientID, m.cfg.GmailClientSecret)
	case cfgDaemonBtn:
		if m.daemonRunning {
			if err := stopDaemon(); err != nil {
				m.statusMsg = "stop failed: " + err.Error()
			} else {
				m.statusMsg = "Daemon stopped."
				m.daemonRunning, m.daemonPID = false, 0
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
		m.editing = true
		m.input.SetValue(m.textFieldValue())
		m.input.SetCursor(len(m.input.Value()))
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
	case cfgFromAddr:
		return m.cfg.FromAddr
	case cfgGmailClientID:
		return m.cfg.GmailClientID
	case cfgGmailClientSecret:
		return m.cfg.GmailClientSecret
	case cfgSMTPHost:
		return m.cfg.SMTPHost
	case cfgSMTPPort:
		if m.cfg.SMTPPort == 0 {
			return ""
		}
		return strconv.Itoa(m.cfg.SMTPPort)
	case cfgSMTPUser:
		return m.cfg.SMTPUser
	case cfgSMTPPass:
		return m.cfg.SMTPPassword
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
	case cfgFromAddr:
		m.cfg.FromAddr = val
	case cfgGmailClientID:
		m.cfg.GmailClientID = val
	case cfgGmailClientSecret:
		m.cfg.GmailClientSecret = val
	case cfgSMTPHost:
		m.cfg.SMTPHost = val
	case cfgSMTPPort:
		if p, err := strconv.Atoi(val); err == nil {
			m.cfg.SMTPPort = p
		}
	case cfgSMTPUser:
		m.cfg.SMTPUser = val
	case cfgSMTPPass:
		m.cfg.SMTPPassword = val
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

func cycleEmailProvider(current string, forward bool) string {
	opts := []string{"smtp", "gmail"}
	for i, v := range opts {
		if v == current {
			if forward {
				return opts[(i+1)%len(opts)]
			}
			return opts[(i+len(opts)-1)%len(opts)]
		}
	}
	return "smtp"
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
	lbl := func(s string) string {
		return lipgloss.NewStyle().Foreground(colorSubtext).Width(18).Render(s)
	}
	tog := func(val string) string {
		return lipgloss.NewStyle().Foreground(colorPrimary).Bold(true).Render("[" + val + "]")
	}
	tv := func(val string) string {
		if val == "" {
			return styleMuted.Render("(not set)")
		}
		return lipgloss.NewStyle().Foreground(colorText).Render(val)
	}
	sectionHdr := func(title string) string {
		return "\n  " + lipgloss.NewStyle().Foreground(colorMuted).Render(
			"── "+title+" "+strings.Repeat("─", 30-len(title)),
		)
	}

	type row struct {
		field cfgField
		label string
		value string
	}

	var lines []string

	// Core settings
	coreRows := []row{
		{cfgDaemonMode, "Daemon mode", tog(string(m.cfg.DaemonMode))},
		{cfgNightlyTime, "Run at", tv(m.cfg.NightlyTime)},
		{cfgProvider, "Provider", tog(m.cfg.Provider)},
		{cfgModel, "Model", tv(m.cfg.Model)},
		{cfgAPIKey, "API key", tv(maskKey(m.cfg.APIKey))},
		{cfgWorkDir, "Work dir", tv(m.cfg.WorkDir)},
	}
	for _, r := range coreRows {
		lines = append(lines, m.renderRow(r.field, r.label, r.value, lbl))
	}
	if m.cfg.Provider == "local" {
		val := tv(m.cfg.LocalURL)
		if m.editing && m.cursor == cfgLocalURL {
			val = m.input.View()
		}
		lines = append(lines, m.renderRowRaw(cfgLocalURL, "Local URL", val, lbl))
	}

	// Email section
	lines = append(lines, sectionHdr("Email"))
	lines = append(lines, m.renderRow(cfgEmailMode, "Email mode", tog(emailModeLabel(m.cfg.EmailMode)), lbl))
	lines = append(lines, m.renderRow(cfgEmailProvider, "Provider", tog(m.cfg.EmailProvider), lbl))
	lines = append(lines, m.renderRow(cfgFromAddr, "From address", tv(m.cfg.FromAddr), lbl))

	if m.cfg.EmailProvider == "gmail" {
		lines = append(lines, sectionHdr("Gmail OAuth"))
		lines = append(lines, m.renderRow(cfgGmailClientID, "Client ID", tv(maskKey(m.cfg.GmailClientID)), lbl))
		lines = append(lines, m.renderRow(cfgGmailClientSecret, "Client secret", tv(maskKey(m.cfg.GmailClientSecret)), lbl))

		pollVal := "off"
		if m.cfg.GmailPollEnabled {
			pollVal = "on"
		}
		lines = append(lines, m.renderRow(cfgGmailPollEnabled, "Inbox polling", tog(pollVal), lbl))

		// Auth button row
		authStatus := styleMuted.Render("not authenticated")
		if isOAuthAuthenticated() {
			authStatus = lipgloss.NewStyle().Foreground(colorDone).Render("✓ authenticated")
		}
		btnLabel := lipgloss.NewStyle().Foreground(colorPrimary).Bold(true).Render("[Authenticate Gmail]")
		btnLine := lbl("Sign in") + authStatus + "  " + btnLabel
		if m.cursor == cfgGmailAuthBtn {
			btnLine = lipgloss.NewStyle().
				Background(colorSelected).Foreground(colorText).Padding(0, 1).
				Render(fmt.Sprintf("%-18s%s  %s", "Sign in", authStatus, "[Authenticate Gmail]"))
		}
		lines = append(lines, "  "+btnLine)
	} else {
		lines = append(lines, sectionHdr("SMTP"))
		smtpRows := []row{
			{cfgSMTPHost, "SMTP host", tv(m.cfg.SMTPHost)},
			{cfgSMTPPort, "SMTP port", tv(strconv.Itoa(m.cfg.SMTPPort))},
			{cfgSMTPUser, "SMTP user", tv(m.cfg.SMTPUser)},
			{cfgSMTPPass, "SMTP password", tv(maskKey(m.cfg.SMTPPassword))},
		}
		for _, r := range smtpRows {
			lines = append(lines, m.renderRow(r.field, r.label, r.value, lbl))
		}
	}

	// Daemon button
	lines = append(lines, "")
	statusStr, btnStr := m.daemonStatusRow()
	btnLine := lbl("Daemon") + statusStr + "  " + btnStr
	if m.cursor == cfgDaemonBtn {
		btnLine = lipgloss.NewStyle().
			Background(colorSelected).Foreground(colorText).Padding(0, 1).
			Render(fmt.Sprintf("%-18s%s  %s", "Daemon", statusStr, btnStr))
	}
	lines = append(lines, "  "+btnLine)
	return lines
}

func (m configModel) renderRow(field cfgField, label, value string, lbl func(string) string) string {
	val := value
	if m.editing && m.cursor == field {
		val = m.input.View()
	}
	return m.renderRowRaw(field, label, val, lbl)
}

func (m configModel) renderRowRaw(field cfgField, label, val string, lbl func(string) string) string {
	if m.cursor == field && !m.editing {
		return "  " + lipgloss.NewStyle().
			Background(colorSelected).Foreground(colorText).Padding(0, 1).
			Render(fmt.Sprintf("%-18s%s", label, val))
	}
	return "  " + lbl(label) + val
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
	if k == "" {
		return ""
	}
	if len(k) <= 8 {
		return strings.Repeat("*", len(k))
	}
	return k[:4] + strings.Repeat("*", len(k)-8) + k[len(k)-4:]
}

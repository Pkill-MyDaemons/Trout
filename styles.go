package main

import "github.com/charmbracelet/lipgloss"

var (
	colorPrimary  = lipgloss.Color("#7C3AED")
	colorMuted    = lipgloss.Color("#6B7280")
	colorBorder   = lipgloss.Color("#374151")
	colorUnread   = lipgloss.Color("#F59E0B")
	colorDone     = lipgloss.Color("#10B981")
	colorInProg   = lipgloss.Color("#3B82F6")
	colorTodo     = lipgloss.Color("#6B7280")
	colorAgent    = lipgloss.Color("#A78BFA")
	colorUser     = lipgloss.Color("#34D399")
	colorSelected = lipgloss.Color("#1F2937")
	colorBg       = lipgloss.Color("#111827")
	colorText     = lipgloss.Color("#F9FAFB")
	colorSubtext  = lipgloss.Color("#9CA3AF")

	styleTitle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorPrimary).
			MarginBottom(1)

	styleSelected = lipgloss.NewStyle().
			Background(colorSelected).
			Foreground(colorText)

	styleUnreadDot = lipgloss.NewStyle().
			Foreground(colorUnread).
			Bold(true)

	styleMuted = lipgloss.NewStyle().Foreground(colorMuted)

	styleBorder = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorBorder)

	styleStatusTodo = lipgloss.NewStyle().
			Foreground(colorTodo).
			Bold(true)

	styleStatusInProg = lipgloss.NewStyle().
				Foreground(colorInProg).
				Bold(true)

	styleStatusDone = lipgloss.NewStyle().
			Foreground(colorDone).
			Bold(true)

	styleAgentName = lipgloss.NewStyle().
			Foreground(colorAgent).
			Bold(true)

	styleUserName = lipgloss.NewStyle().
			Foreground(colorUser).
			Bold(true)

	styleHelp = lipgloss.NewStyle().
			Foreground(colorMuted).
			MarginTop(1)
)

func statusStyle(s Status) lipgloss.Style {
	switch s {
	case StatusInProgress:
		return styleStatusInProg
	case StatusDone:
		return styleStatusDone
	default:
		return styleStatusTodo
	}
}

func statusLabel(s Status) string {
	switch s {
	case StatusInProgress:
		return "in progress"
	case StatusDone:
		return "done"
	default:
		return "todo"
	}
}

package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type listView int

const (
	viewList listView = iota
	viewAddTitle
	viewAddDesc
)

type listModel struct {
	store    *Store
	cursor   int
	view     listView
	inputTitle textinput.Model
	inputDesc  textinput.Model
	width    int
	height   int
}

func newListModel(store *Store) listModel {
	ti := textinput.New()
	ti.Placeholder = "Task title..."
	ti.CharLimit = 120

	td := textinput.New()
	td.Placeholder = "Description (optional)..."
	td.CharLimit = 500

	return listModel{
		store:      store,
		inputTitle: ti,
		inputDesc:  td,
	}
}

func (m listModel) Init() tea.Cmd {
	return nil
}

func (m listModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case tea.KeyMsg:
		switch m.view {
		case viewList:
			return m.updateList(msg)
		case viewAddTitle:
			return m.updateAddTitle(msg)
		case viewAddDesc:
			return m.updateAddDesc(msg)
		}
	}

	if m.view == viewAddTitle {
		var cmd tea.Cmd
		m.inputTitle, cmd = m.inputTitle.Update(msg)
		return m, cmd
	}
	if m.view == viewAddDesc {
		var cmd tea.Cmd
		m.inputDesc, cmd = m.inputDesc.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m listModel) updateList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	tasks := m.store.Tasks
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit

	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}

	case "down", "j":
		if m.cursor < len(tasks)-1 {
			m.cursor++
		}

	case "n":
		m.view = viewAddTitle
		m.inputTitle.SetValue("")
		m.inputTitle.Focus()
		return m, textinput.Blink

	case "d":
		if len(tasks) > 0 {
			m.store.deleteTask(tasks[m.cursor].ID)
			if m.cursor >= len(m.store.Tasks) && m.cursor > 0 {
				m.cursor--
			}
		}

	case "s":
		if len(tasks) > 0 {
			t := tasks[m.cursor]
			switch t.Status {
			case StatusTodo:
				t.Status = StatusInProgress
			case StatusInProgress:
				t.Status = StatusDone
			case StatusDone:
				t.Status = StatusTodo
			}
			_ = saveStore(m.store)
		}

	case "c":
		cfg, _ := loadConfig()
		if cfg == nil {
			cfg = defaultConfig()
		}
		return newConfigModel(m.store, cfg, m.width, m.height), nil

	case "enter":
		if len(tasks) > 0 {
			task := tasks[m.cursor]
			if task.HasUnread {
				task.HasUnread = false
				_ = saveStore(m.store)
			}
			detail := newDetailModel(m.store, task, m.width, m.height)
			return detail, nil
		}
	}
	return m, nil
}

func (m listModel) updateAddTitle(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.view = viewList
		return m, nil
	case "enter":
		if strings.TrimSpace(m.inputTitle.Value()) != "" {
			m.view = viewAddDesc
			m.inputDesc.SetValue("")
			m.inputDesc.Focus()
			return m, textinput.Blink
		}
	case "ctrl+c":
		return m, tea.Quit
	}
	var cmd tea.Cmd
	m.inputTitle, cmd = m.inputTitle.Update(msg)
	return m, cmd
}

func (m listModel) updateAddDesc(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.view = viewList
		return m, nil
	case "enter":
		m.store.addTask(
			strings.TrimSpace(m.inputTitle.Value()),
			strings.TrimSpace(m.inputDesc.Value()),
		)
		m.cursor = len(m.store.Tasks) - 1
		m.view = viewList
		return m, nil
	case "ctrl+c":
		return m, tea.Quit
	}
	var cmd tea.Cmd
	m.inputDesc, cmd = m.inputDesc.Update(msg)
	return m, cmd
}

func (m listModel) View() string {
	switch m.view {
	case viewAddTitle:
		return m.renderInput("New task — title", m.inputTitle.View(), "enter to continue • esc to cancel")
	case viewAddDesc:
		return m.renderInput("New task — description", m.inputDesc.View(), "enter to save • esc to cancel")
	}
	return m.renderList()
}

func (m listModel) renderInput(header, input, help string) string {
	box := styleBorder.
		Width(60).
		Padding(1, 2).
		Render(
			styleTitle.Render(header) + "\n\n" +
				input + "\n\n" +
				styleHelp.Render(help),
		)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}

func (m listModel) renderList() string {
	tasks := m.store.Tasks

	daemonStatus := ""
	if running, _ := isDaemonRunning(); running {
		daemonStatus = "  " + styleUnreadDot.Render("●") + styleMuted.Render(" daemon")
	}
	header := styleTitle.Render("Tasks") + styleMuted.Render(fmt.Sprintf("  %d task(s)", len(tasks))) + daemonStatus

	var rows []string
	for i, t := range tasks {
		dot := "  "
		if t.HasUnread {
			dot = styleUnreadDot.Render("● ")
		}

		title := t.Title
		if i == m.cursor {
			title = styleSelected.
				Width(m.width - 20).
				Padding(0, 1).
				Render(title)
		}

		status := statusStyle(t.Status).Render(fmt.Sprintf("[%s]", statusLabel(t.Status)))
		row := fmt.Sprintf("%s%-*s  %s", dot, m.width-22, title, status)
		rows = append(rows, row)
	}

	if len(rows) == 0 {
		rows = append(rows, styleMuted.Render("  No tasks yet. Press n to add one."))
	}

	help := styleHelp.Render("n new  •  enter open  •  s cycle status  •  d delete  •  c config  •  q quit")

	content := header + "\n\n" + strings.Join(rows, "\n") + "\n\n" + help
	return lipgloss.NewStyle().Padding(1, 2).Render(content)
}

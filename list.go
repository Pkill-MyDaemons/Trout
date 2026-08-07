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
	viewFilter
	viewTags
)

type listModel struct {
	store        *Store
	cursor       int
	view         listView
	inputTitle   textinput.Model
	inputDesc    textinput.Model
	filterInput  textinput.Model
	filterQuery  string
	tagsInput    textinput.Model
	width        int
	height       int
	spinnerFrame int
}

func newListModel(store *Store) listModel {
	ti := textinput.New()
	ti.Placeholder = "Task title..."
	ti.CharLimit = 120

	td := textinput.New()
	td.Placeholder = "Description (optional)..."
	td.CharLimit = 500

	fi := textinput.New()
	fi.Placeholder = "Filter by title, description, or tag..."
	fi.CharLimit = 120

	tg := textinput.New()
	tg.Placeholder = "tag-one, tag-two..."
	tg.CharLimit = 200

	return listModel{
		store:       store,
		inputTitle:  ti,
		inputDesc:   td,
		filterInput: fi,
		tagsInput:   tg,
	}
}

func (m listModel) Init() tea.Cmd {
	return storeTick(false)
}

// visibleTasks returns the tasks matching the current filter query (all
// tasks if no filter is set), searching title, description, and tags.
func (m listModel) visibleTasks() []*Task {
	if strings.TrimSpace(m.filterQuery) == "" {
		return m.store.Tasks
	}
	q := strings.ToLower(m.filterQuery)
	var out []*Task
	for _, t := range m.store.Tasks {
		if strings.Contains(strings.ToLower(t.Title), q) ||
			strings.Contains(strings.ToLower(t.Description), q) {
			out = append(out, t)
			continue
		}
		for _, tag := range t.Tags {
			if strings.Contains(strings.ToLower(tag), q) {
				out = append(out, t)
				break
			}
		}
	}
	return out
}

func (m listModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case storeTickMsg:
		anyProcessing := false
		if fresh, err := loadStore(); err == nil {
			m.store = fresh
		}
		tasks := m.visibleTasks()
		if m.cursor >= len(tasks) && m.cursor > 0 {
			m.cursor = len(tasks) - 1
		}
		for _, t := range m.store.Tasks {
			if t.Processing {
				anyProcessing = true
				break
			}
		}
		m.spinnerFrame++
		return m, storeTick(anyProcessing)

	case tea.KeyMsg:
		switch m.view {
		case viewList:
			return m.updateList(msg)
		case viewAddTitle:
			return m.updateAddTitle(msg)
		case viewAddDesc:
			return m.updateAddDesc(msg)
		case viewFilter:
			return m.updateFilter(msg)
		case viewTags:
			return m.updateTags(msg)
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
	tasks := m.visibleTasks()
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

	case "/":
		m.view = viewFilter
		m.filterInput.SetValue(m.filterQuery)
		m.filterInput.Focus()
		return m, textinput.Blink

	case "t":
		if len(tasks) > 0 {
			m.view = viewTags
			m.tagsInput.SetValue(strings.Join(tasks[m.cursor].Tags, ", "))
			m.tagsInput.Focus()
			return m, textinput.Blink
		}

	case "d":
		if len(tasks) > 0 {
			m.store.deleteTask(tasks[m.cursor].ID)
			if m.cursor >= len(m.visibleTasks()) && m.cursor > 0 {
				m.cursor--
			}
		}

	case "s":
		if len(tasks) > 0 {
			m.store.cycleStatus(tasks[m.cursor].ID)
		}

	case "R":
		if fresh, err := loadStore(); err == nil {
			m.store = fresh
			if m.cursor >= len(m.visibleTasks()) && m.cursor > 0 {
				m.cursor = len(m.visibleTasks()) - 1
			}
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
				if t := m.store.markRead(task.ID); t != nil {
					task = t
				}
			}
			detail := newDetailModel(m.store, task, m.width, m.height)
			return detail, storeTick(task.Processing)
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
		m.cursor = len(m.visibleTasks()) - 1
		m.view = viewList
		return m, nil
	case "ctrl+c":
		return m, tea.Quit
	}
	var cmd tea.Cmd
	m.inputDesc, cmd = m.inputDesc.Update(msg)
	return m, cmd
}

func (m listModel) updateFilter(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.view = viewList
		return m, nil
	case "enter":
		m.filterQuery = strings.TrimSpace(m.filterInput.Value())
		m.cursor = 0
		m.view = viewList
		return m, nil
	case "ctrl+c":
		return m, tea.Quit
	}
	var cmd tea.Cmd
	m.filterInput, cmd = m.filterInput.Update(msg)
	return m, cmd
}

func (m listModel) updateTags(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.view = viewList
		return m, nil
	case "enter":
		tasks := m.visibleTasks()
		if m.cursor < len(tasks) {
			var tags []string
			for _, part := range strings.Split(m.tagsInput.Value(), ",") {
				if p := strings.TrimSpace(part); p != "" {
					tags = append(tags, p)
				}
			}
			m.store.setTags(tasks[m.cursor].ID, tags)
		}
		m.view = viewList
		return m, nil
	case "ctrl+c":
		return m, tea.Quit
	}
	var cmd tea.Cmd
	m.tagsInput, cmd = m.tagsInput.Update(msg)
	return m, cmd
}

func (m listModel) View() string {
	switch m.view {
	case viewAddTitle:
		return m.renderInput("New task — title", m.inputTitle.View(), "enter to continue • esc to cancel")
	case viewAddDesc:
		return m.renderInput("New task — description", m.inputDesc.View(), "enter to save • esc to cancel")
	case viewFilter:
		return m.renderInput("Filter tasks", m.filterInput.View(), "enter to apply • empty + enter to clear • esc to cancel")
	case viewTags:
		return m.renderInput("Edit tags (comma-separated)", m.tagsInput.View(), "enter to save • esc to cancel")
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
	tasks := m.visibleTasks()

	daemonStatus := ""
	if running, _ := isDaemonRunning(); running {
		daemonStatus = "  " + styleUnreadDot.Render("●") + styleMuted.Render(" daemon")
	}
	header := styleTitle.Render("Tasks") + styleMuted.Render(fmt.Sprintf("  %d task(s)", len(tasks))) + daemonStatus
	if m.filterQuery != "" {
		header += styleMuted.Render(fmt.Sprintf("  •  filter: %q", m.filterQuery))
	}

	var rows []string
	for i, t := range tasks {
		dot := "  "
		if t.HasUnread {
			dot = styleUnreadDot.Render("● ")
		}

		title := t.Title
		if i == m.cursor {
			title = styleSelected.
				Width(m.width-20).
				Padding(0, 1).
				Render(title)
		}

		status := statusStyle(t.Status).Render(fmt.Sprintf("[%s]", statusLabel(t.Status)))
		row := fmt.Sprintf("%s%-*s  %s", dot, m.width-22, title, status)

		if t.Processing {
			row += "  " + styleActivity.Render(spinnerChar(m.spinnerFrame)+" "+truncate(t.CurrentActivity, 40))
		} else if len(t.Tags) > 0 {
			var chips []string
			for _, tag := range t.Tags {
				chips = append(chips, styleTag.Render(tag))
			}
			row += "  " + strings.Join(chips, " ")
		}

		rows = append(rows, row)
	}

	if len(rows) == 0 {
		msg := "  No tasks yet. Press n to add one."
		if m.filterQuery != "" {
			msg = "  No tasks match the filter."
		}
		rows = append(rows, styleMuted.Render(msg))
	}

	help := styleHelp.Render("n new  •  enter open  •  s cycle status  •  t tags  •  / filter  •  d delete  •  R reload  •  c config  •  q quit")

	content := header + "\n\n" + strings.Join(rows, "\n") + "\n\n" + help
	return lipgloss.NewStyle().Padding(1, 2).Render(content)
}

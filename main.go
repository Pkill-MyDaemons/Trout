package main

import (
	"os"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--daemon" {
		cfg, err := loadConfig()
		if err != nil {
			cfg = defaultConfig()
		}
		runDaemonLoop(cfg)
		return
	}

	store, err := loadStore()
	if err != nil || store == nil {
		store = &Store{}
	}

	p := tea.NewProgram(newListModel(store), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		os.Exit(1)
	}
}

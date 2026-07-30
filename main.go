package main

import (
	"fmt"
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

	if len(os.Args) > 1 && os.Args[1] == "--run-title" {
		title, workDir, outPath, err := parseRunTitleArgs(os.Args[2:])
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}
		cfg, err := loadConfig()
		if err != nil {
			cfg = defaultConfig()
		}
		if err := runEphemeralTask(cfg, title, workDir, outPath); err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}
		fmt.Println("done")
		return
	}

	if len(os.Args) > 2 && os.Args[1] == "--run-task" {
		id := os.Args[2]
		cfg, err := loadConfig()
		if err != nil {
			cfg = defaultConfig()
		}
		store, err := loadStore()
		if err != nil {
			fmt.Fprintf(os.Stderr, "load store: %v\n", err)
			os.Exit(1)
		}
		var target *Task
		for _, t := range store.Tasks {
			if t.ID == id {
				target = t
				break
			}
		}
		if target == nil {
			fmt.Fprintf(os.Stderr, "task %q not found\n", id)
			os.Exit(1)
		}
		processTask(cfg, target, store)
		fmt.Println("done")
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

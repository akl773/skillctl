package main

import (
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"akhilsingh.in/skillctl/internal/config"
	"akhilsingh.in/skillctl/internal/ui"
)

func main() {
	sourceRepo := flag.String("source-repo", "", "Path to skills source repo (default: ~/.skills-curated)")
	version := flag.Bool("version", false, "Print version and exit")
	flag.Parse()

	if *version {
		fmt.Printf("skillctl v%s\n", config.Version)
		os.Exit(0)
	}

	paths := config.ResolvePaths(*sourceRepo)
	if err := config.EnsureSetup(paths); err != nil {
		fmt.Fprintf(os.Stderr, "[ERROR] %s\n", err)
		os.Exit(1)
	}

	m := ui.NewModel(paths)
	p := tea.NewProgram(m, tea.WithAltScreen())

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running skillctl: %s\n", err)
		os.Exit(1)
	}
}

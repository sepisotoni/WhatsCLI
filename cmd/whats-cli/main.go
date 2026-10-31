package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"whatscli/internal/app"
	"whatscli/internal/history"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "whats-cli:", err)
		os.Exit(1)
	}
}

func run() error {
	defaultDB, err := defaultDatabasePath()
	if err != nil {
		return err
	}
	dbPath := flag.String("db", defaultDB, "path to the local SQLite history database")
	flag.Parse()

	repo, err := history.Open(*dbPath)
	if err != nil {
		return err
	}
	defer repo.Close()

	program := tea.NewProgram(app.New(repo), tea.WithAltScreen())
	_, err = program.Run()
	return err
}

func defaultDatabasePath() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locate user config directory: %w", err)
	}
	return filepath.Join(configDir, "whats-cli", "history.db"), nil
}

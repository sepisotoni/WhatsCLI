package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
	"whatscli/internal/app"
	"whatscli/internal/config"
	"whatscli/internal/history"
	"whatscli/internal/messaging"
	"whatscli/internal/privatefs"
	"whatscli/internal/ui"
	"whatscli/internal/updater"
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
	stateDir := flag.String("state-dir", filepath.Dir(defaultDB), "private settings/session/history directory on this host")
	dbPath := flag.String("db", "", "override history database path")
	offline := flag.Bool("offline", false, "disable all WhatsApp network operations")
	demo := flag.Bool("demo", false, "explore synthetic conversations using disposable state; never connects to WhatsApp")
	version := flag.Bool("version", false, "show application version")
	colors := flag.String("colors", "auto", "colour depth: auto, truecolor or 256")
	flag.Parse()
	if *version {
		fmt.Println("WhatsCLI " + updater.BuildVersion)
		return nil
	}
	profile, ok := ui.ParseProfile(*colors, os.Getenv)
	if !ok {
		return errors.New("-colors must be auto, truecolor or 256")
	}
	ui.SetProfile(profile)
	if *demo {
		if *dbPath != "" {
			return errors.New("-demo cannot be combined with -db")
		}
		dir, err := os.MkdirTemp("", "whatscli-demo-*")
		if err != nil {
			return err
		}
		defer os.RemoveAll(dir)
		*stateDir, *offline = dir, true
	}
	if err := privatefs.EnsureDir(*stateDir); err != nil {
		return err
	}
	settingsPath := filepath.Join(*stateDir, "settings.json")
	settings, err := config.Load(settingsPath)
	if err != nil {
		return err
	}
	if *demo {
		settings.ConsentVersion = config.ConsentVersion
		settings.Mouse = true
	}
	path := *dbPath
	if path == "" {
		path = filepath.Join(*stateDir, "history.db")
	}
	repo, err := history.Open(path)
	if err != nil {
		return err
	}
	defer repo.Close()
	if *demo {
		if err := app.SeedDemo(context.Background(), repo); err != nil {
			return err
		}
	}
	var client messaging.Client
	if *offline {
		client = &offlineClient{events: make(chan messaging.Event)}
	} else {
		client, err = messaging.New(filepath.Join(*stateDir, "session.db"))
		if err != nil {
			return err
		}
	}
	runtime := app.NewRuntime(repo, client, settings.Accepted())
	defer runtime.Close()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	defer cancel()
	options := []tea.ProgramOption{tea.WithAltScreen(), tea.WithContext(ctx)}
	if settings.Mouse {
		options = append(options, tea.WithMouseAllMotion())
	}
	model := app.NewWithRuntime(runtime, settings, settingsPath)
	if *demo {
		model = app.DemoModel(runtime, settings, settingsPath)
	} else {
		model = model.WithState(filepath.Join(*stateDir, "ui-state.json"), *stateDir)
	}
	program := tea.NewProgram(model, options...)
	final, err := program.Run()
	if fm, ok := final.(app.Model); ok {
		fm.FlushState()
	}
	if errors.Is(err, tea.ErrProgramKilled) && ctx.Err() != nil {
		return nil
	}
	return err
}

func defaultDatabasePath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locate user config directory: %w", err)
	}
	return filepath.Join(dir, "whats-cli", "history.db"), nil
}

type offlineClient struct {
	events chan messaging.Event
	once   sync.Once
}

func (c *offlineClient) Connect(context.Context) error  { return errors.New("offline mode is enabled") }
func (c *offlineClient) Close() error                   { c.once.Do(func() { close(c.events) }); return nil }
func (c *offlineClient) Logout(context.Context) error   { return errors.New("offline mode is enabled") }
func (c *offlineClient) Events() <-chan messaging.Event { return c.events }
func (c *offlineClient) NewMessageID() string           { return "" }
func (c *offlineClient) SendText(context.Context, string, string, string) (history.IncomingEvent, error) {
	return history.IncomingEvent{}, errors.New("offline mode is enabled")
}
func (c *offlineClient) SendFile(context.Context, string, string, string) (history.IncomingEvent, error) {
	return history.IncomingEvent{}, errors.New("offline mode is enabled")
}

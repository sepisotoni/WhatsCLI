package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/unix"

	"whatscli/internal/privatefs"
)

const ConsentVersion = 1

// Settings are explicit user choices. MouseConfigured distinguishes an opt-out
// from older settings files that persisted the former disabled default.
type Settings struct {
	ConsentVersion  int  `json:"consent_version"`
	Mouse           bool `json:"mouse"`
	MouseConfigured bool `json:"mouse_configured,omitempty"`
	ReducedMotion   bool `json:"reduced_motion"`
	HidePhotos      bool `json:"hide_photos"`
	HideWallpaper   bool `json:"hide_wallpaper"`
	Clock12h        bool `json:"clock_12h"`
	Bell            bool `json:"bell"`
}

func (s Settings) Accepted() bool  { return s.ConsentVersion == ConsentVersion }
func (s Settings) Photos() bool    { return !s.HidePhotos }
func (s Settings) Wallpaper() bool { return !s.HideWallpaper }

// UIState is non-secret presentation state kept beside settings: read marks
// (for unread badges) and archived chats, both keyed by "account|chat".
type UIState struct {
	Read     map[string]time.Time `json:"read,omitempty"`
	Archived map[string]bool      `json:"archived,omitempty"`
}

// readJSON opens without following symlinks and validates the descriptor, so a
// path swapped between inspection and open cannot redirect the read.
func readJSON(path string, v any) error {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		if errors.Is(err, unix.ENOENT) {
			return os.ErrNotExist
		}
		return fmt.Errorf("open state file: %w", err)
	}
	file := os.NewFile(uintptr(fd), path)
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return errors.New("settings must be a regular file")
	}
	decoder := json.NewDecoder(io.LimitReader(file, 1<<20+1))
	if err := decoder.Decode(v); err != nil {
		return fmt.Errorf("read state file: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return errors.New("state file contains trailing or oversized data")
	}
	return nil
}

func writeJSON(path string, v any) error {
	if err := privatefs.EnsureDir(filepath.Dir(path)); err != nil {
		return err
	}
	if info, err := os.Lstat(path); err == nil && !info.Mode().IsRegular() {
		return errors.New("settings must be a regular file")
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".settings-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err = file.Write(append(data, '\n')); err != nil {
		file.Close()
		return err
	}
	if err = file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}

func Load(path string) (Settings, error) {
	var settings Settings
	err := readJSON(path, &settings)
	if errors.Is(err, os.ErrNotExist) {
		return Settings{Mouse: true, MouseConfigured: true}, nil
	}
	if err != nil {
		return Settings{}, err
	}
	if !settings.MouseConfigured {
		settings.Mouse = true
		settings.MouseConfigured = true
	}
	return settings, nil
}

func Save(path string, settings Settings) error {
	settings.MouseConfigured = true
	return writeJSON(path, settings)
}

func LoadState(path string) (UIState, error) {
	var state UIState
	err := readJSON(path, &state)
	if errors.Is(err, os.ErrNotExist) {
		return UIState{}, nil
	}
	if err != nil {
		return UIState{}, err
	}
	return state, nil
}

func SaveState(path string, state UIState) error { return writeJSON(path, state) }

package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"whatscli/internal/privatefs"
)

const ConsentVersion = 1

type Settings struct {
	ConsentVersion int  `json:"consent_version"`
	Mouse          bool `json:"mouse"`
}

func (s Settings) Accepted() bool { return s.ConsentVersion == ConsentVersion }

func Load(path string) (Settings, error) {
	var settings Settings
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return settings, nil
	}
	if err != nil {
		return settings, fmt.Errorf("inspect settings: %w", err)
	}
	if !info.Mode().IsRegular() {
		return settings, errors.New("settings must be a regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return settings, err
	}
	defer file.Close()
	decoder := json.NewDecoder(io.LimitReader(file, 65537))
	if err := decoder.Decode(&settings); err != nil {
		return Settings{}, fmt.Errorf("read settings: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return Settings{}, errors.New("settings contains trailing or oversized data")
	}
	return settings, nil
}

func Save(path string, settings Settings) error {
	if err := privatefs.EnsureDir(filepath.Dir(path)); err != nil {
		return err
	}
	if info, err := os.Lstat(path); err == nil && !info.Mode().IsRegular() {
		return errors.New("settings must be a regular file")
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	data, err := json.MarshalIndent(settings, "", "  ")
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

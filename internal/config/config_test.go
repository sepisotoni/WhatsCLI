package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSettingsRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "settings.json")
	settings, err := Load(path)
	if err != nil || settings.Accepted() {
		t.Fatal("missing settings must not grant consent", err)
	}
	if err := Save(path, Settings{ConsentVersion: ConsentVersion, Mouse: true}); err != nil {
		t.Fatal(err)
	}
	settings, err = Load(path)
	if err != nil || !settings.Accepted() || !settings.Mouse {
		t.Fatal("settings did not persist", err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatal("settings not private", err)
	}
}

func TestSettingsRejectSymlinkAndTrailingJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	target := filepath.Join(dir, "target")
	if err := os.WriteFile(target, []byte(`{"consent_version":1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("read symlink settings")
	}
	if err := Save(path, Settings{}); err == nil {
		t.Fatal("overwrote symlink settings")
	}
	if err := os.WriteFile(target, []byte(`{} {}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(target); err == nil {
		t.Fatal("accepted trailing data")
	}
}

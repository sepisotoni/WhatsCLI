package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
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

func TestOldSettingsKeepNewDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{"consent_version":1,"mouse":false}`), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := Load(path)
	if err != nil || !s.Accepted() || !s.Mouse || !s.MouseConfigured || !s.Photos() || !s.Wallpaper() || s.ReducedMotion || s.Bell {
		t.Fatalf("unexpected defaults: %+v %v", s, err)
	}
	s.Mouse = false
	if err := Save(path, s); err != nil {
		t.Fatal(err)
	}
	s, err = Load(path)
	if err != nil || s.Mouse {
		t.Fatalf("explicit mouse opt-out did not persist: %+v %v", s, err)
	}
}

func TestUIStateRoundTripAndRejectsSymlink(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ui-state.json")
	state := UIState{Read: map[string]time.Time{"a|b": time.Unix(100, 0).UTC()}, Archived: map[string]bool{"a|c": true}}
	if err := SaveState(path, state); err != nil {
		t.Fatal(err)
	}
	got, err := LoadState(path)
	if err != nil || !got.Read["a|b"].Equal(state.Read["a|b"]) || !got.Archived["a|c"] {
		t.Fatalf("round trip failed: %+v %v", got, err)
	}
	if info, _ := os.Stat(path); info.Mode().Perm() != 0o600 {
		t.Fatal("state must be private")
	}
	if missing, err := LoadState(filepath.Join(dir, "none.json")); err != nil || len(missing.Read) != 0 {
		t.Fatal("missing state should be empty")
	}
	link := filepath.Join(dir, "link.json")
	os.Symlink(path, link)
	if _, err := LoadState(link); err == nil {
		t.Fatal("followed a symlink")
	}
}

package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOpenBrowserInvokesDesktopOpener(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "opened-url")
	opener := filepath.Join(dir, "xdg-open")
	script := "#!/bin/sh\nprintf '%s' \"$1\" > \"$OPEN_MARKER\"\n"
	if err := os.WriteFile(opener, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	t.Setenv("OPEN_MARKER", marker)
	url := "http://127.0.0.1:1234/secret-path/"
	if err := openBrowser(url); err != nil {
		t.Fatal(err)
	}
	opened, err := os.ReadFile(marker)
	if err != nil || string(opened) != url {
		t.Fatalf("browser opener received %q, err=%v", opened, err)
	}
}

package privatefs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPrivateFilesAndDirectories(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "new", "nested", "state.db")
	if err := EnsureFile(path); err != nil {
		t.Fatal(err)
	}
	for path, mode := range map[string]os.FileMode{root: 0o755, filepath.Dir(path): 0o700, path: 0o600} {
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != mode {
			t.Fatalf("mode %s: %v %v", path, info, err)
		}
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := EnsureFile(path); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("file not secured: %v", info.Mode())
	}
	if err := EnsureDir(root); err != nil {
		t.Fatal(err)
	}
	info, _ = os.Stat(root)
	if info.Mode().Perm() != 0o700 {
		t.Fatalf("directory not secured: %v", info.Mode())
	}
}

func TestRejectSymlinksHardlinksAndNonregularFiles(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target")
	if err := os.WriteFile(target, []byte("private"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if err := EnsureFile(link); err == nil {
		t.Fatal("accepted file symlink")
	}
	dirlink := filepath.Join(root, "dirlink")
	if err := os.Symlink(root, dirlink); err != nil {
		t.Fatal(err)
	}
	if err := EnsureFile(filepath.Join(dirlink, "db")); err == nil {
		t.Fatal("accepted parent symlink")
	}
	if err := EnsureDir(dirlink); err == nil {
		t.Fatal("accepted directory symlink")
	}
	if err := EnsureFile(root); err == nil {
		t.Fatal("accepted directory as file")
	}
	hard := filepath.Join(root, "hard")
	if err := os.Link(target, hard); err != nil {
		t.Fatal(err)
	}
	if err := EnsureFile(hard); err == nil {
		t.Fatal("accepted hard link")
	}
	info, _ := os.Stat(target)
	if info.Mode().Perm() != 0o644 {
		t.Fatal("modified linked target permissions")
	}
}

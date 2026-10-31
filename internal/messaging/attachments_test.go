//go:build unix

package messaging

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestInspectHostLocalAttachment(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a document with spaces.txt")
	if err := os.WriteFile(path, []byte("hello"), 0600); err != nil {
		t.Fatal(err)
	}
	attachment, err := InspectAttachment(path)
	if err != nil {
		t.Fatal(err)
	}
	if attachment.Name != "a document with spaces.txt" || attachment.Size != 5 {
		t.Fatalf("incorrect preview: %+v", attachment)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []string{dir, link, filepath.Join(dir, "missing")} {
		if _, err := InspectAttachment(invalid); err == nil {
			t.Fatalf("accepted invalid attachment %q", invalid)
		}
	}
	large := filepath.Join(dir, "large")
	file, err := os.Create(large)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(MaxAttachmentSize + 1); err != nil {
		t.Fatal(err)
	}
	file.Close()
	if _, err := InspectAttachment(large); err == nil {
		t.Fatal("accepted oversized attachment")
	}
}

func TestInspectFIFOIsNonblocking(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pipe")
	if err := unix.Mkfifo(path, 0600); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { _, err := InspectAttachment(path); done <- err }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("accepted FIFO")
		}
	case <-time.After(time.Second):
		t.Fatal("FIFO inspection blocked")
	}
	// The low-level open must also be nonblocking if a regular file was replaced
	// by a FIFO between inspection and opening. fstat rejects it in the caller.
	opened := make(chan struct{})
	go func() {
		file, err := openRegularFile(path)
		if err == nil {
			file.Close()
		}
		close(opened)
	}()
	select {
	case <-opened:
	case <-time.After(time.Second):
		t.Fatal("raced FIFO open blocked")
	}
}

func TestSafeOpenRejectsSymlink(t *testing.T) {
	path := filepath.Join(t.TempDir(), "source")
	if err := os.WriteFile(path, []byte("private"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(path, path+"-link"); err != nil {
		t.Fatal(err)
	}
	file, err := openRegularFile(path + "-link")
	if file != nil {
		file.Close()
	}
	if err == nil {
		t.Fatal("safe open followed a replaced symlink")
	}
}

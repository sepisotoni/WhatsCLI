package messaging

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestZipFolderIncludesRegularFilesAndSkipsSymlinks(t *testing.T) {
	root := filepath.Join(t.TempDir(), "my folder")
	os.MkdirAll(filepath.Join(root, "sub", "deep"), 0o700)
	os.WriteFile(filepath.Join(root, "a.txt"), []byte("alpha"), 0o600)
	os.WriteFile(filepath.Join(root, "sub", "deep", "b.bin"), []byte{1, 2, 3, 4}, 0o600)
	outside := filepath.Join(t.TempDir(), "secret.txt")
	os.WriteFile(outside, []byte("must not be uploaded"), 0o600)
	os.Symlink(outside, filepath.Join(root, "link.txt"))
	os.Symlink(filepath.Dir(outside), filepath.Join(root, "dirlink"))

	info, err := InspectFolder(root)
	if err != nil || info.Files != 2 || info.Skipped != 2 {
		t.Fatalf("inspect: %+v %v", info, err)
	}
	dst := t.TempDir()
	archive, info, err := ZipFolder(root, dst)
	if err != nil || filepath.Base(archive) != "my folder.zip" || info.Files != 2 {
		t.Fatalf("zip: %q %+v %v", archive, info, err)
	}
	zr, err := zip.OpenReader(archive)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	var names []string
	for _, f := range zr.File {
		names = append(names, f.Name)
		if strings.Contains(f.Name, "..") || strings.HasPrefix(f.Name, "/") {
			t.Fatalf("unsafe entry %q", f.Name)
		}
	}
	if strings.Join(names, ",") != "my folder/a.txt,my folder/sub/deep/b.bin" {
		t.Fatalf("entries: %v", names)
	}
	if att, err := InspectAttachment(archive); err != nil || att.Name != "my folder.zip" {
		t.Fatal("archive must be a valid attachment", err)
	}
}

func TestZipFolderRejectsSymlinkRootAndFiles(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "real")
	os.MkdirAll(real, 0o700)
	os.WriteFile(filepath.Join(real, "f"), []byte("x"), 0o600)
	link := filepath.Join(dir, "link")
	os.Symlink(real, link)
	if _, err := InspectFolder(link); err == nil {
		t.Fatal("followed a symlinked root")
	}
	if _, err := InspectFolder(filepath.Join(real, "f")); err == nil {
		t.Fatal("accepted a file as a folder")
	}
	if _, _, err := ZipFolder(link, t.TempDir()); err == nil {
		t.Fatal("zipped a symlinked root")
	}
}

func TestContentTypePrefersExtension(t *testing.T) {
	if got := contentType("report.pdf", []byte("plain text")); got != "application/pdf" {
		t.Fatal(got)
	}
	if got := contentType("noextension", []byte("hello")); !strings.HasPrefix(got, "text/plain") {
		t.Fatal(got)
	}
}

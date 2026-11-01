package messaging

import (
	"archive/zip"
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// FolderInfo summarizes what a folder upload would contain.
type FolderInfo struct {
	Files   int
	Bytes   int64
	Skipped int // symlinks, devices and unreadable entries are never uploaded
}

const (
	maxFolderFiles = 20000
	maxFolderBytes = 512 << 20 // uncompressed ceiling before compression
)

// walkFolder visits regular files without following symlinks.
func walkFolder(root string, visit func(full, rel string, info fs.FileInfo) error) (FolderInfo, error) {
	var out FolderInfo
	rootInfo, err := os.Lstat(root)
	if err != nil || !rootInfo.IsDir() {
		return out, errors.New("folder cannot be read (symlinks are not followed)")
	}
	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			out.Skipped++
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if p == root || d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil || !info.Mode().IsRegular() {
			out.Skipped++
			return nil
		}
		out.Files++
		out.Bytes += info.Size()
		if out.Files > maxFolderFiles || out.Bytes > maxFolderBytes {
			return errors.New("folder is too large to upload as one archive")
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		if visit != nil {
			return visit(p, filepath.ToSlash(rel), info)
		}
		return nil
	})
	return out, err
}

// InspectFolder counts the files a folder upload would archive.
func InspectFolder(root string) (FolderInfo, error) { return walkFolder(root, nil) }

// ZipFolder archives a folder into dstDir and returns the archive path. WhatsApp
// has no folder message, so folders travel as a single .zip document.
func ZipFolder(root, dstDir string) (string, FolderInfo, error) {
	name := filepath.Base(filepath.Clean(root))
	if name == "." || name == string(filepath.Separator) || name == "" {
		name = "folder"
	}
	archive := filepath.Join(dstDir, name+".zip")
	out, err := os.OpenFile(archive, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", FolderInfo{}, errors.New("could not create archive")
	}
	zw := zip.NewWriter(out)
	info, err := walkFolder(root, func(full, rel string, fi fs.FileInfo) error {
		src, err := openRegularFile(full)
		if err != nil {
			return nil // vanished or became unsafe; skip rather than follow it
		}
		defer src.Close()
		current, err := src.Stat()
		if err != nil || !os.SameFile(fi, current) {
			return nil
		}
		header, err := zip.FileInfoHeader(fi)
		if err != nil {
			return err
		}
		header.Name = path.Join(name, rel)
		header.Method = zip.Deflate
		w, err := zw.CreateHeader(header)
		if err != nil {
			return err
		}
		_, err = io.Copy(w, io.LimitReader(src, fi.Size()))
		return err
	})
	if cerr := zw.Close(); err == nil {
		err = cerr
	}
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		if st, serr := os.Stat(archive); serr != nil || st.Size() > MaxAttachmentSize {
			err = errors.New("folder archive exceeds the 64 MiB attachment limit")
		}
	}
	if err != nil {
		os.Remove(archive)
		return "", info, err
	}
	if strings.TrimSpace(name) == "" {
		return "", info, errors.New("invalid folder name")
	}
	return archive, info, nil
}

// Package privatefs creates private state files without following symlinks.
package privatefs

import (
	"errors"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

// EnsureDir creates missing directories with mode 0700 and secures the target
// directory. Existing ancestors are checked for symlinks but never chmodded.
func EnsureDir(path string) error {
	fd, err := openDir(path)
	if err != nil {
		return err
	}
	defer unix.Close(fd)
	writable, err := unix.Openat(fd, ".", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return errors.New("cannot open private directory")
	}
	defer unix.Close(writable)
	if err := unix.Fchmod(writable, 0o700); err != nil {
		return errors.New("cannot secure private directory")
	}
	return nil
}

// EnsureFile creates or secures a regular file with mode 0600. Missing parents
// are private; permissions on existing parent directories remain unchanged.
// Callers must use a trusted private directory: reopening by path later cannot
// prevent another process with write access to that directory replacing it.
func EnsureFile(path string) error {
	absolute, err := filepath.Abs(path)
	if err != nil || path == "" {
		return errors.New("invalid private file path")
	}
	parent, err := openDir(filepath.Dir(absolute))
	if err != nil {
		return err
	}
	defer unix.Close(parent)
	fd, err := unix.Openat(parent, filepath.Base(absolute), unix.O_RDWR|unix.O_CREAT|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0o600)
	if err != nil {
		return errors.New("cannot open private file (symlinks are not allowed)")
	}
	defer unix.Close(fd)
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil || stat.Mode&unix.S_IFMT != unix.S_IFREG || stat.Nlink != 1 {
		return errors.New("private state must be a regular file with one link")
	}
	if err := unix.Fchmod(fd, 0o600); err != nil {
		return errors.New("cannot secure private file")
	}
	return nil
}

func openDir(path string) (int, error) {
	absolute, err := filepath.Abs(path)
	if err != nil || path == "" {
		return -1, errors.New("invalid private directory path")
	}
	fd, err := unix.Open("/", unix.O_PATH|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return -1, errors.Join(errors.New("cannot open filesystem root"), err)
	}
	for _, part := range strings.Split(strings.TrimPrefix(absolute, "/"), "/") {
		if part == "" {
			continue
		}
		next, err := unix.Openat(fd, part, unix.O_PATH|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		if errors.Is(err, unix.ENOENT) {
			if err = unix.Mkdirat(fd, part, 0o700); err == nil || errors.Is(err, unix.EEXIST) {
				next, err = unix.Openat(fd, part, unix.O_PATH|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
			}
		}
		unix.Close(fd)
		if err != nil {
			return -1, errors.New("cannot open private directory (symlinks are not allowed)")
		}
		fd = next
	}
	return fd, nil
}

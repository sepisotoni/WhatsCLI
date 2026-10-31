//go:build unix

package messaging

import (
	"os"

	"golang.org/x/sys/unix"
)

func openRegularFile(path string) (*os.File, error) {
	// NONBLOCK prevents a replaced FIFO from hanging; NOFOLLOW prevents a
	// replaced symlink from reading a different file between Lstat and open.
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_NONBLOCK|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(fd), path), nil
}

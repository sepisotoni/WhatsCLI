//go:build !unix

package messaging

import (
	"errors"
	"os"
)

func openRegularFile(path string) (*os.File, error) {
	return nil, errors.New("safe attachment opening is not supported on this operating system")
}

package app

import (
	"context"
	"errors"
	"io"
	"os/exec"
	"time"
)

func openBrowser(url string) error {
	for _, opener := range [][]string{{"xdg-open"}, {"gio", "open"}} {
		path, err := exec.LookPath(opener[0])
		if err != nil {
			continue
		}
		args := append(append([]string(nil), opener[1:]...), url)
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, path, args...)
		cmd.Stdout, cmd.Stderr, cmd.Stdin = io.Discard, io.Discard, nil
		return cmd.Run()
	}
	return errors.New("no browser opener (xdg-open or gio) is available")
}

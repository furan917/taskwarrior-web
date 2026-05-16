package tw

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"
)

// Sync runs `task sync` and returns the combined stdout+stderr output so the
// caller always gets a human-readable message regardless of which stream
// taskwarrior chose to write to. A non-zero exit is returned as an error.
func (c *Client) Sync(ctx context.Context) (string, error) {
	timeout := c.timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	bin := c.binary
	if bin == "" {
		bin = "task"
	}

	cmd := exec.CommandContext(ctx, bin, "sync")
	pr, pw := io.Pipe()
	cmd.Stdout = pw
	cmd.Stderr = pw

	if err := cmd.Start(); err != nil {
		pw.Close()
		return "", fmt.Errorf("start task sync: %w", err)
	}

	var waitErr error
	done := make(chan struct{})
	go func() {
		waitErr = cmd.Wait()
		pw.Close()
		close(done)
	}()

	out, _ := io.ReadAll(io.LimitReader(pr, maxOutputBytes))
	pr.Close() // unblock any further writes once we have enough output
	<-done
	return strings.TrimSpace(string(out)), waitErr
}

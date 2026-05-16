package tw

import (
	"context"
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
	out, err := exec.CommandContext(ctx, bin, "sync").CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

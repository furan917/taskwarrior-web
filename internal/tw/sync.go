package tw

import (
	"context"
	"strings"
)

// Sync runs `task sync` and returns the combined output. A non-zero exit
// is returned as an error; the output is still populated with whatever
// taskwarrior printed before failing.
func (c *Client) Sync(ctx context.Context) (string, error) {
	out, err := c.runRaw(ctx, []string{"sync"})
	return strings.TrimSpace(string(out)), err
}

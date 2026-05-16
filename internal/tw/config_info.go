package tw

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ConfigInfo holds non-sensitive Taskwarrior configuration values for the
// read-only /config page. Sensitive keys (sync.encryption_secret,
// sync.local_secret) are intentionally omitted and never fetched.
type ConfigInfo struct {
	Version       string   // `task --version`
	DataLocation  string   // rc.data.location (tilde-expanded)
	SyncServerURL string   // rc.sync.server.url; empty = not configured
	SyncClientID  string   // rc.sync.server.client_id; empty = not configured
	DateFormat    string   // rc.dateformat; empty = Taskwarrior default
	JournalTime   bool     // rc.journal.time == "yes"
	Recurrence    string   // rc.recurrence; empty = Taskwarrior default ("on")
	HookFiles     []string // filenames under <data.location>/hooks/; nil = none
	UDAs          []UDA
}

// GetConfigInfo fetches all non-sensitive configuration values. Sub-calls
// that fail (missing key, binary unavailable) are silently folded to zero
// values so a partially-configured install still renders a useful page.
func (c *Client) GetConfigInfo(ctx context.Context) ConfigInfo {
	var info ConfigInfo

	if out, err := c.runRaw(ctx, []string{"--version"}); err == nil {
		info.Version = strings.TrimSpace(string(out))
	}

	raw := c.getRcKey(ctx, "rc.data.location")
	info.DataLocation = expandTilde(raw)
	info.SyncServerURL = c.getRcKey(ctx, "rc.sync.server.url")
	info.SyncClientID = c.getRcKey(ctx, "rc.sync.server.client_id")
	info.DateFormat = c.getRcKey(ctx, "rc.dateformat")
	info.JournalTime = strings.EqualFold(c.getRcKey(ctx, "rc.journal.time"), "yes")
	info.Recurrence = c.getRcKey(ctx, "rc.recurrence")
	info.UDAs = c.UDAsCached(ctx)

	if info.DataLocation != "" {
		info.HookFiles = listHooks(filepath.Join(info.DataLocation, "hooks"))
	}

	return info
}

// expandTilde replaces a leading "~" in p with the user's home directory.
func expandTilde(p string) string {
	if !strings.HasPrefix(p, "~") {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return p
	}
	return filepath.Join(home, p[1:])
}

// listHooks returns the filenames (not full paths) of all files directly inside
// dir. Returns nil when dir does not exist or cannot be read.
func listHooks(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names
}

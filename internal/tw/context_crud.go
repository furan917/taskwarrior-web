package tw

import (
	"context"
	"fmt"
)

// DefineContext creates or overwrites a named context with the given read
// filter. Taskwarrior's `task context define` is idempotent - calling it on
// an existing name replaces the filter in place.
func (c *Client) DefineContext(ctx context.Context, name, readFilter string) error {
	if !ContextNamePattern.MatchString(name) {
		return fmt.Errorf("%w: context name %q", ErrInvalid, name)
	}
	if FilterContainsRcOverride(readFilter) {
		return fmt.Errorf("%w: read filter contains rc.* override", ErrInvalid)
	}
	if err := c.Run(ctx, "context", "define", name, readFilter); err != nil {
		return err
	}
	c.contexts.invalidate()
	return nil
}

// SetContextWriteFilter sets the write filter for an existing context via
// `task config context.<name>.write`. An empty writeFilter clears the key.
func (c *Client) SetContextWriteFilter(ctx context.Context, name, writeFilter string) error {
	if !ContextNamePattern.MatchString(name) {
		return fmt.Errorf("%w: context name %q", ErrInvalid, name)
	}
	if FilterContainsRcOverride(writeFilter) {
		return fmt.Errorf("%w: write filter contains rc.* override", ErrInvalid)
	}
	if err := c.Run(ctx, "config", "context."+name+".write", writeFilter); err != nil {
		return err
	}
	c.contexts.invalidate()
	return nil
}

// DeleteContext removes a named context via `task context delete`.
func (c *Client) DeleteContext(ctx context.Context, name string) error {
	if !ContextNamePattern.MatchString(name) {
		return fmt.Errorf("%w: context name %q", ErrInvalid, name)
	}
	if err := c.Run(ctx, "context", "delete", name); err != nil {
		return err
	}
	c.contexts.invalidate()
	return nil
}

// RenameContext renames a context by defining a new one and deleting the old.
// If oldName == newName it skips the delete step (pure filter update).
func (c *Client) RenameContext(ctx context.Context, oldName, newName, readFilter, writeFilter string) error {
	if err := c.DefineContext(ctx, newName, readFilter); err != nil {
		return err
	}
	if writeFilter != "" {
		if err := c.SetContextWriteFilter(ctx, newName, writeFilter); err != nil {
			return err
		}
	}
	if oldName != newName {
		if err := c.DeleteContext(ctx, oldName); err != nil {
			return err
		}
	}
	return nil
}

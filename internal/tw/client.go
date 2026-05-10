package tw

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	defaultTimeout = 10 * time.Second
	maxOutputBytes = 64 << 20 // 64 MiB cap on stdout per invocation
	cacheTTL       = 60 * time.Second

	// stderrTailBytes caps how much stderr we buffer per invocation. Used
	// by writeIfTaskParseError to classify Taskwarrior parser errors as
	// 400s instead of 500s. Bounded so a flood of stderr can't blow memory.
	stderrTailBytes = 4 << 10 // 4 KiB
)

// safetyArgs are prepended to every invocation. confirmation=no neutralises
// interactive prompts; json.array=on guarantees `task export` produces a
// well-formed JSON array even for empty results.
var safetyArgs = []string{
	"rc.confirmation=no",
	"rc.recurrence.confirmation=no",
	"rc.json.array=on",
}

type Client struct {
	binary  string        // empty -> "task" via $PATH
	timeout time.Duration // zero -> defaultTimeout

	// Discovery caches: each is a TTL-invalidated ttlCache so a transient
	// `task` failure during the first call doesn't poison the cache to
	// empty for the process lifetime, and so newly-defined projects /
	// tags / UDAs / contexts surface within cacheTTL of being added via
	// the CLI without a server restart. Replaces the v0-v5 sync.Once
	// pattern, which had both problems.
	udas     ttlCache[[]UDA]
	projects ttlCache[[]string]
	tags     ttlCache[[]string]
	contexts ttlCache[[]Context]
	reports  ttlCache[[]string]

	// filterCache memoises `task _get rc.report.<name>.filter` per name for
	// the Client's lifetime. Each entry is a *filterEntry whose sync.Once
	// gates the underlying shell call; concurrent callers for the same
	// report name share a single fetch. Per-report filters are stored in
	// ~/.taskrc and don't change on a normal day, so a once-cache is
	// adequate here (the discovery caches above use TTL because they're
	// derived from task data which DOES change).
	filterCache sync.Map // string -> *filterEntry
}

type filterEntry struct {
	once  sync.Once
	value string
}

// ttlCache is a lazy single-value TTL cache. On miss it calls fetch under
// lock; on fetch error it keeps the prior value if any (don't poison: a
// transient binary failure shouldn't blank the dropdown for the rest of
// the process lifetime). Generic over T so the four discovery lists share
// one implementation.
type ttlCache[T any] struct {
	mu      sync.Mutex
	value   T
	expiry  time.Time
	fetched bool
}

func (c *ttlCache[T]) invalidate() {
	c.mu.Lock()
	c.expiry = time.Time{} // zero is always Before any real time, forcing a re-fetch
	c.mu.Unlock()
}

func (c *ttlCache[T]) load(ctx context.Context, fetch func(context.Context) (T, error)) T {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.fetched && time.Now().Before(c.expiry) {
		return c.value
	}
	v, err := fetch(ctx)
	if err != nil {
		// Don't poison: keep the prior value if we have one. On true
		// first-call failure (fetched == false), c.value is the zero
		// value of T, which matches what a fresh sync.Once + nil-error
		// path would have produced.
		return c.value
	}
	c.value = v
	c.expiry = time.Now().Add(cacheTTL)
	c.fetched = true
	return c.value
}

// ClientOption configures a Client at construction time. Functional-options
// pattern keeps NewClient() backwards compatible while letting tests inject
// a fake binary path or a tighter timeout without setting unexported
// fields directly.
type ClientOption func(*Client)

// WithTimeout overrides the per-invocation timeout. Zero or negative falls
// back to defaultTimeout.
func WithTimeout(d time.Duration) ClientOption {
	return func(c *Client) {
		if d > 0 {
			c.timeout = d
		}
	}
}

// WithBinary overrides the `task` binary path. Empty string falls back to
// $PATH lookup. Used in tests to point at a fake script instead.
func WithBinary(path string) ClientOption {
	return func(c *Client) { c.binary = path }
}

func NewClient(opts ...ClientOption) *Client {
	c := &Client{}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// argv builds the argv slice for a Taskwarrior invocation by concatenating
// safetyArgs with the caller's args. Single helper instead of nine
// `append(append([]string{}, safetyArgs...), ...)` sites scattered through
// this file.
func (c *Client) argv(args ...string) []string {
	full := make([]string, 0, len(safetyArgs)+len(args))
	full = append(full, safetyArgs...)
	full = append(full, args...)
	return full
}

// ErrUnsafeArg is a defence-in-depth signal: handlers should already have
// validated input, but this catches accidental rc.* propagation.
var ErrUnsafeArg = errors.New("unsafe argument")

func guardArgs(args []string) error {
	for _, a := range args {
		if strings.HasPrefix(a, "rc.") {
			return fmt.Errorf("%w: rc.* override not permitted from caller", ErrUnsafeArg)
		}
	}
	return nil
}

// Export runs `task <args> export` and decodes the JSON array into []Task.
// args are filter/report expressions (e.g. "limit:3", "+READY", "agenda").
func (c *Client) Export(ctx context.Context, args ...string) ([]Task, error) {
	if err := guardArgs(args); err != nil {
		return nil, err
	}
	full := c.argv(args...)
	full = append(full, "export")

	out, err := c.runRaw(ctx, full)
	if err != nil {
		return nil, err
	}
	out = bytes.TrimSpace(out)
	if len(out) == 0 {
		return nil, nil
	}
	var tasks []Task
	if err := json.Unmarshal(out, &tasks); err != nil {
		return nil, fmt.Errorf("decode task export: %w", err)
	}
	return tasks, nil
}

// Run executes `task <args>` for non-export operations (add, modify, done,
// delete). Use AddArgs to construct args safely from user input.
func (c *Client) Run(ctx context.Context, args ...string) error {
	if err := guardArgs(args); err != nil {
		return err
	}
	full := c.argv(args...)
	_, err := c.runRaw(ctx, full)
	return err
}

// Annotate appends a note to the task. Description is treated as literal
// positional text via the `--` separator so embedded DOM tokens (+tag,
// due:tomorrow, rc.foo=bar) are stored verbatim and never interpreted as
// attribute modifiers.
//
// Empty (whitespace-only) text returns ErrInvalid; text longer than
// MaxDescriptionBytes is rejected to keep argv well under any platform's
// ARG_MAX.
func (c *Client) Annotate(ctx context.Context, id, text string) error {
	if !IDPattern.MatchString(id) {
		return fmt.Errorf("%w: id %q", ErrInvalid, id)
	}
	if strings.TrimSpace(text) == "" {
		return fmt.Errorf("%w: annotation text is required", ErrInvalid)
	}
	if len(text) > MaxDescriptionBytes {
		return fmt.Errorf("%w: annotation text too long (max %d bytes)", ErrInvalid, MaxDescriptionBytes)
	}
	return c.Run(ctx, id, "annotate", "--", text)
}

// Denotate removes the annotation whose description matches text. Taskwarrior
// uses substring match; we pass the full description verbatim via `--` so
// punctuation in the note (+tag, due:..., shell metachars) is treated as
// positional text.
func (c *Client) Denotate(ctx context.Context, id, text string) error {
	if !IDPattern.MatchString(id) {
		return fmt.Errorf("%w: id %q", ErrInvalid, id)
	}
	if strings.TrimSpace(text) == "" {
		return fmt.Errorf("%w: annotation text is required", ErrInvalid)
	}
	return c.Run(ctx, id, "denotate", "--", text)
}

// Start marks a task as actively being worked on (Taskwarrior records the
// timestamp on the task's `start` field, which then drives the +ACTIVE
// virtual tag). Idempotent in Taskwarrior - re-starting an already-active
// task is a no-op.
func (c *Client) Start(ctx context.Context, id string) error {
	if !IDPattern.MatchString(id) {
		return fmt.Errorf("%w: id %q", ErrInvalid, id)
	}
	return c.Run(ctx, id, "start")
}

// Stop clears the `start` timestamp set by Start. Idempotent: stopping an
// already-stopped task is a no-op in Taskwarrior.
func (c *Client) Stop(ctx context.Context, id string) error {
	if !IDPattern.MatchString(id) {
		return fmt.Errorf("%w: id %q", ErrInvalid, id)
	}
	return c.Run(ctx, id, "stop")
}

// Duplicate is `task <id> duplicate` - clones the task's editable fields
// (description, project, tags, due/wait/scheduled, UDAs) into a new pending
// task. Recurrence is NOT carried across; that's a Taskwarrior policy
// decision and exactly what we want for "create similar task".
func (c *Client) Duplicate(ctx context.Context, id string) error {
	if !IDPattern.MatchString(id) {
		return fmt.Errorf("%w: id %q", ErrInvalid, id)
	}
	return c.Run(ctx, id, "duplicate")
}

// ResolveReportFilter returns the filter expression configured for the named
// report in the user's .taskrc (e.g. "report.agenda.filter"). Empty string
// (no error) if the report has no filter defined.
//
// This calls `task _get rc.report.<name>.filter`. The rc.* arg here is
// trusted (constructed from a hardcoded report name, not user input), so it
// bypasses guardArgs by going through runRaw directly.
func (c *Client) ResolveReportFilter(ctx context.Context, name string) (string, error) {
	// Belt-and-braces validation: only allow simple report names.
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '_' || r == '-':
		default:
			return "", fmt.Errorf("%w: invalid report name %q", ErrUnsafeArg, name)
		}
	}
	if name == "" {
		return "", fmt.Errorf("%w: empty report name", ErrUnsafeArg)
	}

	args := c.argv("_get", "rc.report."+name+".filter")
	out, err := c.runRaw(ctx, args)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// ReportFilterCached is the memoised counterpart to ResolveReportFilter:
// the per-name lookup is shelled at most once per Client lifetime, after
// which the cached string is returned to every caller. On error (any kind:
// invalid name, exec failure, timeout) the empty string is cached, so the
// handler-side fallback path runs once and stays stable. Logging the
// underlying error is the caller's responsibility - this method intentionally
// surfaces nothing about how the value was obtained.
func (c *Client) ReportFilterCached(ctx context.Context, name string) string {
	v, _ := c.filterCache.LoadOrStore(name, &filterEntry{})
	entry := v.(*filterEntry)
	entry.once.Do(func() {
		got, _ := c.ResolveReportFilter(ctx, name)
		entry.value = got
	})
	return entry.value
}

// ListUDAs returns the User-Defined Attributes currently declared in the
// user's ~/.taskrc, with their declared type and label. Empty result with nil
// error means no UDAs are defined.
//
// Implementation: shells `task _udas` to enumerate names, then per-name calls
// `task _get rc.uda.<name>.type` and `task _get rc.uda.<name>.label`. Names
// failing UDANamePattern are dropped silently as a defence-in-depth filter
// against a hostile taskrc smuggling parser tokens through the cache.
//
// Both _udas and _get rc.uda.* arguments are constructed entirely from
// constants and validated names (no user input), so they go via runRaw and
// bypass guardArgs (which would otherwise reject the rc.uda.* prefix).
func (c *Client) ListUDAs(ctx context.Context) ([]UDA, error) {
	listArgs := c.argv("_udas")
	out, err := c.runRaw(ctx, listArgs)
	if err != nil {
		return nil, fmt.Errorf("list udas: %w", err)
	}

	var udas []UDA
	for line := range strings.SplitSeq(string(out), "\n") {
		name := strings.TrimSpace(line)
		if name == "" {
			continue
		}
		if !UDANamePattern.MatchString(name) {
			continue
		}

		typ := c.getRcKey(ctx, "rc.uda."+name+".type")
		label := c.getRcKey(ctx, "rc.uda."+name+".label")
		values := parseUDAValues(c.getRcKey(ctx, "rc.uda."+name+".values"))
		udas = append(udas, UDA{Name: name, Type: typ, Label: label, Values: values})
	}
	return udas, nil
}

// parseUDAValues splits the comma-separated value list returned by
// `task _get rc.uda.<name>.values`, dropping the trailing empty entry that
// Taskwarrior emits to signal "empty allowed" (the form layer always offers
// a separate clear option). Empty input returns nil so the caller can
// distinguish "no enum constraint" from "enum with no values" cleanly.
func parseUDAValues(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		v := strings.TrimSpace(p)
		if v == "" {
			continue
		}
		out = append(out, v)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// JournalTimeEnabled reports whether `rc.journal.time` is set to "yes" in the
// active taskrc. With it enabled, taskwarrior appends "Started …" / "Stopped …"
// annotations on every start/stop call, giving the web UI enough data to build
// a timesheet.
func (c *Client) JournalTimeEnabled(ctx context.Context) bool {
	return strings.EqualFold(c.getRcKey(ctx, "rc.journal.time"), "yes")
}

// EnableJournalTime writes `journal.time=yes` to the active taskrc via
// `task config journal.time yes`. Idempotent — safe to call if already on.
func (c *Client) EnableJournalTime(ctx context.Context) error {
	return c.Run(ctx, "config", "journal.time", "yes")
}

// getRcKey looks up a single rc.* key with a short timeout, returning empty
// string on any error so the caller can fall back to its default. The key is
// trusted (constructed from a validated UDA name and a constant suffix) so
// this bypasses guardArgs.
func (c *Client) getRcKey(ctx context.Context, key string) string {
	args := c.argv("_get", key)
	out, err := c.runRaw(ctx, args)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// UDAsCached returns the UDA list, computing it once per Client lifetime. The
// underlying ListUDAs call is gated by sync.Once; on first-call failure the
// cached value is the empty list and subsequent calls return that empty list
// without retrying.
func (c *Client) UDAsCached(ctx context.Context) []UDA {
	return c.udas.load(ctx, c.ListUDAs)
}

// runRaw is the trusted-caller entry point that skips guardArgs. Callers
// (Export, Run, ResolveReportFilter) are responsible for either validating
// user-supplied args via guardArgs or passing only constants/internally
// constructed args.
//
// stderr is captured into a bounded scratch buffer (stderrTailBytes) and
// folded into the returned error as a *TaskExitError when the command
// exits non-zero. This lets handlers classify Taskwarrior parser errors
// (e.g. "Could not interpret the date 'x'.") as 400s instead of generic
// 500s. The captured stderr is NEVER logged - the error type carries it
// for in-process classification only, then it's discarded.
func (c *Client) runRaw(ctx context.Context, args []string) ([]byte, error) {
	timeout := c.timeout
	if timeout == 0 {
		timeout = defaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	bin := c.binary
	if bin == "" {
		bin = "task"
	}
	cmd := exec.CommandContext(ctx, bin, args...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	// stderr goes to a bounded buffer for classification only; it's never
	// logged or surfaced in plaintext - Taskwarrior may echo descriptions
	// or UUIDs on parse errors, so the buffer is held in the returned
	// error type and consumed by handlers that need to fingerprint the
	// failure shape.
	var stderrBuf bytes.Buffer
	cmd.Stderr = &stderrBuf

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start task: %w", err)
	}

	out, readErr := io.ReadAll(io.LimitReader(stdout, maxOutputBytes))

	if waitErr := cmd.Wait(); waitErr != nil {
		stderr := stderrBuf.Bytes()
		if len(stderr) > stderrTailBytes {
			stderr = stderr[len(stderr)-stderrTailBytes:]
		}
		exitCode := -1
		var exitErr *exec.ExitError
		if errors.As(waitErr, &exitErr) {
			exitCode = exitErr.ExitCode()
		}
		stdoutTail := out
		if len(stdoutTail) > stderrTailBytes {
			stdoutTail = stdoutTail[len(stdoutTail)-stderrTailBytes:]
		}
		return out, &TaskExitError{
			ExitCode: exitCode,
			Stderr:   string(stderr),
			Stdout:   string(stdoutTail),
			Wrapped:  waitErr,
		}
	}
	if readErr != nil {
		return out, fmt.Errorf("read task stdout: %w", readErr)
	}
	if int64(len(out)) >= maxOutputBytes {
		// Discard the partial buffer: callers always treat a non-nil err as
		// "no usable output", and returning it muddies that contract.
		return nil, fmt.Errorf("task output exceeded %d bytes", maxOutputBytes)
	}
	return out, nil
}

// TaskExitError is returned by runRaw when the `task` binary exits non-zero.
// It carries the captured stderr (bounded to stderrTailBytes) so handlers
// can classify the failure - e.g. surface "Could not interpret the date 'x'."
// as a per-field 400 rather than a generic 500. Callers that don't care
// can rely on the Error() string which redacts stderr to the exit code only.
type TaskExitError struct {
	ExitCode int
	Stderr   string // bounded to stderrTailBytes; never log this verbatim
	Stdout   string // bounded to stderrTailBytes; carries TW's user-facing
	// summary line ("Deleted 0 tasks.", "Modified 1 task.") so callers can
	// classify a non-zero exit as a real error vs an idempotent no-op.
	Wrapped error
}

// noOpExitPattern matches Taskwarrior's "X 0 tasks." summary lines emitted
// when an operation found no tasks to act on (e.g. trying to delete an
// already-deleted task, or modify with no changes). For idempotent
// commands (delete, done, modify) this is functionally success - the
// desired end state already holds.
var noOpExitPattern = regexp.MustCompile(`\b(Deleted|Completed|Modified|Updated|Started|Stopped) 0 tasks?\.`)

// IsNoOpExit reports whether err is a TaskExitError whose stdout indicates
// the requested operation was a no-op because the task was already in the
// target state. Use this in handlers for idempotent actions to convert a
// "task is not deletable / Deleted 0 tasks" exit-1 into a quiet success
// rather than a 500 - the user's intent (gone / done) is already met.
func IsNoOpExit(err error) bool {
	var te *TaskExitError
	if !errors.As(err, &te) {
		return false
	}
	return noOpExitPattern.MatchString(te.Stdout)
}

// IsNotInitialised reports whether err is a TaskExitError caused by a missing
// ~/.taskrc — Taskwarrior exits 2 with this message when it has never been run.
func IsNotInitialised(err error) bool {
	var te *TaskExitError
	if !errors.As(err, &te) {
		return false
	}
	return strings.Contains(te.Stderr, "Cannot proceed without rc file")
}

func (e *TaskExitError) Error() string {
	if e.ExitCode >= 0 {
		return fmt.Sprintf("task command failed: exit status %d", e.ExitCode)
	}
	return fmt.Sprintf("task command failed: %v", e.Wrapped)
}

func (e *TaskExitError) Unwrap() error { return e.Wrapped }

// IsInvalidFilter reports whether err is a TaskExitError caused by a filter
// expression that Taskwarrior's export evaluator cannot parse. This is a
// safety net for filter shapes (e.g. -+tag) that SafeReadFilter did not
// catch at composition time.
func IsInvalidFilter(err error) bool {
	var te *TaskExitError
	if !errors.As(err, &te) {
		return false
	}
	return strings.Contains(te.Stderr, "The expression could not be evaluated")
}

// Project / tag discovery (auto-suggest sources). Cached for the process
// lifetime; restart to pick up newly-defined names.
//
// projectListPattern / tagListPattern mirror ProjectPattern / TagPattern in
// task.go but live here so the discovery path doesn't accidentally accept
// names the AddArgs validator would later reject. Anything failing the
// pattern is dropped silently as defence-in-depth against a hostile sqlite
// state smuggling parser tokens through the cache.

var projectListPattern = regexp.MustCompile(`^[a-zA-Z0-9._]+$`)
var tagListPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// ReportNamePattern is the allowlist for Taskwarrior report names. Used
// both internally as the discovery filter and by the views layer to
// validate the {name} URL path param of /r/{name}. Same shape as
// ContextNamePattern and tagListPattern - letters, digits, dash,
// underscore.
var ReportNamePattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// virtualTags is the set of Taskwarrior virtual tags - computed at query time
// from task state, never user-creatable. `task _tags` emits these alongside
// real user tags so we filter them out of suggest lists where they'd offer
// the user something they can't actually set.
//
// Source: https://taskwarrior.org/docs/virtual-tags/ (Taskwarrior 3.x).
var virtualTags = map[string]struct{}{
	"ACTIVE": {}, "ANNOTATED": {}, "BLOCKED": {}, "BLOCKING": {},
	"CHILD": {}, "COMPLETED": {}, "DELETED": {}, "DUE": {},
	"DUETODAY": {}, "INSTANCE": {}, "LATEST": {}, "MONTH": {},
	"ORPHAN": {}, "OVERDUE": {}, "PARENT": {}, "PENDING": {},
	"PRIORITY": {}, "PROJECT": {}, "QUARTER": {}, "READY": {},
	"SCHEDULED": {}, "TAGGED": {}, "TEMPLATE": {}, "TODAY": {},
	"TOMORROW": {}, "UDA": {}, "UNBLOCKED": {}, "UNTIL": {},
	"WAITING": {}, "WEEK": {}, "YEAR": {}, "YESTERDAY": {},
}

// ListReports shells `task _reports` and returns the sorted, deduplicated
// list of report names matching the same shape as Taskwarrior's report
// names. Used by the views layer to dynamically surface user-defined
// reports in the nav, replacing the previous hardcoded curatedReportSpecs
// map (curated specs still drive the four pinned tabs).
//
// Names are filtered through ReportNamePattern as defence in depth so a
// hostile taskrc cannot smuggle filter-fragment characters through report
// discovery.
func (c *Client) ListReports(ctx context.Context) ([]string, error) {
	args := c.argv("_reports")
	out, err := c.runRaw(ctx, args)
	if err != nil {
		return nil, fmt.Errorf("list reports: %w", err)
	}
	return filterStringList(string(out), ReportNamePattern), nil
}

// ListProjects shells `task _projects` and returns the deduplicated, sorted
// list of project names matching projectListPattern. Empty result with nil
// error means the user has no projects yet.
//
// _projects is built into Taskwarrior and emits one project name per line on
// stdout. The argument is a constant so this goes via runRaw.
func (c *Client) ListProjects(ctx context.Context) ([]string, error) {
	args := c.argv("_projects")
	out, err := c.runRaw(ctx, args)
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	return filterStringList(string(out), projectListPattern), nil
}

// ListTags is the same shape as ListProjects but for `task _tags`. Virtual
// tags (ACTIVE, BLOCKED, OVERDUE, …) are stripped because they're computed
// from task state, not user-set, so suggesting them would mislead the user.
func (c *Client) ListTags(ctx context.Context) ([]string, error) {
	args := c.argv("_tags")
	out, err := c.runRaw(ctx, args)
	if err != nil {
		return nil, fmt.Errorf("list tags: %w", err)
	}
	all := filterStringList(string(out), tagListPattern)
	out2 := all[:0]
	for _, t := range all {
		if _, isVirtual := virtualTags[t]; isVirtual {
			continue
		}
		out2 = append(out2, t)
	}
	return out2, nil
}

// filterStringList parses newline-separated output, drops blanks/duplicates
// and any entry failing pat, and returns a sorted slice. Returned slice is
// non-nil empty when nothing survives the filter so callers can range over it
// without nil-checking.
func filterStringList(raw string, pat *regexp.Regexp) []string {
	seen := map[string]bool{}
	out := []string{}
	for line := range strings.SplitSeq(raw, "\n") {
		v := strings.TrimSpace(line)
		if v == "" || seen[v] {
			continue
		}
		if !pat.MatchString(v) {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

// ProjectsCached returns the project list, lazily fetched and TTL-invalidated
// via the shared ttlCache. Newly-defined projects surface within cacheTTL of
// being added via the CLI without a server restart, and a transient `task`
// binary failure during the first call doesn't poison the cache to empty.
func (c *Client) ProjectsCached(ctx context.Context) []string {
	return c.projects.load(ctx, c.ListProjects)
}

// TagsCached is the tag-side counterpart to ProjectsCached.
func (c *Client) TagsCached(ctx context.Context) []string {
	return c.tags.load(ctx, c.ListTags)
}

// ReportsCached is the reports-side counterpart. Used by the views layer
// to surface user-defined reports in the nav alongside the curated
// defaults (Ready / Next / Agenda / Forecast).
func (c *Client) ReportsCached(ctx context.Context) []string {
	return c.reports.load(ctx, c.ListReports)
}

// Undo wraps `task undo`. The safetyArgs prefix already contains
// rc.confirmation=no so Taskwarrior won't interactively prompt; the call
// reverses the most recent change atomically. Repeated calls walk further
// back through the undo log.
func (c *Client) Undo(ctx context.Context) error {
	return c.Run(ctx, "undo")
}

// ListContexts shells `task context list` and parses the human-readable
// table. Output shape (Taskwarrior 3.x):
//
//	Name  Type   Definition   Active
//	---- ------ ------------ --------
//	work read   +work         yes
//	work write  +work         no
//	home read   +home
//	...
//	(empty when no contexts defined; the bare `task contexts` form returns
//	"No matches." with exit 1 in 3.x and is no longer used)
//
// `rc.defaultwidth:1000` defeats Taskwarrior's column-wrap heuristic so long
// OR-chained filters land on one line per row - the parser doesn't merge
// continuation lines and would otherwise truncate them.
//
// Multiple rows for the same name (one per type) are merged into a single
// Context entry; ReadFilter / WriteFilter capture the per-type filter and
// Active reflects whichever row was marked active (Taskwarrior tracks one
// active read context and one active write context independently, but the
// UI only surfaces read).
//
// Names failing ContextNamePattern are dropped silently as defence-in-depth.
func (c *Client) ListContexts(ctx context.Context) ([]Context, error) {
	args := c.argv("rc.defaultwidth:1000", "context", "list")
	out, err := c.runRaw(ctx, args)
	if err != nil {
		return nil, fmt.Errorf("list contexts: %w", err)
	}
	return parseContextsTable(string(out)), nil
}

// parseContextsTable extracts Context entries from the raw stdout of
// `task contexts`. Lifted into a free function so it can be unit-tested
// without shelling.
func parseContextsTable(raw string) []Context {
	byName := map[string]*Context{}
	order := []string{}

	for line := range strings.SplitSeq(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Skip header / separator / footer rows. The header has the literal
		// "Name" column title; the separator is a sequence of dashes; the
		// footer matches "N contexts" or "No contexts defined.".
		lower := strings.ToLower(line)
		if strings.HasPrefix(lower, "name") && strings.Contains(lower, "type") {
			continue
		}
		if strings.HasPrefix(line, "-") {
			continue
		}
		if strings.HasSuffix(lower, "contexts defined.") {
			continue
		}
		if strings.Contains(lower, "context") && (strings.HasSuffix(lower, ")") || strings.HasSuffix(lower, "active") || strings.HasSuffix(lower, "active.")) {
			// "3 contexts (2 of which are active)" footer.
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		name := fields[0]
		if !ContextNamePattern.MatchString(name) {
			continue
		}
		typ := strings.ToLower(fields[1])
		if typ != "read" && typ != "write" {
			continue
		}

		// Active flag is the trailing field if it looks like yes/no; the
		// filter text is everything between Type and Active.
		active := false
		filterEnd := len(fields)
		if last := strings.ToLower(fields[len(fields)-1]); last == "yes" || last == "no" {
			active = last == "yes"
			filterEnd = len(fields) - 1
		}
		filter := strings.Join(fields[2:filterEnd], " ")

		entry, ok := byName[name]
		if !ok {
			entry = &Context{Name: name}
			byName[name] = entry
			order = append(order, name)
		}
		switch typ {
		case "read":
			entry.ReadFilter = filter
			if active {
				entry.Active = true
			}
		case "write":
			entry.WriteFilter = filter
		}
	}

	out := make([]Context, 0, len(order))
	for _, name := range order {
		out = append(out, *byName[name])
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// ContextsCached returns the contexts list, lazily fetched and
// TTL-invalidated via the shared ttlCache.
//
// The Active flag captured here is a snapshot of the moment of cache fill
// and MUST NOT be relied on - use ActiveContext for the live value.
func (c *Client) ContextsCached(ctx context.Context) []Context {
	return c.contexts.load(ctx, c.ListContexts)
}

// ActiveContext shells `task _get rc.context` and returns the active
// context name, or empty string when no context is active. Errors are
// folded into "no active context" to keep the UI rendering even if the
// binary is briefly flaky - the dropdown will just show "(none)" as
// active in that case.
//
// `_context` (without `_get`) is the discovery-list helper in TW 3.x and
// emits ALL defined context names regardless of which is active, so it's
// the wrong tool here. `_get rc.context` reads the single rc setting that
// tracks the live active state.
//
// NOT cached: the user can change the active context out-of-band via the
// CLI or via the POST /context route, and a stale value here would mislead
// the pill / title / empty-state. The call is cheap (~10-30 ms) and runs
// once per page render.
func (c *Client) ActiveContext(ctx context.Context) string {
	args := c.argv("_get", "rc.context")
	out, err := c.runRaw(ctx, args)
	if err != nil {
		return ""
	}
	name := strings.TrimSpace(string(out))
	if name == "" {
		return ""
	}
	if !ContextNamePattern.MatchString(name) {
		return ""
	}
	return name
}

// Dependents returns every pending task whose `depends` list includes uuid -
// i.e. the set of tasks blocked on this one. Implemented as
// `task export depends.has:<uuid> status:pending`. The argument is validated
// against IDPattern as defence-in-depth so callers cannot smuggle filter
// fragments through the path-value plumbing.
//
// Not cached: the result is per-uuid and the dataset is small (one Export per
// expand panel), so a sync.Map of *filterEntry-style memoisation would buy
// nothing meaningful and complicate invalidation when a task is modified.
func (c *Client) Dependents(ctx context.Context, uuid string) ([]Task, error) {
	if !IDPattern.MatchString(uuid) {
		return nil, fmt.Errorf("%w: uuid %q", ErrInvalid, uuid)
	}
	return c.Export(ctx, "depends.has:"+uuid, "status:pending")
}

// SetContext activates the named context, or clears it when name is empty.
// Validation of the name is the caller's responsibility; we re-check here as
// defence-in-depth so a future refactor can't bypass the handler-side check.
//
// Implementation:
//   - empty name -> `task context none`
//   - non-empty -> `task context <name>`
//
// Both forms ride safetyArgs so Taskwarrior doesn't prompt interactively.
func (c *Client) SetContext(ctx context.Context, name string) error {
	if name == "" {
		return c.Run(ctx, "context", "none")
	}
	if !ContextNamePattern.MatchString(name) {
		return fmt.Errorf("%w: context name %q", ErrInvalid, name)
	}
	return c.Run(ctx, "context", name)
}

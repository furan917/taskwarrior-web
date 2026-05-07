package tw

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Annotation is one timestamped note attached to a task. Taskwarrior emits
// these in the `annotations` array on `task export` for any task that has
// been annotated via `task <id> annotate "<text>"`.
type Annotation struct {
	Entry       string `json:"entry"` // YYYYMMDDTHHMMSSZ
	Description string `json:"description"`
}

// Task mirrors `task export` JSON for one task. Time fields are stored as raw
// Taskwarrior strings ("YYYYMMDDTHHMMSSZ") and parsed lazily via ParseTime.
//
// UDAs holds any top-level JSON keys that aren't recognised as Taskwarrior
// built-ins; these are populated from the raw export by Task.UnmarshalJSON. The
// map is name -> stringified value so the rendering layer can treat every UDA
// uniformly without knowing its declared type.
type Task struct {
	ID          int               `json:"id"`
	UUID        string            `json:"uuid"`
	Description string            `json:"description"`
	Status      string            `json:"status"`
	Entry       string            `json:"entry"`
	Modified    string            `json:"modified,omitempty"`
	Due         string            `json:"due,omitempty"`
	Wait        string            `json:"wait,omitempty"`
	Scheduled   string            `json:"scheduled,omitempty"`
	Project     string            `json:"project,omitempty"`
	Tags        []string          `json:"tags,omitempty"`
	Urgency     float64           `json:"urgency,omitempty"`
	Annotations []Annotation      `json:"annotations,omitempty"`
	Depends     []string          `json:"depends,omitempty"`
	UDAs        map[string]string `json:"-"`
}

// taskBuiltinKeys lists every JSON key that Task captures as a typed field (or
// deliberately ignores). UnmarshalJSON treats anything outside this set as a
// UDA. Taskwarrior emits a few internal fields we don't model (parent, mask,
// imask, recur, until, depends, start, end) - skip those rather than
// mis-classify them as UDAs.
//
// `priority` is intentionally NOT listed: Taskwarrior 3.x emits it as a
// top-level JSON key (mirroring the legacy built-in field) even when the
// user has redeclared it as a UDA in `~/.taskrc`. Treating it as a UDA
// here keeps the read path symmetric with the write path (which sends
// `priority:M` via readUDAArgs) so a value set via the edit form re-renders
// correctly in t.UDAs.
var taskBuiltinKeys = map[string]struct{}{
	"id":          {},
	"uuid":        {},
	"description": {},
	"status":      {},
	"entry":       {},
	"modified":    {},
	"due":         {},
	"wait":        {},
	"scheduled":   {},
	"project":     {},
	"tags":        {},
	"urgency":     {},
	"annotations": {},
	"start":       {},
	"end":         {},
	"parent":      {},
	"mask":        {},
	"imask":       {},
	"recur":       {},
	"until":       {},
	"depends":     {},
}

// UnmarshalJSON decodes the typed fields via a struct alias (so we don't
// recurse into ourselves) and then collects every other top-level key into
// UDAs. Numeric and boolean values are stored as their JSON literal text;
// strings are unquoted; objects/arrays are kept as raw JSON for visibility but
// not rendered specially.
func (t *Task) UnmarshalJSON(data []byte) error {
	type taskAlias Task
	var typed taskAlias
	if err := json.Unmarshal(data, &typed); err != nil {
		return err
	}
	*t = Task(typed)

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	for key, value := range raw {
		if _, builtin := taskBuiltinKeys[key]; builtin {
			continue
		}
		if !UDANamePattern.MatchString(key) {
			// Defence-in-depth: if a stray key has shell-meta chars or other
			// junk, drop it rather than surface it.
			continue
		}
		if t.UDAs == nil {
			t.UDAs = make(map[string]string)
		}
		t.UDAs[key] = stringifyJSON(value)
	}
	return nil
}

// MarshalJSON re-emits the typed fields plus any captured UDAs as top-level
// keys, mirroring the shape `task export` originally produced. Used only by
// tests that round-trip Task through json.Marshal -> json.Unmarshal; the
// production server only ever decodes.
func (t Task) MarshalJSON() ([]byte, error) {
	type taskAlias Task
	typed := taskAlias(t)
	typed.UDAs = nil

	base, err := json.Marshal(typed)
	if err != nil {
		return nil, err
	}
	if len(t.UDAs) == 0 {
		return base, nil
	}

	// Splice the UDA keys into the encoded object. base is "{...}" or "{}".
	out := make(map[string]json.RawMessage)
	if err := json.Unmarshal(base, &out); err != nil {
		return nil, err
	}
	for k, v := range t.UDAs {
		// Always emit as a JSON string; we lose the original numeric/bool typing
		// but tests only need value preservation, and round-trip through
		// UnmarshalJSON yields the same string back.
		enc, err := json.Marshal(v)
		if err != nil {
			return nil, err
		}
		out[k] = enc
	}
	return json.Marshal(out)
}

// stringifyJSON converts one raw JSON value into the human-readable form we
// store in Task.UDAs. The caller has already filtered out builtins, so this
// only needs to handle the four leaf cases: string (unquote), bool/number
// (literal text), null (empty), object/array (raw JSON).
func stringifyJSON(raw json.RawMessage) string {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return ""
	}
	// Strings: try to decode and surface the literal value.
	if trimmed[0] == '"' {
		var s string
		if err := json.Unmarshal(raw, &s); err == nil {
			return s
		}
	}
	// Numbers, booleans, objects, arrays: keep the raw JSON literal.
	return trimmed
}

func (t Task) IsOverdue() bool {
	if t.Due == "" {
		return false
	}
	due, err := ParseTime(t.Due)
	if err != nil {
		return false
	}
	return time.Now().After(due)
}

// AddInput is the validated form payload for `task add` / `task <id> modify`.
// Free text (Description) is preserved literally; structured fields are
// regex-allowlisted.
//
// Depends is a list of UUID strings naming tasks this one depends on. Each
// entry is validated against IDPattern and emitted as a single
// `depends:UUID,UUID` arg; ModifyArgs emits a bare `depends:` clear-arg when
// the slice is empty so the user can drop every dependency by submitting an
// empty value.
type AddInput struct {
	Description string
	Project     string
	Tags        []string
	Due         string
	Wait        string
	Scheduled   string
	Depends     []string
}

// IDPattern matches Taskwarrior task references: numeric short ID or full UUID.
var IDPattern = regexp.MustCompile(`^[0-9]+$|^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// ProjectPattern matches valid project names: letters, digits, dots, underscores.
var ProjectPattern = regexp.MustCompile(`^[a-zA-Z0-9._]+$`)

// TagPattern matches valid tag names: letters, digits, dashes, underscores.
var TagPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// datePattern is the shape filter for date inputs. It accepts ISO dates
// ("2026-05-09", "2026-05-09T14:00"), keywords ("tomorrow", "eom"),
// durations and offsets ("+2d", "-3h", "due-3d", "scheduled+1w") while
// rejecting:
//   - whitespace, shell-meta and free-form prose ("Due in 2 days") via
//     the safe charset
//   - pure-digit strings ("1", "2", "12345") that Taskwarrior interprets
//     ambiguously and usually surface as 500 from the binary - users
//     typing "2" almost always mean "+2d" or "tomorrow", not the literal
//     digit Taskwarrior would try to parse
//
// The trailing `[a-zA-Z\-][a-zA-Z0-9:_+\-]*` clause requires at least one
// letter or hyphen anywhere in the string, which is true for every legal
// Taskwarrior date form (ISO has hyphens, keywords have letters,
// durations have a unit letter).
var datePattern = regexp.MustCompile(`^[a-zA-Z0-9:_+\-]*[a-zA-Z\-][a-zA-Z0-9:_+\-]*$`)

var ErrInvalid = errors.New("invalid input")

// MaxDescriptionBytes caps the description / annotation text we'll forward
// to Taskwarrior. macOS' ARG_MAX is ~1 MiB so a multi-megabyte description
// would blow the exec call anyway; bounding here surfaces the rejection
// as a clean 400 with field-level highlighting instead of an opaque 500
// later. 4 KiB is generous for a real task description and well under
// any platform argv limit.
const MaxDescriptionBytes = 4 << 10 // 4 KiB

// ValidationError is the typed validation failure returned by AddArgs /
// ModifyArgs. Field names mirror the form input names (description,
// project, tags, due, wait, scheduled, depends) so handlers can pluck
// the field via errors.As and red-border the matching <input>. Reason
// is an optional human-readable hint; callers that surface to the user
// usually rewrite the message in form_errors.go's classifier rather
// than echoing this one.
//
// Wraps ErrInvalid so existing errors.Is(err, tw.ErrInvalid) checks keep
// working - this is purely an additive type for callers that want field
// info without parsing message text.
type ValidationError struct {
	Field  string
	Value  string
	Reason string
}

func (e *ValidationError) Error() string {
	switch {
	case e.Reason != "" && e.Value != "":
		return fmt.Sprintf("invalid input: %s %q: %s", e.Field, e.Value, e.Reason)
	case e.Reason != "":
		return fmt.Sprintf("invalid input: %s: %s", e.Field, e.Reason)
	case e.Value != "":
		return fmt.Sprintf("invalid input: %s %q", e.Field, e.Value)
	}
	return fmt.Sprintf("invalid input: %s", e.Field)
}

func (e *ValidationError) Unwrap() error { return ErrInvalid }

// AddArgs serialises an AddInput for `task add`, skipping empty optional
// fields. The description always lands as `description:"<text>"` so
// embedded DOM tokens (+tag, due:tomorrow, rc.foo=bar) are stored as
// literal text. Failures return a *ValidationError that wraps ErrInvalid.
func (in AddInput) AddArgs() ([]string, error) {
	return in.buildArgs(false)
}

// ModifyArgs is the modify-side counterpart to AddArgs: empty optional
// fields are emitted as bare `key:` clear-args so a `task <id> modify`
// can blank a previously-set value. Description must still be non-empty
// (it has no clear semantic). Used by the edit form's PUT handler.
func (in AddInput) ModifyArgs() ([]string, error) {
	return in.buildArgs(true)
}

// buildArgs is the shared serialiser for AddArgs / ModifyArgs. The clear
// flag toggles between "skip empty optional fields" (add semantics) and
// "emit `key:` clear-args" (modify semantics). Folds 90% of what was
// duplicated between the two public methods - the only divergence is
// the empty-value handling per field type.
func (in AddInput) buildArgs(clear bool) ([]string, error) {
	if strings.TrimSpace(in.Description) == "" {
		return nil, &ValidationError{Field: "description", Reason: "required"}
	}
	if len(in.Description) > MaxDescriptionBytes {
		return nil, &ValidationError{Field: "description", Reason: fmt.Sprintf("too long (max %d bytes)", MaxDescriptionBytes)}
	}
	args := []string{"description:" + QuoteArg(in.Description)}

	args, err := appendOrClear(args, "project", in.Project, ProjectPattern, clear)
	if err != nil {
		return nil, err
	}

	for _, tag := range in.Tags {
		if !TagPattern.MatchString(tag) {
			return nil, &ValidationError{Field: "tags", Value: tag}
		}
		args = append(args, "+"+tag)
	}

	for _, df := range []struct{ name, value string }{
		{"due", in.Due},
		{"wait", in.Wait},
		{"scheduled", in.Scheduled},
	} {
		args, err = appendOrClear(args, df.name, df.value, datePattern, clear)
		if err != nil {
			return nil, err
		}
	}

	// Depends is a strict subset of the optional-field shape: AddArgs only
	// emits the arg when there's a non-empty list, ModifyArgs always
	// emits (so empty list clears all dependencies). dependsArg
	// validates each UUID and joins them.
	if clear || len(in.Depends) > 0 {
		dep, err := dependsArg(in.Depends)
		if err != nil {
			return nil, err
		}
		args = append(args, dep)
	}

	return args, nil
}

// appendOrClear handles the shared "validate-and-emit OR clear" logic for
// optional pattern-validated fields (project, due, wait, scheduled). Empty
// value + clear == false -> skip the field entirely. Empty value + clear
// == true -> append the bare `name:` clear-arg. Non-empty value + pattern
// match -> append `name:"<value>"`. Non-empty value + pattern mismatch ->
// *ValidationError.
func appendOrClear(args []string, field, value string, pat *regexp.Regexp, clear bool) ([]string, error) {
	if value == "" {
		if clear {
			return append(args, field+":"), nil
		}
		return args, nil
	}
	if !pat.MatchString(value) {
		return nil, &ValidationError{Field: field, Value: value}
	}
	return append(args, field+":"+QuoteArg(value)), nil
}

// dependsArg validates each UUID in deps against IDPattern and returns the
// single `depends:UUID,UUID,...` argv fragment Taskwarrior consumes. Any
// malformed entry is rejected with *ValidationError; a zero-length list
// returns the bare clear-arg `depends:` so callers (ModifyArgs) can use it
// directly.
func dependsArg(deps []string) (string, error) {
	if len(deps) == 0 {
		return "depends:", nil
	}
	for _, d := range deps {
		if !IDPattern.MatchString(d) {
			return "", &ValidationError{Field: "depends", Value: d}
		}
	}
	return "depends:" + strings.Join(deps, ","), nil
}


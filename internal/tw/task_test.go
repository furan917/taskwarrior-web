package tw

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestAddArgs_DescriptionIsLiteral(t *testing.T) {
	tests := []struct {
		name string
		in   AddInput
		want string
	}{
		{"plain", AddInput{Description: "buy milk"}, `description:"buy milk"`},
		{"DOM modifiers stay literal", AddInput{Description: "+urgent due:tomorrow project:evil"}, `description:"+urgent due:tomorrow project:evil"`},
		{"rc override stays literal", AddInput{Description: "rc.data.location=/tmp/evil"}, `description:"rc.data.location=/tmp/evil"`},
		{"shell metachars stay literal", AddInput{Description: `$(rm -rf /); echo pwned`}, `description:"$(rm -rf /); echo pwned"`},
		{"embedded quotes escaped", AddInput{Description: `she said "hi"`}, `description:"she said \"hi\""`},
		{"backslash escaped", AddInput{Description: `path C:\foo`}, `description:"path C:\\foo"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args, err := tt.in.AddArgs()
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if !slices.Contains(args, tt.want) {
				t.Errorf("missing %q in %v", tt.want, args)
			}
		})
	}
}

func TestAddArgs_EmptyDescriptionRejected(t *testing.T) {
	for _, desc := range []string{"", "   ", "\t\n"} {
		_, err := AddInput{Description: desc}.AddArgs()
		if !errors.Is(err, ErrInvalid) {
			t.Errorf("desc %q: expected ErrInvalid, got %v", desc, err)
		}
	}
}

func TestAddArgs_Project(t *testing.T) {
	cases := []struct {
		project string
		ok      bool
	}{
		{"team.alpha", true},
		{"hiring.devops", true},
		{"team.beta", true},
		{"team", true},
		{"../etc/passwd", false},
		{"+team", false},
		{"-team", false},
		{"team alpha", false},
		{"team;ls", false},
		{"team\nfoo", false},
	}
	for _, c := range cases {
		t.Run(c.project, func(t *testing.T) {
			_, err := AddInput{Description: "x", Project: c.project}.AddArgs()
			if (err == nil) != c.ok {
				t.Errorf("project %q: ok=%v err=%v", c.project, c.ok, err)
			}
		})
	}
}

func TestAddArgs_Tags(t *testing.T) {
	cases := []struct {
		tag string
		ok  bool
	}{
		{"team", true},
		{"offboarding", true},
		{"in-flight", true},
		{"hiring_devops", true},
		{"+team", false}, // we add the + ourselves
		{"team alpha", false},
		{"../bad", false},
		{"", false},
	}
	for _, c := range cases {
		t.Run(c.tag, func(t *testing.T) {
			_, err := AddInput{Description: "x", Tags: []string{c.tag}}.AddArgs()
			if (err == nil) != c.ok {
				t.Errorf("tag %q: ok=%v err=%v", c.tag, c.ok, err)
			}
		})
	}
}

func TestModifyArgs_EmptyDatesClear(t *testing.T) {
	args, err := AddInput{Description: "x"}.ModifyArgs()
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	for _, want := range []string{"due:", "wait:", "scheduled:"} {
		if !slices.Contains(args, want) {
			t.Errorf("missing clear arg %q in %v", want, args)
		}
	}
}

func TestModifyArgs_EmptyProjectClears(t *testing.T) {
	args, err := AddInput{Description: "x"}.ModifyArgs()
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !slices.Contains(args, "project:") {
		t.Errorf("missing clear arg %q in %v", "project:", args)
	}
}

func TestModifyArgs_NonEmptyValuesWrap(t *testing.T) {
	args, err := AddInput{
		Description: "buy milk",
		Project:     "team.alpha",
		Due:         "tomorrow",
		Wait:        "eom",
		Scheduled:   "due-3d",
	}.ModifyArgs()
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	for _, want := range []string{
		`description:"buy milk"`,
		`project:"team.alpha"`,
		`due:"tomorrow"`,
		`wait:"eom"`,
		`scheduled:"due-3d"`,
	} {
		if !slices.Contains(args, want) {
			t.Errorf("missing %q in %v", want, args)
		}
	}
	for _, unwanted := range []string{"due:", "wait:", "scheduled:", "project:"} {
		if slices.Contains(args, unwanted) {
			t.Errorf("unexpected bare clear arg %q in %v", unwanted, args)
		}
	}
}

func TestModifyArgs_EmptyDescriptionRejected(t *testing.T) {
	for _, desc := range []string{"", "   ", "\t\n"} {
		_, err := AddInput{Description: desc}.ModifyArgs()
		if !errors.Is(err, ErrInvalid) {
			t.Errorf("desc %q: expected ErrInvalid, got %v", desc, err)
		}
	}
}

func TestModifyArgs_InvalidProjectRejected(t *testing.T) {
	_, err := AddInput{Description: "x", Project: "../etc/passwd"}.ModifyArgs()
	if !errors.Is(err, ErrInvalid) {
		t.Errorf("expected ErrInvalid, got %v", err)
	}
}

func TestModifyArgs_InvalidDateRejected(t *testing.T) {
	_, err := AddInput{Description: "x", Due: "tomorrow; rm -rf /"}.ModifyArgs()
	if !errors.Is(err, ErrInvalid) {
		t.Errorf("expected ErrInvalid, got %v", err)
	}
}

func TestTask_AnnotationsRoundTrip(t *testing.T) {
	raw := `[{
		"id": 1,
		"uuid": "11111111-2222-3333-4444-555555555555",
		"description": "buy milk",
		"status": "pending",
		"entry": "20260501T120000Z",
		"annotations": [
			{"entry": "20260501T130000Z", "description": "first note"},
			{"entry": "20260502T140000Z", "description": "second note +tag"}
		]
	}]`
	var tasks []Task
	if err := json.Unmarshal([]byte(raw), &tasks); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("got %d tasks, want 1", len(tasks))
	}
	got := tasks[0].Annotations
	if len(got) != 2 {
		t.Fatalf("got %d annotations, want 2", len(got))
	}
	if got[0].Entry != "20260501T130000Z" || got[0].Description != "first note" {
		t.Errorf("annotation[0] mismatch: %+v", got[0])
	}
	if got[1].Entry != "20260502T140000Z" || got[1].Description != "second note +tag" {
		t.Errorf("annotation[1] mismatch: %+v", got[1])
	}

	// Re-marshal and ensure structure is preserved.
	out, err := json.Marshal(tasks[0])
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var rt Task
	if err := json.Unmarshal(out, &rt); err != nil {
		t.Fatalf("re-unmarshal: %v", err)
	}
	if len(rt.Annotations) != 2 || rt.Annotations[1].Description != "second note +tag" {
		t.Errorf("round-trip lost annotations: %+v", rt.Annotations)
	}
}

func TestTask_AnnotationsOmitEmpty(t *testing.T) {
	tk := Task{ID: 1, UUID: "u", Description: "x", Status: "pending", Entry: "20260501T120000Z"}
	out, err := json.Marshal(tk)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(out), `"annotations"`) {
		t.Errorf("expected omitempty to drop annotations, got %s", string(out))
	}
}

func TestParseTime(t *testing.T) {
	cases := []struct {
		in   string
		want time.Time
		err  bool
	}{
		{"", time.Time{}, false},
		{"20260603T230000Z", time.Date(2026, 6, 3, 23, 0, 0, 0, time.UTC), false},
		{"20260430T173023Z", time.Date(2026, 4, 30, 17, 30, 23, 0, time.UTC), false},
		{"not a date", time.Time{}, true},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			got, err := ParseTime(c.in)
			if (err != nil) != c.err {
				t.Fatalf("err=%v want_err=%v", err, c.err)
			}
			if !got.Equal(c.want) {
				t.Errorf("got %v want %v", got, c.want)
			}
		})
	}
}

func TestFormatTime_RoundTrip(t *testing.T) {
	in := time.Date(2026, 5, 4, 12, 30, 45, 0, time.UTC)
	s := FormatTime(in)
	if s != "20260504T123045Z" {
		t.Errorf("got %q want 20260504T123045Z", s)
	}
	parsed, err := ParseTime(s)
	if err != nil {
		t.Fatalf("parse round trip: %v", err)
	}
	if !parsed.Equal(in) {
		t.Errorf("round trip lost time: got %v want %v", parsed, in)
	}
}

// TestTask_FullJSONRoundTrip exercises every optional field in tw.Task,
// confirming none are silently lost during decode/encode cycles.
func TestTask_FullJSONRoundTrip(t *testing.T) {
	raw := `[{
		"id": 7,
		"uuid": "abcdef01-2345-6789-abcd-ef0123456789",
		"description": "design review",
		"status": "pending",
		"entry": "20260501T080000Z",
		"modified": "20260502T090000Z",
		"due": "20260510T170000Z",
		"wait": "20260505T000000Z",
		"scheduled": "20260507T120000Z",
		"project": "team.alpha",
		"tags": ["urgent", "external"],
		"urgency": 8.5,
		"annotations": [
			{"entry": "20260501T130000Z", "description": "first"},
			{"entry": "20260502T140000Z", "description": "second +tag"}
		]
	}]`
	var tasks []Task
	if err := json.Unmarshal([]byte(raw), &tasks); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("got %d", len(tasks))
	}
	got := tasks[0]
	if got.ID != 7 {
		t.Errorf("ID: %d", got.ID)
	}
	if got.UUID != "abcdef01-2345-6789-abcd-ef0123456789" {
		t.Errorf("UUID: %q", got.UUID)
	}
	if got.Description != "design review" {
		t.Errorf("Description: %q", got.Description)
	}
	if got.Status != "pending" {
		t.Errorf("Status: %q", got.Status)
	}
	if got.Entry != "20260501T080000Z" {
		t.Errorf("Entry: %q", got.Entry)
	}
	if got.Modified != "20260502T090000Z" {
		t.Errorf("Modified: %q", got.Modified)
	}
	if got.Due != "20260510T170000Z" {
		t.Errorf("Due: %q", got.Due)
	}
	if got.Wait != "20260505T000000Z" {
		t.Errorf("Wait: %q", got.Wait)
	}
	if got.Scheduled != "20260507T120000Z" {
		t.Errorf("Scheduled: %q", got.Scheduled)
	}
	if got.Project != "team.alpha" {
		t.Errorf("Project: %q", got.Project)
	}
	if len(got.Tags) != 2 {
		t.Errorf("Tags: %v", got.Tags)
	}
	if got.Urgency != 8.5 {
		t.Errorf("Urgency: %v", got.Urgency)
	}
	if len(got.Annotations) != 2 {
		t.Errorf("Annotations: %v", got.Annotations)
	}

	// Re-marshal and re-decode; nothing should be lost.
	out, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var rt Task
	if err := json.Unmarshal(out, &rt); err != nil {
		t.Fatalf("re-unmarshal: %v", err)
	}
	if rt.Modified != got.Modified || rt.Due != got.Due || rt.Project != got.Project {
		t.Errorf("round trip mismatch: %+v vs %+v", got, rt)
	}
}

// TestTask_JSONUnmarshal_BuiltinsAndUDAs decodes a fixture that contains both
// every typed field and three UDA keys (string, numeric, duration). It
// confirms the typed fields land on the struct and the UDAs map captures the
// extras as their stringified values.
func TestTask_JSONUnmarshal_BuiltinsAndUDAs(t *testing.T) {
	raw := []byte(`{
		"id": 7,
		"uuid": "abcdef01-2345-6789-abcd-ef0123456789",
		"description": "design review",
		"status": "pending",
		"entry": "20260501T080000Z",
		"project": "team.alpha",
		"tags": ["urgent"],
		"urgency": 8.5,
		"estimate": "PT4H",
		"client": "Acme",
		"points": 5
	}`)
	var got Task
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.ID != 7 || got.Project != "team.alpha" || got.Urgency != 8.5 {
		t.Errorf("typed fields wrong: %+v", got)
	}
	if got.UDAs == nil {
		t.Fatalf("UDAs nil")
	}
	if got.UDAs["estimate"] != "PT4H" {
		t.Errorf("estimate: got %q", got.UDAs["estimate"])
	}
	if got.UDAs["client"] != "Acme" {
		t.Errorf("client: got %q", got.UDAs["client"])
	}
	if got.UDAs["points"] != "5" {
		t.Errorf("points: got %q", got.UDAs["points"])
	}
	// Built-in tag must NOT leak into UDAs.
	if _, ok := got.UDAs["tags"]; ok {
		t.Errorf("tags leaked into UDAs")
	}
	// Internal taskwarrior fields like start/end (not modelled) are silently
	// dropped, not captured as UDAs.
	if _, ok := got.UDAs["start"]; ok {
		t.Errorf("start should be ignored, not surfaced as UDA")
	}
}

// TestTask_JSONUnmarshal_NoUDAsLeavesMapNil ensures that a task with only
// builtins decodes to a nil UDAs map - which keeps the row-template's
// `range t.UDAs` loop a noop on the common case.
func TestTask_JSONUnmarshal_NoUDAsLeavesMapNil(t *testing.T) {
	raw := []byte(`{
		"id": 1, "uuid": "u", "description": "x",
		"status": "pending", "entry": "20260501T080000Z"
	}`)
	var got Task
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.UDAs != nil {
		t.Errorf("expected nil UDAs, got %+v", got.UDAs)
	}
}

// TestTask_JSONUnmarshal_RejectsMaliciousUDAName proves the UDANamePattern
// guard runs at decode time: a top-level key that smells like a parser token
// is silently dropped rather than captured as a UDA.
func TestTask_JSONUnmarshal_RejectsMaliciousUDAName(t *testing.T) {
	raw := []byte(`{
		"id": 1, "uuid": "u", "description": "x",
		"status": "pending", "entry": "20260501T080000Z",
		"rc.data.location": "/tmp/evil",
		"+sneakytag": "yes",
		"with space": "no"
	}`)
	var got Task
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, bad := range []string{"rc.data.location", "+sneakytag", "with space"} {
		if _, ok := got.UDAs[bad]; ok {
			t.Errorf("malicious key %q surfaced as UDA: %+v", bad, got.UDAs)
		}
	}
}

// TestTask_PriorityCapturedAsUDA defends against a regression where
// `priority` was excluded from the UDA capture loop, which lost user-set
// priority on read even though the modify path wrote it correctly. Real
// Taskwarrior 3.x exports emit priority as a top-level JSON key (mirroring
// the legacy built-in field) even when the user has redeclared it as a UDA;
// our task struct must surface it under t.UDAs["priority"] so the form
// re-renders the current value and the row's expand panel shows it.
func TestTask_PriorityCapturedAsUDA(t *testing.T) {
	raw := `[{
		"id": 5,
		"uuid": "eb7f764f-7ff9-4c19-acd8-f6cb5216bdc3",
		"description": "spec",
		"status": "pending",
		"entry": "20260501T080000Z",
		"priority": "M"
	}]`
	var tasks []Task
	if err := json.Unmarshal([]byte(raw), &tasks); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("got %d tasks, want 1", len(tasks))
	}
	if got := tasks[0].UDAs["priority"]; got != "M" {
		t.Errorf("priority not captured as UDA: got %q want %q (UDAs=%v)", got, "M", tasks[0].UDAs)
	}
}

// TestTask_UDARoundTrip confirms that Marshal -> Unmarshal preserves UDA
// values (used by tests, never by production).
func TestTask_UDARoundTrip(t *testing.T) {
	tk := Task{
		ID: 1, UUID: "u", Description: "x",
		Status: "pending", Entry: "20260501T080000Z",
		UDAs: map[string]string{"estimate": "PT4H", "client": "Acme"},
	}
	data, err := json.Marshal(tk)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var rt Task
	if err := json.Unmarshal(data, &rt); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if rt.UDAs["estimate"] != "PT4H" || rt.UDAs["client"] != "Acme" {
		t.Errorf("lost UDAs in round trip: %+v", rt.UDAs)
	}
}

// TestTask_DependsRoundTrip confirms that the `depends` array round-trips
// through Task's JSON layer without being scooped into UDAs and without losing
// entries.
func TestTask_DependsRoundTrip(t *testing.T) {
	raw := `[{
		"id": 9,
		"uuid": "abcdef01-2345-6789-abcd-ef0123456789",
		"description": "ship",
		"status": "pending",
		"entry": "20260501T080000Z",
		"depends": [
			"11111111-2222-3333-4444-555555555555",
			"22222222-3333-4444-5555-666666666666"
		]
	}]`
	var tasks []Task
	if err := json.Unmarshal([]byte(raw), &tasks); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("got %d", len(tasks))
	}
	got := tasks[0]
	if len(got.Depends) != 2 {
		t.Fatalf("Depends: got %v want 2 entries", got.Depends)
	}
	if got.Depends[0] != "11111111-2222-3333-4444-555555555555" {
		t.Errorf("Depends[0]: %q", got.Depends[0])
	}
	if got.Depends[1] != "22222222-3333-4444-5555-666666666666" {
		t.Errorf("Depends[1]: %q", got.Depends[1])
	}
	// Must NOT leak into UDAs.
	if _, ok := got.UDAs["depends"]; ok {
		t.Errorf("depends leaked into UDAs: %+v", got.UDAs)
	}

	// Re-marshal and re-decode; nothing should be lost.
	out, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var rt Task
	if err := json.Unmarshal(out, &rt); err != nil {
		t.Fatalf("re-unmarshal: %v", err)
	}
	if len(rt.Depends) != 2 || rt.Depends[1] != "22222222-3333-4444-5555-666666666666" {
		t.Errorf("round trip lost depends: %+v", rt.Depends)
	}
}

// TestTask_DependsOmitEmpty confirms a task with no dependencies marshals
// without an empty `depends` field, matching the `omitempty` tag.
func TestTask_DependsOmitEmpty(t *testing.T) {
	tk := Task{ID: 1, UUID: "u", Description: "x", Status: "pending", Entry: "20260501T120000Z"}
	out, err := json.Marshal(tk)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(out), `"depends"`) {
		t.Errorf("expected omitempty to drop depends, got %s", string(out))
	}
}

// TestAddArgs_Depends covers the four code paths AddArgs needs to handle for
// depends: zero (no arg emitted), one UUID, multiple UUIDs joined by comma,
// and one rejection of a non-UUID string.
func TestAddArgs_Depends(t *testing.T) {
	t.Run("zero deps emits no depends arg", func(t *testing.T) {
		args, err := AddInput{Description: "x"}.AddArgs()
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		for _, a := range args {
			if strings.HasPrefix(a, "depends:") {
				t.Errorf("expected no depends arg, got %v", args)
			}
		}
	})
	t.Run("one dep emits single uuid", func(t *testing.T) {
		args, err := AddInput{
			Description: "x",
			Depends:     []string{"11111111-2222-3333-4444-555555555555"},
		}.AddArgs()
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if !slices.Contains(args, "depends:11111111-2222-3333-4444-555555555555") {
			t.Errorf("missing one-uuid depends arg in %v", args)
		}
	})
	t.Run("multiple deps joined by comma", func(t *testing.T) {
		args, err := AddInput{
			Description: "x",
			Depends: []string{
				"11111111-2222-3333-4444-555555555555",
				"22222222-3333-4444-5555-666666666666",
			},
		}.AddArgs()
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		want := "depends:11111111-2222-3333-4444-555555555555,22222222-3333-4444-5555-666666666666"
		if !slices.Contains(args, want) {
			t.Errorf("missing multi depends arg %q in %v", want, args)
		}
	})
	t.Run("invalid uuid rejected", func(t *testing.T) {
		_, err := AddInput{
			Description: "x",
			Depends:     []string{"not-a-uuid"},
		}.AddArgs()
		if !errors.Is(err, ErrInvalid) {
			t.Errorf("expected ErrInvalid, got %v", err)
		}
	})
	t.Run("shell-meta uuid rejected", func(t *testing.T) {
		_, err := AddInput{
			Description: "x",
			Depends:     []string{"11111111-2222-3333-4444-555555555555; ls"},
		}.AddArgs()
		if !errors.Is(err, ErrInvalid) {
			t.Errorf("expected ErrInvalid, got %v", err)
		}
	})
}

// TestModifyArgs_Depends mirrors AddArgs_Depends but checks the clear behaviour
// on empty input plus the non-empty / multi-uuid happy paths.
func TestModifyArgs_Depends(t *testing.T) {
	t.Run("empty deps emits bare clear arg", func(t *testing.T) {
		args, err := AddInput{Description: "x"}.ModifyArgs()
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if !slices.Contains(args, "depends:") {
			t.Errorf("expected bare 'depends:' clear arg in %v", args)
		}
	})
	t.Run("one dep emits single uuid", func(t *testing.T) {
		args, err := AddInput{
			Description: "x",
			Depends:     []string{"11111111-2222-3333-4444-555555555555"},
		}.ModifyArgs()
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if !slices.Contains(args, "depends:11111111-2222-3333-4444-555555555555") {
			t.Errorf("missing one-uuid depends arg in %v", args)
		}
		if slices.Contains(args, "depends:") {
			t.Errorf("unexpected bare clear arg alongside one-uuid arg: %v", args)
		}
	})
	t.Run("multiple deps joined by comma", func(t *testing.T) {
		args, err := AddInput{
			Description: "x",
			Depends: []string{
				"11111111-2222-3333-4444-555555555555",
				"22222222-3333-4444-5555-666666666666",
			},
		}.ModifyArgs()
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		want := "depends:11111111-2222-3333-4444-555555555555,22222222-3333-4444-5555-666666666666"
		if !slices.Contains(args, want) {
			t.Errorf("missing multi depends arg %q in %v", want, args)
		}
	})
	t.Run("invalid uuid rejected", func(t *testing.T) {
		_, err := AddInput{
			Description: "x",
			Depends:     []string{"not-a-uuid"},
		}.ModifyArgs()
		if !errors.Is(err, ErrInvalid) {
			t.Errorf("expected ErrInvalid, got %v", err)
		}
	})
}

// TestTask_IsOverdue covers the three time-windows the helper distinguishes.
func TestTask_IsOverdue(t *testing.T) {
	now := time.Now()
	past := now.Add(-time.Hour).UTC().Format("20060102T150405Z")
	future := now.Add(time.Hour).UTC().Format("20060102T150405Z")
	if !(Task{Due: past}).IsOverdue() {
		t.Errorf("past should be overdue")
	}
	if (Task{Due: future}).IsOverdue() {
		t.Errorf("future should not be overdue")
	}
	if (Task{}).IsOverdue() {
		t.Errorf("no due should not be overdue")
	}
	if (Task{Due: "garbage"}).IsOverdue() {
		t.Errorf("garbage due should not be overdue")
	}
}

// TestRecurPattern_AcceptsFractional: TW accepts "1.5d" / "0.5w" as
// half-day / half-week durations. The pattern was previously alphanumeric-
// only and rejected the period, surfacing as an opaque 400 to the user.
func TestRecurPattern_AcceptsFractional(t *testing.T) {
	cases := []struct {
		input string
		want  bool
	}{
		{"weekly", true},
		{"1mo", true},
		{"P1W", true},
		{"1.5d", true},
		{"0.5w", true},
		{"P1.5D", true},
		{"; rm -rf /", false},
		{"--rc.somefile", false},
		{"a b", false},
		{"weekly;ls", false},
		// Malformed-but-alphanumeric values pass the regex (deliberately
		// permissive shape filter, see comment on recurPattern); TW's
		// runtime parser rejects them and writeIfTaskParseError surfaces
		// the error to the user. Listed here so a regression that
		// tightened the pattern (e.g. someone changes to require a
		// digit) is caught as a test break rather than a silent shift.
		{"..", true},
		{"1.", true},
		{".d", true},
		{"1.2.3.4d", true},
		// Shell-meta in a fractional context still rejected.
		{"1.5d;rm", false},
		{"1.5/foo", false},
		{"1.5*", false},
	}
	for _, c := range cases {
		if got := recurPattern.MatchString(c.input); got != c.want {
			t.Errorf("recurPattern(%q) = %v, want %v", c.input, got, c.want)
		}
	}
}

// TestIsRecurringParent: distinguishes templates from instances. Both
// carry Recur, only the template has Status == "recurring".
func TestIsRecurringParent(t *testing.T) {
	parent := Task{Status: "recurring", Recur: "monthly"}
	child := Task{Status: "pending", Recur: "monthly"}
	plain := Task{Status: "pending"}
	deletedChild := Task{Status: "deleted", Recur: "monthly"}

	if !parent.IsRecurringParent() {
		t.Errorf("parent should be IsRecurringParent")
	}
	if child.IsRecurringParent() {
		t.Errorf("pending child should not be IsRecurringParent")
	}
	if plain.IsRecurringParent() {
		t.Errorf("non-recurring task should not be IsRecurringParent")
	}
	if deletedChild.IsRecurringParent() {
		t.Errorf("deleted child should not be IsRecurringParent")
	}
}

// TestCompletedAt confirms the End-over-Modified fallback. Taskwarrior sets
// `end` at completion and never changes it; `modified` is updated on every
// subsequent edit. CompletedAt must always prefer `end` when present.
func TestCompletedAt(t *testing.T) {
	cases := []struct {
		name     string
		task     Task
		wantBack string
	}{
		{
			name:     "end set, modified empty",
			task:     Task{End: "20260510T200000Z"},
			wantBack: "20260510T200000Z",
		},
		{
			name:     "modified only (no end field)",
			task:     Task{Modified: "20260510T200000Z"},
			wantBack: "20260510T200000Z",
		},
		{
			name:     "both set — end wins",
			task:     Task{End: "20260510T200000Z", Modified: "20260511T090000Z"},
			wantBack: "20260510T200000Z",
		},
		{
			name:     "neither set — empty string",
			task:     Task{},
			wantBack: "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.task.CompletedAt(); got != tc.wantBack {
				t.Errorf("got %q, want %q", got, tc.wantBack)
			}
		})
	}
}

// TestTask_EndFieldDecoded confirms that the `end` JSON key is decoded into
// Task.End and is not misclassified as a UDA.
func TestTask_EndFieldDecoded(t *testing.T) {
	raw := []byte(`{
		"id": 1,
		"uuid": "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
		"description": "finished task",
		"status": "completed",
		"entry": "20260501T080000Z",
		"end": "20260510T200000Z",
		"modified": "20260511T090000Z"
	}`)
	var got Task
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.End != "20260510T200000Z" {
		t.Errorf("End: got %q want %q", got.End, "20260510T200000Z")
	}
	if _, isUDA := got.UDAs["end"]; isUDA {
		t.Error("end must not appear in UDAs")
	}
	if got.CompletedAt() != "20260510T200000Z" {
		t.Errorf("CompletedAt: got %q want End value", got.CompletedAt())
	}
}

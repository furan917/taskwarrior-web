package views

import (
	"strings"
	"testing"
	"time"

	"github.com/furan917/taskwarrior-web-portal/internal/tw"
)

// twTime formats t as a Taskwarrior YYYYMMDDTHHMMSSZ string in UTC. It's a
// one-line copy of the same helper in handlers/calendar_test.go; we don't
// share the body across packages because exporting a one-liner just to
// satisfy DRY costs more than the duplication. If either copy changes, keep
// them in sync.
func twTime(t time.Time) string { return t.UTC().Format("20060102T150405Z") }

func TestHumanDate(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string // "" means "expect non-empty parsed local date" (regex-ish)
	}{
		{"empty", "", ""},
		{"unparseable returns input", "not-a-date", "not-a-date"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := humanDate(c.in); got != c.want {
				t.Errorf("got %q want %q", got, c.want)
			}
		})
	}

	// A real timestamp -> YYYY-MM-DD in local zone.
	tt := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	got := humanDate(twTime(tt))
	want := tt.Local().Format("2006-01-02")
	if got != want {
		t.Errorf("real ts: got %q want %q", got, want)
	}
}

func TestHumanDateTime(t *testing.T) {
	if got := humanDateTime(""); got != "" {
		t.Errorf("empty: got %q", got)
	}
	if got := humanDateTime("nope"); got != "nope" {
		t.Errorf("unparseable: got %q want %q", got, "nope")
	}
	tt := time.Date(2026, 5, 1, 13, 30, 0, 0, time.UTC)
	got := humanDateTime(twTime(tt))
	want := tt.Local().Format("Mon 2 Jan, 15:04")
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestUrgencyTier(t *testing.T) {
	cases := []struct {
		score float64
		want  string
	}{
		{-5, "low"},
		{0, "low"},
		{2.99, "low"},
		{3, "med"},
		{5.99, "med"},
		{6, "high"},
		{8.99, "high"},
		{9, "critical"},
		{12, "critical"},
	}
	for _, c := range cases {
		if got := urgencyTier(c.score); got != c.want {
			t.Errorf("score %v: got %q want %q", c.score, got, c.want)
		}
	}
}

func TestTierSolidLightBadge(t *testing.T) {
	for _, tier := range []string{"critical", "high", "med", "low"} {
		if s := tierSolid(tier); s == "" {
			t.Errorf("tierSolid(%q) empty", tier)
		}
		if l := tierLight(tier); l == "" {
			t.Errorf("tierLight(%q) empty", tier)
		}
		if b := tierBadge(tier); b == "" {
			t.Errorf("tierBadge(%q) empty", tier)
		}
	}

	// Critical/high/med/low must each pick the documented colour family.
	if !strings.Contains(tierSolid("critical"), "red") {
		t.Errorf("critical should be red, got %q", tierSolid("critical"))
	}
	if !strings.Contains(tierSolid("high"), "orange") {
		t.Errorf("high should be orange, got %q", tierSolid("high"))
	}
	if !strings.Contains(tierSolid("med"), "yellow") {
		t.Errorf("med should be yellow, got %q", tierSolid("med"))
	}
	if !strings.Contains(tierSolid("low"), "blue") {
		t.Errorf("low should be blue, got %q", tierSolid("low"))
	}

	// tierLight must include the inset ring class (visual cue for scheduled day).
	for _, tier := range []string{"critical", "high", "med", "low"} {
		if !strings.Contains(tierLight(tier), "ring-inset") {
			t.Errorf("tierLight(%q) missing ring-inset: %q", tier, tierLight(tier))
		}
	}

	// tierBadge must NOT include a ring (kept minimal vs tierLight).
	for _, tier := range []string{"critical", "high", "med", "low"} {
		if strings.Contains(tierBadge(tier), "ring-inset") {
			t.Errorf("tierBadge(%q) should not include ring-inset: %q", tier, tierBadge(tier))
		}
	}

	// Unknown tier name falls through to "low" (blue).
	if !strings.Contains(tierSolid("unknown"), "blue") {
		t.Errorf("unknown tier should fall through to low/blue, got %q", tierSolid("unknown"))
	}
}

func TestDueLabel(t *testing.T) {
	now := time.Now()

	// Empty / unparseable falls back to raw input.
	if got := dueLabel(""); got != "" {
		t.Errorf("empty: got %q", got)
	}
	if got := dueLabel("garbage"); got != "garbage" {
		t.Errorf("garbage: got %q want %q", got, "garbage")
	}

	// Overdue by ~2 days.
	overdue := twTime(now.Add(-48 * time.Hour))
	if got := dueLabel(overdue); !strings.Contains(got, "overdue") {
		t.Errorf("overdue: got %q want substring 'overdue'", got)
	}

	// Within 24h: "today".
	soon := twTime(now.Add(2 * time.Hour))
	if got := dueLabel(soon); got != "today" {
		t.Errorf("soon: got %q want 'today'", got)
	}

	// Future: "in Nd".
	future := twTime(now.Add(72 * time.Hour))
	if got := dueLabel(future); !strings.HasPrefix(got, "in ") || !strings.HasSuffix(got, "d") {
		t.Errorf("future: got %q want prefix 'in ' suffix 'd'", got)
	}
}

func TestDueBadgeClass(t *testing.T) {
	now := time.Now()
	base := "inline-flex items-center rounded px-2 py-0.5 text-xs font-medium"

	// Empty -> low/blue.
	if got := dueBadgeClass(""); !strings.HasPrefix(got, base) || !strings.Contains(got, "blue") {
		t.Errorf("empty: got %q want base+blue", got)
	}

	// Overdue -> critical/red.
	if got := dueBadgeClass(twTime(now.Add(-48 * time.Hour))); !strings.Contains(got, "red") {
		t.Errorf("overdue: got %q want red", got)
	}
	// Within 24h -> high/orange.
	if got := dueBadgeClass(twTime(now.Add(2 * time.Hour))); !strings.Contains(got, "orange") {
		t.Errorf("near: got %q want orange", got)
	}
	// 3 days -> med/yellow.
	if got := dueBadgeClass(twTime(now.Add(72 * time.Hour))); !strings.Contains(got, "yellow") {
		t.Errorf("3d: got %q want yellow", got)
	}
	// 30 days -> low/blue.
	if got := dueBadgeClass(twTime(now.Add(30 * 24 * time.Hour))); !strings.Contains(got, "blue") {
		t.Errorf("30d: got %q want blue", got)
	}
}

func TestUrgencyBarColour(t *testing.T) {
	for _, c := range []struct {
		score float64
		want  string
	}{
		{0, "blue"},
		{2, "blue"},
		{4, "yellow"},
		{7, "orange"},
		{10, "red"},
	} {
		got := urgencyBarColour(c.score)
		if !strings.Contains(got, c.want) {
			t.Errorf("score %v: got %q want substring %q", c.score, got, c.want)
		}
	}
}

func TestUrgencyPercent(t *testing.T) {
	cases := []struct {
		score float64
		want  float64
	}{
		{-1, 0},
		{0, 0},
		{5, 50},
		{10, 100},
		{15, 100}, // clamps
	}
	for _, c := range cases {
		if got := urgencyPercent(c.score); got != c.want {
			t.Errorf("score %v: got %v want %v", c.score, got, c.want)
		}
	}
}

func TestSortChipsByUrgency(t *testing.T) {
	chips := []CalendarChip{
		{Task: tw.Task{Urgency: 1}, Position: "single"},
		{Task: tw.Task{Urgency: 9}, Position: "single"},
		{Task: tw.Task{Urgency: 5}, Position: "single"},
	}
	SortChipsByUrgency(chips)
	if chips[0].Task.Urgency != 9 || chips[1].Task.Urgency != 5 || chips[2].Task.Urgency != 1 {
		t.Errorf("not sorted desc: %v", chips)
	}
}

func TestTaskSpan(t *testing.T) {
	// No due date -> not on calendar.
	if _, _, ok := TaskSpan(tw.Task{}); ok {
		t.Errorf("empty task: ok=true want false")
	}

	// Due only -> single-day span.
	due := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	tk := tw.Task{Due: twTime(due)}
	start, end, ok := TaskSpan(tk)
	if !ok {
		t.Fatalf("due-only: ok=false")
	}
	if !start.Equal(end) {
		t.Errorf("due-only: start=%v end=%v want equal", start, end)
	}

	// Scheduled before due -> scheduled..due range.
	sched := time.Date(2026, 5, 7, 9, 0, 0, 0, time.UTC)
	tk2 := tw.Task{Due: twTime(due), Scheduled: twTime(sched)}
	start, end, ok = TaskSpan(tk2)
	if !ok {
		t.Fatalf("sched: ok=false")
	}
	if !start.Equal(DateOnly(sched.Local())) {
		t.Errorf("sched: start=%v want %v", start, sched.Local())
	}
	if !end.Equal(DateOnly(due.Local())) {
		t.Errorf("sched: end=%v want %v", end, due.Local())
	}

	// Scheduled AFTER due -> ignored, single-day span.
	schedLater := due.Add(48 * time.Hour)
	tk3 := tw.Task{Due: twTime(due), Scheduled: twTime(schedLater)}
	start, end, ok = TaskSpan(tk3)
	if !ok || !start.Equal(end) {
		t.Errorf("sched-after-due: should collapse, got start=%v end=%v ok=%v", start, end, ok)
	}

	// Wait fallback: no scheduled, wait before due.
	wait := due.Add(-72 * time.Hour)
	tk4 := tw.Task{Due: twTime(due), Wait: twTime(wait)}
	start, _, ok = TaskSpan(tk4)
	if !ok {
		t.Fatalf("wait-fallback: ok=false")
	}
	if !start.Equal(DateOnly(wait.Local())) {
		t.Errorf("wait-fallback: start=%v want %v", start, wait.Local())
	}

	// Unparseable due -> not on calendar.
	if _, _, ok := TaskSpan(tw.Task{Due: "garbage"}); ok {
		t.Errorf("garbage due: ok=true want false")
	}
}

func TestCalendarCellClass(t *testing.T) {
	// Out-of-period: faded background.
	out := calendarCellClass(CalendarCell{InPeriod: false})
	if !strings.Contains(out, "bg-zinc-50") {
		t.Errorf("out-of-period: got %q want bg-zinc-50", out)
	}
	// Today: blue highlight.
	today := calendarCellClass(CalendarCell{InPeriod: true, IsToday: true})
	if !strings.Contains(today, "bg-blue-50") {
		t.Errorf("today: got %q want bg-blue-50", today)
	}
	// Plain in-period day: white background.
	plain := calendarCellClass(CalendarCell{InPeriod: true})
	if !strings.Contains(plain, "bg-white") {
		t.Errorf("plain: got %q want bg-white", plain)
	}
}

func TestCalendarChipClass(t *testing.T) {
	hi := tw.Task{Urgency: 10}
	// Single -> solid (white text on red).
	single := calendarChipClass(CalendarChip{Task: hi, Position: "single"})
	if !strings.Contains(single, "bg-red-600") || !strings.Contains(single, "text-white") {
		t.Errorf("single hi: got %q want red+white", single)
	}
	// End -> solid (due day).
	end := calendarChipClass(CalendarChip{Task: hi, Position: "end"})
	if !strings.Contains(end, "bg-red-600") {
		t.Errorf("end hi: got %q want bg-red-600", end)
	}
	// Start -> light (scheduled day).
	start := calendarChipClass(CalendarChip{Task: hi, Position: "start"})
	if !strings.Contains(start, "ring-inset") {
		t.Errorf("start hi: got %q want ring-inset (tierLight)", start)
	}
	// Middle -> light AND no rounded corners.
	mid := calendarChipClass(CalendarChip{Task: hi, Position: "middle"})
	if !strings.Contains(mid, "rounded-none") {
		t.Errorf("middle: got %q want rounded-none", mid)
	}
	// Start -> rounded-l + rounded-r-none.
	if !strings.Contains(start, "rounded-l") || !strings.Contains(start, "rounded-r-none") {
		t.Errorf("start corners: got %q", start)
	}
	// End -> rounded-r + rounded-l-none.
	if !strings.Contains(end, "rounded-r") || !strings.Contains(end, "rounded-l-none") {
		t.Errorf("end corners: got %q", end)
	}
	// Single -> plain rounded.
	if !strings.Contains(single, "rounded") {
		t.Errorf("single rounded: got %q", single)
	}
}

// TestDenotateVals confirms the JSON encoder handles the cases the old
// hand-rolled escaper did (plain text, embedded quotes, backslashes), AND
// the cases it broke (control characters, unicode line terminators) -
// json.Marshal escapes those as \uXXXX so HTMX can parse the body. Output
// follows json.Marshal's compact form (no space after colon).
func TestDenotateVals(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"plain note", `{"text":"plain note"}`},
		{`with "quote"`, `{"text":"with \"quote\""}`},
		{`backslash \ in note`, `{"text":"backslash \\ in note"}`},
		{`combo \ and "q"`, `{"text":"combo \\ and \"q\""}`},
		{"line\nbreak", `{"text":"line\nbreak"}`},
		{"tab\there", `{"text":"tab\there"}`},
		{"null\x00byte", "{\"text\":\"null\\u0000byte\"}"},
	}
	for _, c := range cases {
		if got := denotateVals(c.in); got != c.want {
			t.Errorf("in=%q\ngot=%q\nwant=%q", c.in, got, c.want)
		}
	}
}

func TestEmptyMessage(t *testing.T) {
	for _, report := range []string{"next", "ready", "agenda", "forecast", "other"} {
		got := emptyMessage(report)
		if got == "" {
			t.Errorf("emptyMessage(%q) is empty", report)
		}
	}
}

func TestTaskKey(t *testing.T) {
	if got := taskKey(tw.Task{UUID: "abc"}); got != "abc" {
		t.Errorf("uuid: got %q", got)
	}
	if got := taskKey(tw.Task{ID: 42}); got != "42" {
		t.Errorf("id: got %q", got)
	}
	if got := taskKey(tw.Task{}); got != "0" {
		t.Errorf("empty: got %q", got)
	}
}

func TestRowURLs(t *testing.T) {
	tk := tw.Task{UUID: "u-1"}
	if got := rowDoneURL(tk); got != "/tasks/u-1/done" {
		t.Errorf("done url: %q", got)
	}
	if got := rowEditURL(tk); got != "/forms/edit/u-1" {
		t.Errorf("edit url: %q", got)
	}
}

func TestPartialURL(t *testing.T) {
	if got := partialURL("next", ""); got != "/partials/list?report=next" {
		t.Errorf("report: got %q", got)
	}
	if got := partialURL("", "team.alpha"); got != "/partials/list?project=team.alpha" {
		t.Errorf("project: got %q", got)
	}
	// project takes precedence when both supplied.
	if got := partialURL("next", "p"); got != "/partials/list?project=p" {
		t.Errorf("both: got %q", got)
	}
}

func TestPartialURLWithSort(t *testing.T) {
	// Default sort, no filter: omit both params.
	if got := partialURLWithSort("next", "", DefaultSort, ""); got != "/partials/list?report=next" {
		t.Errorf("default: got %q", got)
	}
	// Non-default sort, no filter.
	got := partialURLWithSort("next", "", SortSpec{Key: "due", Asc: true}, "")
	if got != "/partials/list?report=next&sort=due:asc" {
		t.Errorf("explicit due asc: got %q", got)
	}
	// Project drilldown with sort.
	got = partialURLWithSort("", "team.alpha", SortSpec{Key: "project", Asc: true}, "")
	if got != "/partials/list?project=team.alpha&sort=project:asc" {
		t.Errorf("project drilldown: got %q", got)
	}
	// Filter only (default sort).
	got = partialURLWithSort("next", "", DefaultSort, "+work")
	if got != "/partials/list?report=next&filter=%2Bwork" {
		t.Errorf("filter only: got %q", got)
	}
	// Sort + filter together.
	got = partialURLWithSort("next", "", SortSpec{Key: "due", Asc: true}, "priority:H")
	if got != "/partials/list?report=next&sort=due:asc&filter=priority%3AH" {
		t.Errorf("sort+filter: got %q", got)
	}
}

func TestSortURL_FlipsDirectionOnSameKey(t *testing.T) {
	cur := SortSpec{Key: "urgency", Asc: false}
	got := sortURL("next", "", cur, "urgency")
	if !strings.Contains(got, "sort=urgency%3Aasc") {
		t.Errorf("expected flip to asc: got %q", got)
	}
}

func TestSortURL_NewKeyUsesDefaultDir(t *testing.T) {
	cur := SortSpec{Key: "urgency", Asc: false}
	// due defaults to asc.
	got := sortURL("next", "", cur, "due")
	if !strings.Contains(got, "sort=due%3Aasc") {
		t.Errorf("expected due asc: got %q", got)
	}
	// entry defaults to desc.
	got = sortURL("next", "", cur, "entry")
	if !strings.Contains(got, "sort=entry%3Adesc") {
		t.Errorf("expected entry desc: got %q", got)
	}
}

func TestSortURL_PrefersProjectOverReport(t *testing.T) {
	got := sortURL("", "team.alpha", SortSpec{Key: "urgency"}, "due")
	if !strings.Contains(got, "project=team.alpha") {
		t.Errorf("expected project param: got %q", got)
	}
	if strings.Contains(got, "report=") {
		t.Errorf("did not expect report param when project set: got %q", got)
	}
}

func TestSortLinkClass_HighlightsActive(t *testing.T) {
	cur := SortSpec{Key: "due", Asc: true}
	if got := sortLinkClass(cur, "due"); !strings.Contains(got, "font-semibold") {
		t.Errorf("active key should be bold: got %q", got)
	}
	if got := sortLinkClass(cur, "urgency"); strings.Contains(got, "font-semibold") {
		t.Errorf("inactive key should not be bold: got %q", got)
	}
}

func TestSortArrow(t *testing.T) {
	if got := sortArrow(SortSpec{Key: "due", Asc: true}, "due"); got != "↑" {
		t.Errorf("asc: got %q", got)
	}
	if got := sortArrow(SortSpec{Key: "due", Asc: false}, "due"); got != "↓" {
		t.Errorf("desc: got %q", got)
	}
	if got := sortArrow(SortSpec{Key: "due", Asc: true}, "urgency"); got != "" {
		t.Errorf("inactive: got %q", got)
	}
}

func TestFormatSort(t *testing.T) {
	if got := FormatSort(SortSpec{Key: "urgency", Asc: false}); got != "urgency:desc" {
		t.Errorf("urgency desc: %q", got)
	}
	if got := FormatSort(SortSpec{Key: "due", Asc: true}); got != "due:asc" {
		t.Errorf("due asc: %q", got)
	}
}

func TestStartOfMonday(t *testing.T) {
	// Mon 4 May 2026 -> itself.
	mon := time.Date(2026, 5, 4, 14, 0, 0, 0, time.UTC)
	got := StartOfMonday(mon)
	want := time.Date(2026, 5, 4, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("monday: got %v want %v", got, want)
	}
	// Sunday 3 May 2026 -> previous Monday 27 Apr.
	sun := time.Date(2026, 5, 3, 23, 59, 0, 0, time.UTC)
	got = StartOfMonday(sun)
	want = time.Date(2026, 4, 27, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("sunday: got %v want %v", got, want)
	}
	// Wed 6 May 2026 -> Mon 4 May.
	wed := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)
	got = StartOfMonday(wed)
	want = time.Date(2026, 5, 4, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("wed: got %v want %v", got, want)
	}
}

func TestAddDaysAddMonths(t *testing.T) {
	base := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	if got := AddDays(base, 3); !got.Equal(time.Date(2026, 5, 4, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("AddDays: %v", got)
	}
	if got := AddMonths(base, 2); !got.Equal(time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("AddMonths: %v", got)
	}
}

func TestDateOnly(t *testing.T) {
	in := time.Date(2026, 5, 1, 14, 30, 45, 999, time.UTC)
	got := DateOnly(in)
	want := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("got %v want %v", got, want)
	}
}

func TestCalendarSwitchURL(t *testing.T) {
	p := CalendarPage{Anchor: time.Date(2026, 5, 4, 0, 0, 0, 0, time.UTC)}
	got := calendarSwitchURL(p, "week")
	want := "/calendar?view=week&date=2026-05-04"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestCalendarDayURL(t *testing.T) {
	d := time.Date(2026, 5, 4, 0, 0, 0, 0, time.UTC)
	if got := calendarDayURL(d); got != "/calendar?view=day&date=2026-05-04" {
		t.Errorf("got %q", got)
	}
}

func TestSortedTasksByUrgency(t *testing.T) {
	tasks := []tw.Task{
		{ID: 1, Urgency: 2},
		{ID: 2, Urgency: 9},
		{ID: 3, Urgency: 5},
	}
	SortTasksByUrgency(tasks)
	if tasks[0].ID != 2 || tasks[1].ID != 3 || tasks[2].ID != 1 {
		t.Errorf("not sorted desc: %+v", tasks)
	}
}

func TestCalendarWeekCellClassDayNumberClass(t *testing.T) {
	// week-cell today highlight.
	w := calendarWeekCellClass(CalendarCell{IsToday: true, InPeriod: true})
	if !strings.Contains(w, "bg-blue-50") {
		t.Errorf("week today: %q", w)
	}
	// not today.
	w2 := calendarWeekCellClass(CalendarCell{InPeriod: true})
	if !strings.Contains(w2, "bg-white") {
		t.Errorf("week plain: %q", w2)
	}
	// day-number today badge: blue pill, white text.
	d := calendarDayNumberClass(CalendarCell{IsToday: true, InPeriod: true})
	if !strings.Contains(d, "bg-blue-600") || !strings.Contains(d, "text-white") {
		t.Errorf("day number today: %q", d)
	}
	// out-of-period day-number: muted.
	d2 := calendarDayNumberClass(CalendarCell{InPeriod: false})
	if !strings.Contains(d2, "text-zinc-400") {
		t.Errorf("day number out: %q", d2)
	}
	// in-period plain.
	d3 := calendarDayNumberClass(CalendarCell{InPeriod: true})
	if !strings.Contains(d3, "text-zinc-700") {
		t.Errorf("day number plain: %q", d3)
	}
}

func TestCalendarDayTaskClass(t *testing.T) {
	if got := calendarDayTaskClass(tw.Task{}); !strings.Contains(got, "block") {
		t.Errorf("got %q", got)
	}
}

func TestContextPrefill(t *testing.T) {
	cases := []struct {
		name        string
		filter      string
		wantProject string
		wantTag     string
	}{
		{"empty", "", "", ""},
		{"project only", "project:team", "team", ""},
		{"tag only", "+client", "", "client"},
		{"tag-or-project: tag wins", "+client or project:client", "", "client"},
		{"or-chain: first lowercase tag", "+tech-oversight or project:security or project:pci", "", "tech-oversight"},
		{"virtual tags ignored", "+OVERDUE or +READY", "", ""},
		{"virtual then real", "+OVERDUE or +urgent", "", "urgent"},
		{"vendor tag", "+vendor or project:vendor", "", "vendor"},
		{"hierarchical project", "project:team", "team", ""},
		{"unknown shape", "due.before:7d", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gotProj, gotTag := ContextPrefill(c.filter)
			if gotProj != c.wantProject {
				t.Errorf("project: got %q want %q", gotProj, c.wantProject)
			}
			if gotTag != c.wantTag {
				t.Errorf("tag: got %q want %q", gotTag, c.wantTag)
			}
		})
	}
}

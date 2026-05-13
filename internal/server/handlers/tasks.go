package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/furan917/taskwarrior-web/internal/config"
	"github.com/furan917/taskwarrior-web/internal/tw"
	"github.com/furan917/taskwarrior-web/internal/views"
)

// bulkTimeout aliases config.BulkTimeout - canonical source of truth is
// the config package; the alias keeps the call-site comment readable.
const bulkTimeout = config.BulkTimeout

// Tasks holds dependencies for write-side handlers.
type Tasks struct {
	TW     *tw.Client
	Logger *slog.Logger
}

func (t *Tasks) Create(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	in := readAddInput(r)
	args, err := in.AddArgs()
	if err != nil {
		writeFormError(w, err)
		return
	}
	udaArgs, err := readUDAArgs(r, t.TW.UDAsCached(r.Context()), false)
	if err != nil {
		writeFormError(w, err)
		return
	}
	args = append(args, udaArgs...)
	if err := t.TW.Run(r.Context(), append([]string{"add"}, args...)...); err != nil {
		t.Logger.Error("task add failed", "err", err)
		if writeIfTaskParseError(w, err) {
			return
		}
		http.Error(w, "task add failed", http.StatusInternalServerError)
		return
	}
	writeRefresh(w)
}

func (t *Tasks) Modify(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !tw.IDPattern.MatchString(id) {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	existing, err := t.TW.Export(r.Context(), id)
	if err != nil {
		t.Logger.Error("export for modify failed", "id", id, "err", err)
		http.Error(w, "task export failed", http.StatusInternalServerError)
		return
	}
	if len(existing) == 0 {
		http.Error(w, "task not found", http.StatusNotFound)
		return
	}
	in := readAddInput(r)
	args, err := in.ModifyArgs()
	if err != nil {
		writeFormError(w, err)
		return
	}
	udaArgs, err := readUDAArgs(r, t.TW.UDAsCached(r.Context()), true)
	if err != nil {
		writeFormError(w, err)
		return
	}
	args = append(args, udaArgs...)
	// Tag delta: Taskwarrior modify accepts +tag and -tag. ModifyArgs already
	// emitted +newTag for everything in input.Tags; append -oldTag for tags
	// the user removed.
	wanted := stringSet(in.Tags)
	for _, oldTag := range existing[0].Tags {
		if _, ok := wanted[oldTag]; !ok {
			args = append(args, "-"+oldTag)
		}
	}
	if err := t.TW.Run(r.Context(), append([]string{id, "modify"}, args...)...); err != nil {
		t.Logger.Error("task modify failed", "err", err)
		if writeIfTaskParseError(w, err) {
			return
		}
		http.Error(w, "modify failed", http.StatusInternalServerError)
		return
	}
	// If the annotation input had text typed in, attach it as an annotation
	// after the modify lands. Mirrors clicking the panel's Add button but
	// covers the much-more-common case where a user types a note and then
	// clicks Save - the dedicated "Add" button is easy to miss when the
	// form has 10+ fields competing for attention.
	if note := strings.TrimSpace(r.FormValue("text")); note != "" {
		if err := t.TW.Annotate(r.Context(), id, note); err != nil {
			t.Logger.Error("inline annotation on modify failed", "err", err)
			// Don't fail the whole modify - the structured fields already
			// landed. Surface a soft error message instead.
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `<div class="rounded border border-amber-300 bg-amber-50 px-3 py-2 text-sm text-amber-900 dark:border-amber-900/60 dark:bg-amber-950/40 dark:text-amber-200" role="alert">Task saved, but the note couldn&apos;t be attached. Try the inline Add button.</div>`)
			return
		}
	}
	writeRefresh(w)
}

// idAction wraps the verbatim "validate id from path -> call tw -> 500 on
// error / refresh on success" shape that Done / Delete / Start / Stop /
// Duplicate all share. The action func receives the validated id and
// returns the underlying tw error (or nil). label is used for log lines
// and the user-facing error body so the response identifies which
// command failed without leaking internals.
func (t *Tasks) idAction(w http.ResponseWriter, r *http.Request, label string, action func(ctx context.Context, id string) error) {
	id := r.PathValue("id")
	if !tw.IDPattern.MatchString(id) {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	if err := action(r.Context(), id); err != nil {
		// Treat "0 tasks affected" exits as success: the user wanted the
		// task in the post-action state (deleted, done) and it already
		// is. A 500 here would mislead - the only thing wrong is that
		// the operation was a no-op. We log at debug for traceability
		// and refresh the list so the UI catches up to actual state.
		if tw.IsNoOpExit(err) {
			t.Logger.Debug("task "+label+" was no-op", "id", id)
			writeRefresh(w)
			return
		}
		t.Logger.Error("task "+label+" failed", "err", err)
		http.Error(w, label+" failed", http.StatusInternalServerError)
		return
	}
	writeRefresh(w)
}

func (t *Tasks) Done(w http.ResponseWriter, r *http.Request) {
	t.idAction(w, r, "done", func(ctx context.Context, id string) error {
		return t.TW.Run(ctx, id, "done")
	})
}

func (t *Tasks) Delete(w http.ResponseWriter, r *http.Request) {
	t.idAction(w, r, "delete", func(ctx context.Context, id string) error {
		// Cascade: deleting a recurring parent should kill the entire
		// series, not just the template row. TW leaves spawned children
		// orphaned (still status:pending) when only the parent is
		// deleted - that's contrary to the user-facing "Delete series"
		// affordance, which promises future occurrences will stop.
		// We look the task up first; if it's the parent template, we
		// delete every pending child via a parent:<uuid> filter before
		// deleting the parent itself.
		//
		// Race: between Export and the cascade Run, a CLI user could in
		// principle spawn a fresh child or modify a sibling's status. In
		// practice TW only spawns instances during a `task` invocation
		// and we run them sequentially here, so the only realistic loss
		// is a child created by a separate concurrent CLI call between
		// these two execs - that child stays orphaned and the user can
		// re-click Delete-series to clean it up. Best-effort is the
		// right contract here.
		//
		// If Export fails (TW briefly unavailable), we'd silently skip
		// the cascade and orphan ALL pending children - exactly what
		// this whole fix exists to prevent. Log at warn so flak shows up
		// in diagnostics, but still proceed to the parent delete so the
		// user-facing action isn't blocked on a transient.
		if tasks, err := t.TW.Export(ctx, id); err != nil {
			t.Logger.Warn("delete-series: pre-cascade lookup failed; skipping child cleanup", "id", id, "err", err)
		} else if len(tasks) == 1 && tasks[0].IsRecurringParent() {
			// "parent:<uuid>" matches only spawned children, never the
			// parent itself (the parent's `parent` field is empty), so
			// this is safe to run before the parent's own delete.
			// status:pending alone covers the visible cases; status:
			// waiting/completed/deleted children are left as-is so we
			// don't churn TW's archive.
			if cerr := t.TW.Run(ctx, "parent:"+id, "status:pending", "delete"); cerr != nil && !tw.IsNoOpExit(cerr) {
				t.Logger.Error("delete-series: cascade to children failed", "parent", id, "err", cerr)
			}
		}
		return t.TW.Run(ctx, id, "delete")
	})
}

// Start marks the task as actively being worked on (Taskwarrior records the
// timestamp on `start` and surfaces +ACTIVE). Idempotent: re-starting an
// already-active task is a no-op in Taskwarrior.
func (t *Tasks) Start(w http.ResponseWriter, r *http.Request) {
	t.idAction(w, r, "start", t.TW.Start)
}

// Stop clears the `start` timestamp set by Start. Idempotent.
func (t *Tasks) Stop(w http.ResponseWriter, r *http.Request) {
	t.idAction(w, r, "stop", t.TW.Stop)
}

// Duplicate clones a task's editable fields into a new pending task via
// `task <id> duplicate`. Recurrence is intentionally NOT carried across by
// Taskwarrior, which is exactly what we want for "create similar task" -
// the user can edit the new instance afterwards.
func (t *Tasks) Duplicate(w http.ResponseWriter, r *http.Request) {
	t.idAction(w, r, "duplicate", t.TW.Duplicate)
}

// intervalEditReq represents an edit to an existing on-disk interval.
// Identity (OriginalStart, OriginalEnd) names the pair as it currently
// exists in the task's journal annotations. OriginalEnd is nil when the
// pair is currently open (active task with only a Started annotation).
type intervalEditReq struct {
	OriginalStart time.Time  `json:"originalStart"`
	OriginalEnd   *time.Time `json:"originalEnd"`
	Start         time.Time  `json:"start"`
	End           *time.Time `json:"end"`
}

type intervalCreateReq struct {
	Start time.Time  `json:"start"`
	End   *time.Time `json:"end"`
}

type intervalDeleteReq struct {
	OriginalStart time.Time  `json:"originalStart"`
	OriginalEnd   *time.Time `json:"originalEnd"`
}

// Bound on the number of operations a single PUT may carry. The 200
// cap applies to the sum of edits+creates+deletes; deletes are cheap
// but counting them in the cap is fine - a sane FE never submits
// hundreds of any operation type in one click.
const maxIntervalsPerRequest = 200

// intervalErrorResp is the JSON body returned for any 4xx response
// from PutIntervals. The FE switches its rendering based on whether
// the Conflicts slice is populated: empty -> show plain error banner;
// non-empty -> also render the conflict panel above the sessions list.
type intervalErrorResp struct {
	Error     string         `json:"error"`
	Conflicts []conflictPair `json:"conflicts,omitempty"`
}

// conflictPair names two intervals that overlap each other in the
// post-diff state. The FE renders one editable mini-row per row in
// the pair; the user resolves the overlap by editing the times
// (which auto-classifies as an edit on next submit) or deleting one.
type conflictPair struct {
	Rows [2]conflictRow `json:"rows"`
}

// conflictRow carries everything the FE needs to render an editable
// row in the conflict panel AND wire it back to the submission
// pipeline:
//   - Kind / OriginalStart / OriginalEnd let the FE find the source
//     row (in the main list or the staging area) and hide it so the
//     conflict-panel duplicate becomes the single source of truth.
//   - CurrentStart / CurrentEnd pre-fill the panel's datetime-local
//     inputs so the user sees what they currently have.
//
// All timestamps are RFC3339 UTC. End fields are nullable to express
// open intervals.
type conflictRow struct {
	Kind          string  `json:"kind"`
	OriginalStart *string `json:"originalStart,omitempty"`
	OriginalEnd   *string `json:"originalEnd,omitempty"`
	CurrentStart  string  `json:"currentStart"`
	CurrentEnd    *string `json:"currentEnd,omitempty"`
}

func writeIntervalError(w http.ResponseWriter, status int, msg string, conflicts []conflictPair) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(intervalErrorResp{Error: msg, Conflicts: conflicts})
}

// httpStatusError lets the per-item validators return both a user-
// facing message and the HTTP status (some violations are 400-class -
// future timestamps - while others are 422-class - end before start).
type httpStatusError struct {
	msg    string
	status int
}

func (e *httpStatusError) Error() string { return e.msg }

// validateNewInterval performs the per-item shape checks shared by
// creates and edits: non-zero start, start not in future, end (if
// present) not before start AND not in future. Equal start/end is
// permitted - datetime-local inputs are minute-granular so a real
// sub-minute interval round-trips through the UI as start == end.
// The opKind argument ("create N" / "edit N") is prefixed into the
// error so the FE can map it back to a specific row.
func validateNewInterval(start time.Time, end *time.Time, now time.Time, opKind string) (time.Time, time.Time, *httpStatusError) {
	if start.IsZero() {
		return time.Time{}, time.Time{}, &httpStatusError{msg: opKind + ": missing start", status: http.StatusBadRequest}
	}
	startUTC := start.UTC()
	if startUTC.After(now) {
		return time.Time{}, time.Time{}, &httpStatusError{msg: opKind + ": start is in the future", status: http.StatusBadRequest}
	}
	if end == nil {
		return startUTC, time.Time{}, nil
	}
	endUTC := end.UTC()
	if endUTC.Before(startUTC) {
		return time.Time{}, time.Time{}, &httpStatusError{msg: opKind + ": end must not be earlier than start", status: http.StatusUnprocessableEntity}
	}
	if endUTC.After(now) {
		return time.Time{}, time.Time{}, &httpStatusError{msg: opKind + ": end is in the future", status: http.StatusBadRequest}
	}
	return startUTC, endUTC, nil
}

// PutIntervals handles PUT /tasks/{id}/intervals - the retroactive
// time-tracking editor. Body shape is a DIFF, not a snapshot:
//
//	{
//	  "edits":   [{"originalStart":"...","originalEnd":"..."|null,
//	              "start":"...","end":"..."|null}],
//	  "creates": [{"start":"...","end":"..."|null}],
//	  "deletes": [{"originalStart":"...","originalEnd":"..."|null}]
//	}
//
// This was a snapshot in the previous design ("intervals" array =
// the complete new state) which combined with FE pagination caused a
// data-loss bug: a save from a page that only had loaded the first
// 14 days silently wiped every older interval. The diff shape fixes
// that root cause - anything the FE doesn't mention is left alone.
//
// Handler responsibilities:
//   - Parse + per-item shape validation (no future, end>=start).
//   - Look up the task to determine .IsActive() (required to decide
//     whether an open interval is legal).
//   - Hand off to tw.Client.UpdateIntervals which applies the diff,
//     validates FINAL-state invariants (overlap, single-open), and
//     writes one atomic `task import`. Cross-state validation lives
//     there because only the diff-applied state has the full picture
//     when the FE may have submitted a partial view.
//
// Response: 204 + HX-Trigger refresh on success.
//
// CONCURRENCY: same race window as before. An out-of-band CLI
// invocation between this handler's Export and UpdateIntervals'
// Export can silently lose changes. The single-record `task import`
// is atomic per TW's docs; `task undo` rolls back the whole diff
// (which lands as one import op) cleanly.
func (t *Tasks) PutIntervals(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !tw.IDPattern.MatchString(id) {
		writeIntervalError(w, http.StatusBadRequest, "invalid id", nil)
		return
	}

	var body struct {
		Edits   []intervalEditReq   `json:"edits"`
		Creates []intervalCreateReq `json:"creates"`
		Deletes []intervalDeleteReq `json:"deletes"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil {
		writeIntervalError(w, http.StatusBadRequest, "bad request: "+err.Error(), nil)
		return
	}
	if dec.More() {
		writeIntervalError(w, http.StatusBadRequest, "bad request: unexpected trailing data after JSON body", nil)
		return
	}
	total := len(body.Edits) + len(body.Creates) + len(body.Deletes)
	if total > maxIntervalsPerRequest {
		writeIntervalError(w, http.StatusRequestEntityTooLarge, fmt.Sprintf("too many operations: %d (cap %d)", total, maxIntervalsPerRequest), nil)
		return
	}
	if total == 0 {
		// Empty diff is a no-op save - close the dialog.
		writeRefresh(w)
		return
	}

	now := time.Now().UTC()

	creates := make([]tw.IntervalCreate, 0, len(body.Creates))
	for i, c := range body.Creates {
		s, stop, err := validateNewInterval(c.Start, c.End, now, fmt.Sprintf("create %d", i))
		if err != nil {
			writeIntervalError(w, err.status, err.Error(), nil)
			return
		}
		creates = append(creates, tw.IntervalCreate{Start: s, Stop: stop})
	}
	edits := make([]tw.IntervalEdit, 0, len(body.Edits))
	for i, e := range body.Edits {
		if e.OriginalStart.IsZero() {
			writeIntervalError(w, http.StatusBadRequest, fmt.Sprintf("edit %d: missing original start", i), nil)
			return
		}
		s, stop, err := validateNewInterval(e.Start, e.End, now, fmt.Sprintf("edit %d", i))
		if err != nil {
			writeIntervalError(w, err.status, err.Error(), nil)
			return
		}
		origEnd := time.Time{}
		if e.OriginalEnd != nil {
			origEnd = e.OriginalEnd.UTC()
		}
		edits = append(edits, tw.IntervalEdit{
			OriginalStart: e.OriginalStart.UTC(),
			OriginalEnd:   origEnd,
			Start:         s,
			Stop:          stop,
		})
	}
	deletes := make([]tw.IntervalDelete, 0, len(body.Deletes))
	for i, d := range body.Deletes {
		if d.OriginalStart.IsZero() {
			writeIntervalError(w, http.StatusBadRequest, fmt.Sprintf("delete %d: missing original start", i), nil)
			return
		}
		origEnd := time.Time{}
		if d.OriginalEnd != nil {
			origEnd = d.OriginalEnd.UTC()
		}
		deletes = append(deletes, tw.IntervalDelete{
			OriginalStart: d.OriginalStart.UTC(),
			OriginalEnd:   origEnd,
		})
	}

	tasks, err := t.TW.Export(r.Context(), id)
	if err != nil {
		t.Logger.Error("intervals: pre-validate export failed", "id", id, "err", err)
		writeIntervalError(w, http.StatusInternalServerError, "task lookup failed", nil)
		return
	}
	if len(tasks) != 1 {
		writeIntervalError(w, http.StatusNotFound, "task not found", nil)
		return
	}
	taskActive := tasks[0].IsActive()

	plan := tw.PlanIntervalUpdate(tasks[0].Annotations, edits, creates, deletes)
	if conflicts := detectConflicts(plan, edits, creates); len(conflicts) > 0 {
		writeIntervalError(w, http.StatusUnprocessableEntity, "Overlapping time tracking period - please correct the highlighted entries.", conflicts)
		return
	}

	if err := t.TW.UpdateIntervals(r.Context(), id, edits, creates, deletes, taskActive); err != nil {
		switch {
		case errors.Is(err, tw.ErrIntervalOverlap):
			writeIntervalError(w, http.StatusUnprocessableEntity, "Overlapping time tracking period - please correct the highlighted entries.", nil)
			return
		case errors.Is(err, tw.ErrMultipleOpenIntervals):
			writeIntervalError(w, http.StatusUnprocessableEntity, "Only one entry can be left open at a time.", nil)
			return
		case errors.Is(err, tw.ErrOpenIntervalRequiresActive):
			writeIntervalError(w, http.StatusUnprocessableEntity, "An open entry requires the task to be active. Press Start tracking first.", nil)
			return
		case errors.Is(err, tw.ErrInvalid):
			writeIntervalError(w, http.StatusBadRequest, "bad request: "+err.Error(), nil)
			return
		}
		t.Logger.Error("intervals: update failed", "id", id, "err", err)
		writeIntervalError(w, http.StatusInternalServerError, "interval update failed", nil)
		return
	}
	writeRefresh(w)
}

// detectConflicts sweeps the post-diff plan and returns every
// overlapping pair as a structured ConflictPair. Each row carries
// the data the FE needs to render an editable mini-row in the
// conflict panel and to wire it back to the corresponding row in the
// main list / staging area.
//
// Sweep-line: each new interval is compared against every still-open
// prior interval - catches non-adjacent overlaps a simple sort-and-
// compare-neighbours pass would miss. Open intervals (zero Stop) use
// a far-future sentinel so a later closed interval intersecting them
// is detected.
func detectConflicts(plan []tw.IntervalPlanItem, edits []tw.IntervalEdit, creates []tw.IntervalCreate) []conflictPair {
	if len(plan) < 2 {
		return nil
	}
	farFuture := time.Date(9999, 12, 31, 0, 0, 0, 0, time.UTC)
	ordered := make([]tw.IntervalPlanItem, len(plan))
	copy(ordered, plan)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].Start.Before(ordered[j].Start) })
	var pairs []conflictPair
	type openItem struct {
		end  time.Time
		item tw.IntervalPlanItem
	}
	var open []openItem
	for _, curr := range ordered {
		stillOpen := open[:0]
		for _, o := range open {
			if curr.Start.Before(o.end) {
				pairs = append(pairs, conflictPair{
					Rows: [2]conflictRow{
						planItemToConflictRow(o.item, edits, creates),
						planItemToConflictRow(curr, edits, creates),
					},
				})
				stillOpen = append(stillOpen, o)
			}
		}
		open = stillOpen
		end := curr.Stop
		if end.IsZero() {
			end = farFuture
		}
		open = append(open, openItem{end: end, item: curr})
	}
	return pairs
}

func planItemToConflictRow(p tw.IntervalPlanItem, edits []tw.IntervalEdit, creates []tw.IntervalCreate) conflictRow {
	row := conflictRow{CurrentStart: p.Start.UTC().Format(time.RFC3339)}
	if !p.Stop.IsZero() {
		s := p.Stop.UTC().Format(time.RFC3339)
		row.CurrentEnd = &s
	}
	switch p.Origin.Kind {
	case tw.OriginCreate:
		row.Kind = "create"
	case tw.OriginEdit:
		row.Kind = "edit"
		if p.Origin.Index >= 0 && p.Origin.Index < len(edits) {
			e := edits[p.Origin.Index]
			os := e.OriginalStart.UTC().Format(time.RFC3339)
			row.OriginalStart = &os
			if !e.OriginalEnd.IsZero() {
				oe := e.OriginalEnd.UTC().Format(time.RFC3339)
				row.OriginalEnd = &oe
			}
		}
	default:
		row.Kind = "existing"
		os := p.Start.UTC().Format(time.RFC3339)
		row.OriginalStart = &os
		if !p.Stop.IsZero() {
			oe := p.Stop.UTC().Format(time.RFC3339)
			row.OriginalEnd = &oe
		}
	}
	return row
}

// maxBulkIDs caps the number of ids accepted in a single bulk request. The
// handler runs `task <id> done|delete` sequentially; capping at 100 bounds
// subprocess fan-out.
const maxBulkIDs = 100

// Annotate appends an annotation. Re-fetches the task and renders the
// AnnotationsPanel partial so the modal updates in-place - deliberately NO
// HX-Trigger: refresh, because that would close the modal.
func (t *Tasks) Annotate(w http.ResponseWriter, r *http.Request) {
	t.annotateOrDenotate(w, r, true)
}

// Denotate removes the annotation matching `text`. Same partial-swap pattern.
func (t *Tasks) Denotate(w http.ResponseWriter, r *http.Request) {
	t.annotateOrDenotate(w, r, false)
}

func (t *Tasks) annotateOrDenotate(w http.ResponseWriter, r *http.Request, annotate bool) {
	id := r.PathValue("id")
	if !tw.IDPattern.MatchString(id) {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	text := strings.TrimSpace(r.FormValue("text"))
	if text == "" {
		http.Error(w, "annotation text required", http.StatusBadRequest)
		return
	}
	var err error
	if annotate {
		err = t.TW.Annotate(r.Context(), id, text)
	} else {
		err = t.TW.Denotate(r.Context(), id, text)
	}
	if err != nil {
		t.Logger.Error("annotation op failed", "annotate", annotate, "err", err)
		http.Error(w, "task command failed", http.StatusInternalServerError)
		return
	}
	tasks, err := t.TW.Export(r.Context(), id)
	if err != nil {
		t.Logger.Error("export after annotation op failed", "id", id, "err", err)
		http.Error(w, "task export failed", http.StatusInternalServerError)
		return
	}
	if len(tasks) == 0 {
		http.Error(w, "task not found after update", http.StatusInternalServerError)
		return
	}
	renderHTML(w, r, "AnnotationsPanel", views.AnnotationsPanel(tasks[0]), t.Logger)
}

// BulkDone marks every supplied task id as done. Failures on individual ids
// are logged but do not abort the batch; the user sees the result via the
// HX-Trigger refresh.
func (t *Tasks) BulkDone(w http.ResponseWriter, r *http.Request) {
	t.bulk(w, r, "done")
}

// BulkDelete deletes every supplied task id. Same failure semantics as
// BulkDone.
func (t *Tasks) BulkDelete(w http.ResponseWriter, r *http.Request) {
	t.bulk(w, r, "delete")
}

func (t *Tasks) bulk(w http.ResponseWriter, r *http.Request, action string) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	raw := r.FormValue("bulk-ids")
	ids := splitCSV(raw)
	if len(ids) == 0 {
		http.Error(w, "no ids supplied", http.StatusBadRequest)
		return
	}
	if len(ids) > maxBulkIDs {
		http.Error(w, "too many ids (max 100)", http.StatusBadRequest)
		return
	}
	for _, id := range ids {
		if !tw.IDPattern.MatchString(id) {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), bulkTimeout)
	defer cancel()

	failures := 0
	completed := 0
	timedOut := false
	for _, id := range ids {
		if err := t.TW.Run(ctx, id, action); err != nil {
			failures++
			t.Logger.Error("bulk task action failed",
				"action", action, "id", id, "err", err)
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				timedOut = true
				break
			}
			continue
		}
		completed++
	}
	if timedOut {
		t.Logger.Warn("bulk task action timed out",
			"action", action, "total", len(ids),
			"completed", completed, "failures", failures)
		http.Error(w, "bulk action timed out", http.StatusGatewayTimeout)
		return
	}
	if failures > 0 {
		t.Logger.Warn("bulk task action completed with failures",
			"action", action, "total", len(ids), "failures", failures)
	}
	writeRefresh(w)
}

// splitCSV parses a comma-separated string, trimming whitespace and dropping
// empty entries. Used for both bulk-action id lists and the tags form field.
func splitCSV(s string) []string {
	out := make([]string, 0)
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func writeRefresh(w http.ResponseWriter) {
	w.Header().Set("HX-Trigger", "refresh")
	w.WriteHeader(http.StatusNoContent)
}

// udaDatePattern is the strict subset accepted for date-typed UDAs. A native
// date <input type="date"> emits YYYY-MM-DD, but the user can also paste
// freeform Taskwarrior keywords ("tomorrow", "due-3d") so we accept the same
// alphanumeric/colon/+/-/_ alphabet the built-in date fields use.
var udaDatePattern = regexp.MustCompile(`^[a-zA-Z0-9:_+\-]+$`)

// readUDAArgs collects UDA values from the request form and returns them as
// `<name>:"<value>"` argv fragments. When isModify is true, an empty value
// becomes a bare `<name>:` clear-arg (mirroring the date-clear behaviour in
// AddInput.ModifyArgs); on create, empty values are skipped so the UDA stays
// unset on the new task.
//
// Per-type validation is intentionally light:
//   - numeric: must parse as float64.
//   - date: must match udaDatePattern.
//   - duration / string / unknown: passed through (Taskwarrior re-parses).
//
// The UDA name itself is re-checked against UDANamePattern on every call as
// defence-in-depth: ListUDAs already filters, but a future refactor that
// bypasses the cache shouldn't be able to inject parser tokens.
func readUDAArgs(r *http.Request, udas []tw.UDA, isModify bool) ([]string, error) {
	if len(udas) == 0 {
		return nil, nil
	}
	args := make([]string, 0, len(udas))
	for _, u := range udas {
		if !tw.UDANamePattern.MatchString(u.Name) {
			continue
		}
		val := strings.TrimSpace(r.FormValue("uda_" + u.Name))
		if val == "" {
			if isModify {
				args = append(args, u.Name+":")
			}
			continue
		}
		// Closed enum: if the UDA defines a values allowlist, the submitted
		// value must be one of them. Empty was already handled above.
		if len(u.Values) > 0 {
			if !udaValueAllowed(val, u.Values) {
				return nil, &udaInputError{name: u.Name, kind: "enum"}
			}
		} else {
			switch u.Type {
			case "numeric":
				if _, err := strconv.ParseFloat(val, 64); err != nil {
					return nil, &udaInputError{name: u.Name, kind: "numeric"}
				}
			case "date":
				if !udaDatePattern.MatchString(val) {
					return nil, &udaInputError{name: u.Name, kind: "date"}
				}
			}
		}
		args = append(args, u.Name+":"+tw.QuoteArg(val))
	}
	return args, nil
}

// udaInputError is the error type returned by readUDAArgs for a per-field
// validation failure. The handlers surface its message verbatim as the 400
// response body so the user can see which UDA was rejected.
type udaInputError struct {
	name string
	kind string
}

func (e *udaInputError) Error() string {
	return "invalid " + e.kind + " value for " + e.name
}

// udaValueAllowed reports whether v is in the closed enum allowlist.
// Comparison is case-sensitive - Taskwarrior's own value comparison is
// case-sensitive too, so this matches its semantics.
func udaValueAllowed(v string, allowed []string) bool {
	for _, a := range allowed {
		if a == v {
			return true
		}
	}
	return false
}

func readAddInput(r *http.Request) tw.AddInput {
	return tw.AddInput{
		Description: r.FormValue("description"),
		Project:     r.FormValue("project"),
		Tags:        splitCSV(r.FormValue("tags")),
		Due:         r.FormValue("due"),
		Wait:        r.FormValue("wait"),
		Scheduled:   r.FormValue("scheduled"),
		Recur:       r.FormValue("recur"),
		Until:       r.FormValue("until"),
		Depends:     splitCSV(r.FormValue("depends")),
	}
}

// stringSet builds a presence-only set from a slice. Uses struct{} values
// (zero bytes) instead of bool to make the intent explicit: callers only
// ever check `_, ok := set[k]`, never read the value.
func stringSet(items []string) map[string]struct{} {
	m := make(map[string]struct{}, len(items))
	for _, s := range items {
		m[s] = struct{}{}
	}
	return m
}

// Undo runs `task undo`, reversing Taskwarrior's most recent change. The
// safetyArgs prefix on every Client invocation already neutralises the
// interactive confirmation prompt, so the call returns as soon as the undo
// commits. The 204 + HX-Trigger: refresh response refreshes the list view.
func (t *Tasks) Undo(w http.ResponseWriter, r *http.Request) {
	if err := t.TW.Undo(r.Context()); err != nil {
		t.Logger.Error("task undo failed", "err", err)
		http.Error(w, "undo failed", http.StatusInternalServerError)
		return
	}
	writeRefresh(w)
}

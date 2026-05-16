package handlers

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/furan917/taskwarrior-web-portal/internal/config"
	"github.com/furan917/taskwarrior-web-portal/internal/tw"
	"github.com/furan917/taskwarrior-web-portal/internal/views"
)

// activeContext returns the current Taskwarrior context name for this
// request, or "" if none is set / lookup failed. Called per-request: the
// state lives in ~/.taskrc and can be flipped out-of-band, so caching would
// produce a stale UI. Bounded by config.ActiveContextTimeout so a wedged
// binary can't stall page rendering.
func activeContext(c *tw.Client, r *http.Request) string {
	ctx, cancel := context.WithTimeout(r.Context(), config.ActiveContextTimeout)
	defer cancel()
	return c.ActiveContext(ctx)
}

// activeContextHasUnsafeWriteFilter reports whether the currently active
// context has a write filter that Taskwarrior cannot safely apply as a
// modification (i.e. it contains logical operators or rc.* overrides). Used
// by the task Create handler to decide whether to bypass native write-filter
// application and rely solely on the form's pre-filled values instead.
func activeContextHasUnsafeWriteFilter(c *tw.Client, r *http.Request) bool {
	name := activeContext(c, r)
	if name == "" {
		return false
	}
	for _, ctx := range c.ContextsCached(r.Context()) {
		if ctx.Name == name {
			// Taskwarrior falls back to the read filter as the write filter
			// when no write filter is explicitly set, so we must check the
			// effective filter, not just WriteFilter.
			effective := ctx.WriteFilter
			if effective == "" {
				effective = ctx.ReadFilter
			}
			return tw.FilterContainsLogicalOperator(effective) || tw.FilterContainsNegation(effective)
		}
	}
	return false
}

// namedContextsForRender converts the cached []tw.Context list into the
// flat shape the views envelope wants. Active is recomputed against the
// per-request active name so the dropdown's checkmark always tracks the
// freshly-read state, not whatever was true at first-cache time.
func namedContextsForRender(c *tw.Client, r *http.Request, active string) []views.NamedContext {
	cached := c.ContextsCached(r.Context())
	if len(cached) == 0 {
		return nil
	}
	out := make([]views.NamedContext, 0, len(cached))
	for _, ctxItem := range cached {
		out = append(out, views.NamedContext{
			Name:   ctxItem.Name,
			Active: ctxItem.Name == active,
		})
	}
	return out
}

// Contexts holds dependencies for the SetContext write-side handler. Kept on
// its own struct (rather than bolted onto Tasks or Views) so the routing in
// registerRoutes is one new line.
type Contexts struct {
	TW     *tw.Client
	Logger *slog.Logger
}

// Set handles POST /context with a single form field `name=`. Empty value
// clears the active context (`task context none`); a non-empty name must
// match ContextNamePattern and is forwarded to `task context <name>`.
//
// Response: 204 + HX-Refresh: true so the htmx-enabled client reloads the
// page from scratch. The pill, title hint, empty-state copy and modal
// subtitle all derive from the request-time active context, so a full reload
// is the simplest way to keep them in sync.
func (c *Contexts) Set(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	name := r.FormValue("name")
	if name != "" && !tw.ContextNamePattern.MatchString(name) {
		http.Error(w, "invalid context name", http.StatusBadRequest)
		return
	}
	if err := c.TW.SetContext(r.Context(), name); err != nil {
		c.Logger.Error("set context failed", "name", name, "err", err)
		http.Error(w, "set context failed", http.StatusInternalServerError)
		return
	}
	// HX-Refresh fires a full-page navigation in the htmx client; the pill
	// / title / empty-states all re-render server-side from the new active
	// state. HX-Trigger: refresh would only re-poll the task list and leave
	// the nav stale.
	w.Header().Set("HX-Refresh", "true")
	w.WriteHeader(http.StatusNoContent)
}

// ManageContexts handles GET /contexts - lists all defined contexts with edit/delete actions.
func (c *Contexts) ManageContexts(w http.ResponseWriter, r *http.Request) {
	contexts := c.TW.ContextsCached(r.Context())
	// Use the shared buildPage so the More dropdown (BuiltinReports +
	// CustomReports) populates consistently with every other page; the
	// /contexts route is a read-only management surface so hasTaskList=false.
	// page.ActiveContext is the same string activeContext() returns, so
	// reuse it for the template's per-row active marker rather than
	// invoking the resolver a second time.
	page := buildPage(c.TW, r, "Contexts", "contexts", false)
	renderHTML(w, r, "Contexts", views.ManageContextsPage(page, contexts, page.ActiveContext), c.Logger)
}

// CreateContextForm handles GET /forms/context/new - renders the empty create modal.
func (c *Contexts) CreateContextForm(w http.ResponseWriter, r *http.Request) {
	csrf := csrfToken(r)
	renderHTML(w, r, "ContextForm", views.ContextFormModal(csrf, "", "", "", true), c.Logger)
}

// EditContextForm handles GET /forms/context/{name} - renders the pre-filled edit modal.
func (c *Contexts) EditContextForm(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !tw.ContextNamePattern.MatchString(name) {
		http.Error(w, "invalid context name", http.StatusBadRequest)
		return
	}
	var found tw.Context
	for _, c2 := range c.TW.ContextsCached(r.Context()) {
		if c2.Name == name {
			found = c2
			break
		}
	}
	csrf := csrfToken(r)
	renderHTML(w, r, "ContextForm", views.ContextFormModal(csrf, found.Name, found.ReadFilter, found.WriteFilter, false), c.Logger)
}

// CreateContext handles POST /contexts - creates a new context.
func (c *Contexts) CreateContext(w http.ResponseWriter, r *http.Request) {
	name, readFilter, writeFilter, ok := parseContextForm(w, r)
	if !ok {
		return
	}
	if err := c.TW.DefineContext(r.Context(), name, readFilter); err != nil {
		c.contextFormError(w, "create", err)
		return
	}
	if writeFilter != "" {
		if err := c.TW.SetContextWriteFilter(r.Context(), name, writeFilter); err != nil {
			c.contextFormError(w, "create", err)
			return
		}
	}
	w.Header().Set("HX-Refresh", "true")
	w.WriteHeader(http.StatusNoContent)
}

// UpdateContext handles PUT /contexts/{name} - updates (and optionally renames) a context.
func (c *Contexts) UpdateContext(w http.ResponseWriter, r *http.Request) {
	oldName := r.PathValue("name")
	if !tw.ContextNamePattern.MatchString(oldName) {
		http.Error(w, "invalid context name", http.StatusBadRequest)
		return
	}
	newName, readFilter, writeFilter, ok := parseContextForm(w, r)
	if !ok {
		return
	}
	if err := c.TW.RenameContext(r.Context(), oldName, newName, readFilter, writeFilter); err != nil {
		c.contextFormError(w, "update", err)
		return
	}
	w.Header().Set("HX-Refresh", "true")
	w.WriteHeader(http.StatusNoContent)
}

// DeleteContext handles DELETE /contexts/{name} - removes a context.
func (c *Contexts) DeleteContext(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !tw.ContextNamePattern.MatchString(name) {
		http.Error(w, "invalid context name", http.StatusBadRequest)
		return
	}
	if err := c.TW.DeleteContext(r.Context(), name); err != nil {
		c.Logger.Error("delete context failed", "name", name, "err", err)
		http.Error(w, "delete context failed", http.StatusInternalServerError)
		return
	}
	w.Header().Set("HX-Refresh", "true")
	w.WriteHeader(http.StatusNoContent)
}

func parseContextForm(w http.ResponseWriter, r *http.Request) (name, readFilter, writeFilter string, ok bool) {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	if err := r.ParseForm(); err != nil {
		writeContextFormError(w, http.StatusBadRequest, "bad form data")
		return "", "", "", false
	}
	name = strings.TrimSpace(r.FormValue("name"))
	readFilter = strings.TrimSpace(r.FormValue("read_filter"))
	writeFilter = strings.TrimSpace(r.FormValue("write_filter"))
	if len(readFilter) > 1024 || len(writeFilter) > 1024 {
		writeContextFormError(w, http.StatusBadRequest, "filter expression too long (max 1024 characters)")
		return "", "", "", false
	}
	if !tw.ContextNamePattern.MatchString(name) {
		writeContextFormError(w, http.StatusBadRequest, "invalid context name: letters, digits, dash and underscore only")
		return "", "", "", false
	}
	if readFilter == "" {
		writeContextFormError(w, http.StatusBadRequest, "read filter is required")
		return "", "", "", false
	}
	if tw.FilterContainsRcOverride(readFilter) {
		writeContextFormError(w, http.StatusBadRequest, "read filter must not contain rc.* overrides")
		return "", "", "", false
	}
	if writeFilter != "" && tw.FilterContainsRcOverride(writeFilter) {
		writeContextFormError(w, http.StatusBadRequest, "write filter must not contain rc.* overrides")
		return "", "", "", false
	}
	if writeFilter != "" && tw.FilterContainsLogicalOperator(writeFilter) {
		writeContextFormError(w, http.StatusBadRequest, "The write filter must be a simple expression. Logical operators (or, and, not) and parentheses are not supported.")
		return "", "", "", false
	}
	if writeFilter != "" && tw.FilterContainsNegation(writeFilter) {
		writeContextFormError(w, http.StatusBadRequest, "The write filter must not contain negation tokens (e.g. -tag). Only additive expressions like +tag or project:name are supported.")
		return "", "", "", false
	}
	return name, readFilter, writeFilter, true
}

// contextFormError renders the per-form error body the HX-target picks up.
// op is a short verb ("create", "update", "delete") so the user sees a
// context-specific message ("delete context failed") rather than the
// generic "operation failed" the previous shape produced - matching the
// surrounding handler messages (e.g. Set's "set context failed").
func (c *Contexts) contextFormError(w http.ResponseWriter, op string, err error) {
	c.Logger.Error(op+" context failed", "err", err)
	if errors.Is(err, tw.ErrInvalid) {
		writeContextFormError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeContextFormError(w, http.StatusInternalServerError, op+" context failed")
}

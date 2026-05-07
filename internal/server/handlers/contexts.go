package handlers

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/furan917/taskwarrior-web/internal/config"
	"github.com/furan917/taskwarrior-web/internal/tw"
	"github.com/furan917/taskwarrior-web/internal/views"
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

package handlers

import (
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/furan917/taskwarrior-web-portal/internal/tw"
	"github.com/furan917/taskwarrior-web-portal/internal/views"
)

// Sync holds the sync handler and its in-memory last-result state.
type Sync struct {
	TW     *tw.Client
	Logger *slog.Logger

	mu     sync.Mutex
	result *views.SyncResult
}

// Run handles POST /sync. Runs `task sync`, stores the result, and returns
// the sync result partial for HTMX to swap in.
func (s *Sync) Run(w http.ResponseWriter, r *http.Request) {
	ranAt := time.Now().UTC().Format("2006-01-02 15:04:05 UTC")
	output, err := s.TW.Sync(r.Context())

	res := &views.SyncResult{
		Output: fmt.Sprintf("Synced at %s\n%s", ranAt, output),
		OK:     err == nil,
	}
	if err != nil && output == "" {
		res.Output = fmt.Sprintf("Synced at %s\n%s", ranAt, err.Error())
	}

	s.mu.Lock()
	s.result = res
	s.mu.Unlock()

	if err != nil && s.Logger != nil {
		s.Logger.Warn("task sync failed", "err", err, "output", output)
	}

	renderHTML(w, r, "SyncResult", views.SyncResultPartial(res), s.Logger)
}

// Result returns the current in-memory sync result (nil if never run).
func (s *Sync) Result() *views.SyncResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.result
}

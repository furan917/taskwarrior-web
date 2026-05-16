package handlers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/furan917/taskwarrior-web-portal/internal/tw"
	"github.com/furan917/taskwarrior-web-portal/internal/views"
)

func (v *Views) TimeReport(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	now := time.Now()

	days := 30
	if d := r.URL.Query().Get("days"); d != "" {
		if n, err := strconv.Atoi(d); err == nil {
			switch n {
			case 7, 14, 30, 90:
				days = n
			}
		}
	}

	page := v.buildPage(r, "Time report", "time-report", false)

	if !v.TW.JournalTimeEnabled(ctx) {
		renderHTML(w, r, "Time report", views.TimeReportPage(page, false, views.TimeReportData{}), v.Logger)
		return
	}

	from := views.AddDays(views.DateOnly(now), -days)
	tasks, err := v.exportWithContext(r,
		"(status:pending or status:waiting or status:completed)",
		"modified.after:"+from.UTC().Format("20060102T150405Z"),
	)
	if err != nil {
		v.Logger.Error("time-report fetch failed", "err", err)
		http.Error(w, "fetch failed", http.StatusInternalServerError)
		return
	}

	sessions := tw.SessionsInRange(tasks, from, views.AddDays(views.DateOnly(now), 1), now)

	projects := map[string]struct{}{}
	for _, s := range sessions {
		if s.Project != "" {
			projects[s.Project] = struct{}{}
		}
	}

	data := views.TimeReportData{
		Days:          days,
		SessionCount:  len(sessions),
		ProjectCount:  len(projects),
		ProjectTotals: buildProjectTimeTree(sessions, now),
		TimeHistory:   computeTimeHistory(tasks, days, now),
	}
	for _, s := range sessions {
		data.TotalMinutes += int(s.Duration(now).Minutes())
	}
	if len(sessions) > 0 {
		data.AvgSessionMinutes = data.TotalMinutes / len(sessions)
	}

	renderHTML(w, r, "Time report", views.TimeReportPage(page, true, data), v.Logger)
}

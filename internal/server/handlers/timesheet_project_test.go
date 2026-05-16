package handlers

import (
	"testing"
	"time"

	"github.com/furan917/taskwarrior-web-portal/internal/tw"
	"github.com/furan917/taskwarrior-web-portal/internal/views"
)

// projectSession is a thin helper that sets a project on the shared session
// helper already defined in timesheet_test.go.
func projectSession(start time.Time, dur time.Duration, uuid, proj string) tw.Session {
	return tw.Session{
		TaskUUID: uuid,
		Project:  proj,
		Start:    start,
		Stop:     start.Add(dur),
	}
}

var testNow = time.Date(2026, 5, 12, 13, 0, 0, 0, time.Local)

func TestBuildProjectTimeTree_NilWhenSingleProject(t *testing.T) {
	sessions := []tw.Session{
		projectSession(testNow, 1*time.Hour, "u1", "work.engineering"),
		projectSession(testNow, 30*time.Minute, "u2", "work.engineering"),
	}
	if got := buildProjectTimeTree(sessions, testNow); got != nil {
		t.Errorf("single distinct project: want nil, got %v", got)
	}
}

func TestBuildProjectTimeTree_NilWhenEmpty(t *testing.T) {
	if got := buildProjectTimeTree(nil, testNow); got != nil {
		t.Errorf("empty sessions: want nil, got %v", got)
	}
}

func TestBuildProjectTimeTree_FlatTwoProjects(t *testing.T) {
	sessions := []tw.Session{
		projectSession(testNow, 2*time.Hour, "u1", "backend"),
		projectSession(testNow, 1*time.Hour, "u2", "frontend"),
	}
	nodes := buildProjectTimeTree(sessions, testNow)
	if len(nodes) != 2 {
		t.Fatalf("want 2 root nodes, got %d", len(nodes))
	}
	// Sorted by SubtreeTotal desc: backend (2h) before frontend (1h).
	if nodes[0].FullName != "backend" {
		t.Errorf("nodes[0].FullName = %q, want backend", nodes[0].FullName)
	}
	if nodes[0].SubtreeTotal != 2*time.Hour {
		t.Errorf("nodes[0].SubtreeTotal = %v, want 2h", nodes[0].SubtreeTotal)
	}
	if nodes[1].FullName != "frontend" {
		t.Errorf("nodes[1].FullName = %q, want frontend", nodes[1].FullName)
	}
}

func TestBuildProjectTimeTree_HierarchyRollsUp(t *testing.T) {
	sessions := []tw.Session{
		projectSession(testNow, 3*time.Hour, "u1", "work.engineering.backend"),
		projectSession(testNow, 2*time.Hour, "u2", "work.engineering.frontend"),
		projectSession(testNow, 1*time.Hour, "u3", "work.product"),
	}
	nodes := buildProjectTimeTree(sessions, testNow)

	// Single root: "work"
	if len(nodes) != 1 {
		t.Fatalf("want 1 root node, got %d", len(nodes))
	}
	root := nodes[0]
	if root.FullName != "work" {
		t.Errorf("root.FullName = %q, want work", root.FullName)
	}
	// work subtotal = 3h + 2h + 1h = 6h
	if root.SubtreeTotal != 6*time.Hour {
		t.Errorf("root.SubtreeTotal = %v, want 6h", root.SubtreeTotal)
	}

	// work has one child: engineering and product
	if len(root.Children) != 2 {
		t.Fatalf("root.Children: want 2, got %d", len(root.Children))
	}
	eng := root.Children[0] // sorted desc: engineering (5h) before product (1h)
	if eng.FullName != "work.engineering" {
		t.Errorf("Children[0].FullName = %q, want work.engineering", eng.FullName)
	}
	if eng.SubtreeTotal != 5*time.Hour {
		t.Errorf("work.engineering SubtreeTotal = %v, want 5h", eng.SubtreeTotal)
	}
	if len(eng.Children) != 2 {
		t.Errorf("work.engineering should have 2 children (backend, frontend), got %d", len(eng.Children))
	}

	prod := root.Children[1]
	if prod.FullName != "work.product" {
		t.Errorf("Children[1].FullName = %q, want work.product", prod.FullName)
	}
	if prod.SubtreeTotal != 1*time.Hour {
		t.Errorf("work.product SubtreeTotal = %v, want 1h", prod.SubtreeTotal)
	}
}

func TestBuildProjectTimeTree_NoProjectBucket(t *testing.T) {
	sessions := []tw.Session{
		projectSession(testNow, 30*time.Minute, "u1", ""),
		projectSession(testNow, 1*time.Hour, "u2", "backend"),
	}
	nodes := buildProjectTimeTree(sessions, testNow)
	if len(nodes) != 2 {
		t.Fatalf("want 2 root nodes, got %d", len(nodes))
	}
	// backend (1h) sorts before "(no project)" (30m) by duration desc.
	if nodes[0].FullName != "backend" {
		t.Errorf("nodes[0].FullName = %q, want backend", nodes[0].FullName)
	}
	noProj := nodes[1]
	if noProj.FullName != "" {
		t.Errorf("no-project node FullName = %q, want empty string", noProj.FullName)
	}
	if noProj.Segment != "(no project)" {
		t.Errorf("no-project Segment = %q, want (no project)", noProj.Segment)
	}
	if noProj.SubtreeTotal != 30*time.Minute {
		t.Errorf("no-project SubtreeTotal = %v, want 30m", noProj.SubtreeTotal)
	}
}

func TestBuildProjectTimeTree_ChildrenSortedByDurationDesc(t *testing.T) {
	sessions := []tw.Session{
		projectSession(testNow, 1*time.Hour, "u1", "work.engineering.frontend"),
		projectSession(testNow, 3*time.Hour, "u2", "work.engineering.backend"),
		projectSession(testNow, 2*time.Hour, "u3", "work.engineering.devops"),
	}
	nodes := buildProjectTimeTree(sessions, testNow)
	if len(nodes) != 1 {
		t.Fatalf("want 1 root, got %d", len(nodes))
	}
	eng := nodes[0].Children[0] // work -> engineering
	if len(eng.Children) != 3 {
		t.Fatalf("engineering should have 3 children, got %d", len(eng.Children))
	}
	if eng.Children[0].FullName != "work.engineering.backend" {
		t.Errorf("children[0] = %q, want backend (3h)", eng.Children[0].FullName)
	}
	if eng.Children[1].FullName != "work.engineering.devops" {
		t.Errorf("children[1] = %q, want devops (2h)", eng.Children[1].FullName)
	}
	if eng.Children[2].FullName != "work.engineering.frontend" {
		t.Errorf("children[2] = %q, want frontend (1h)", eng.Children[2].FullName)
	}
}

func TestBuildTimesheetPage_ProjectTotalsNilForSingleProject(t *testing.T) {
	now := fixedTime()
	anchor := time.Date(2026, 5, 12, 0, 0, 0, 0, time.Local)
	sessions := []tw.Session{
		projectSession(time.Date(2026, 5, 12, 9, 0, 0, 0, time.Local), 1*time.Hour, "u1", "work"),
		projectSession(time.Date(2026, 5, 12, 11, 0, 0, 0, time.Local), 30*time.Minute, "u2", "work"),
	}
	data := BuildTimesheetPage(views.TimesheetViewWeek, anchor, sessions, now)
	if data.ProjectTotals != nil {
		t.Errorf("single project: ProjectTotals should be nil, got %v", data.ProjectTotals)
	}
}

func TestBuildTimesheetPage_ProjectTotalsPopulatedForMultipleProjects(t *testing.T) {
	now := fixedTime()
	anchor := time.Date(2026, 5, 12, 0, 0, 0, 0, time.Local)
	sessions := []tw.Session{
		projectSession(time.Date(2026, 5, 12, 9, 0, 0, 0, time.Local), 2*time.Hour, "u1", "backend"),
		projectSession(time.Date(2026, 5, 12, 11, 0, 0, 0, time.Local), 1*time.Hour, "u2", "frontend"),
	}
	data := BuildTimesheetPage(views.TimesheetViewWeek, anchor, sessions, now)
	if len(data.ProjectTotals) == 0 {
		t.Errorf("multiple projects: ProjectTotals should be populated")
	}
	if data.ProjectTotals[0].FullName != "backend" {
		t.Errorf("top project = %q, want backend", data.ProjectTotals[0].FullName)
	}
}

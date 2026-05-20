package views

import (
	"testing"

	"github.com/furan917/taskwarrior-web-portal/internal/tw"
)

// task helpers

func taskWithTags(tags ...string) tw.Task {
	return tw.Task{
		UUID:        "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
		Description: "test task",
		Status:      "pending",
		Entry:       "20260501T120000Z",
		Tags:        tags,
	}
}

func activeTask(tags ...string) tw.Task {
	t := taskWithTags(tags...)
	t.Start = "20260519T080000Z"
	return t
}

// KanbanColumnFor

func TestKanbanColumnFor_Backlog(t *testing.T) {
	if got := KanbanColumnFor(taskWithTags()); got != "backlog" {
		t.Errorf("no tags: got %q want backlog", got)
	}
}

func TestKanbanColumnFor_Inbox(t *testing.T) {
	if got := KanbanColumnFor(taskWithTags("inbox")); got != "inbox" {
		t.Errorf("got %q want inbox", got)
	}
}

func TestKanbanColumnFor_InProgressByTag(t *testing.T) {
	if got := KanbanColumnFor(taskWithTags("inprogress")); got != "inprogress" {
		t.Errorf("got %q want inprogress", got)
	}
}

func TestKanbanColumnFor_InProgressByActive(t *testing.T) {
	if got := KanbanColumnFor(activeTask()); got != "inprogress" {
		t.Errorf("active task: got %q want inprogress", got)
	}
}

func TestKanbanColumnFor_OnHold(t *testing.T) {
	if got := KanbanColumnFor(taskWithTags("onhold")); got != "onhold" {
		t.Errorf("got %q want onhold", got)
	}
}

func TestKanbanColumnFor_OnHoldBeatsActive(t *testing.T) {
	// +onhold must take precedence over IsActive so a task that was started
	// but then put on hold stays in the right column.
	if got := KanbanColumnFor(activeTask("onhold")); got != "onhold" {
		t.Errorf("onhold+active: got %q want onhold", got)
	}
}

func TestKanbanColumnFor_ActiveBeatsInboxTag(t *testing.T) {
	// An active task with +inbox is in progress (was started), not inbox.
	if got := KanbanColumnFor(activeTask("inbox")); got != "inprogress" {
		t.Errorf("active+inbox: got %q want inprogress", got)
	}
}

// CapKanbanCol

func makeTasks(n int) []tw.Task {
	tasks := make([]tw.Task, n)
	for i := range tasks {
		tasks[i] = tw.Task{Description: "task", Status: "pending"}
	}
	return tasks
}

func TestCapKanbanCol_Empty(t *testing.T) {
	col := CapKanbanCol(nil)
	if col.Total != 0 || len(col.Tasks) != 0 || col.More() != 0 {
		t.Errorf("empty: got total=%d tasks=%d more=%d", col.Total, len(col.Tasks), col.More())
	}
}

func TestCapKanbanCol_UnderCap(t *testing.T) {
	tasks := makeTasks(KanbanColCap - 1)
	col := CapKanbanCol(tasks)
	if col.Total != KanbanColCap-1 {
		t.Errorf("total: got %d want %d", col.Total, KanbanColCap-1)
	}
	if len(col.Tasks) != KanbanColCap-1 {
		t.Errorf("tasks len: got %d want %d", len(col.Tasks), KanbanColCap-1)
	}
	if col.More() != 0 {
		t.Errorf("more: got %d want 0", col.More())
	}
}

func TestCapKanbanCol_AtCap(t *testing.T) {
	tasks := makeTasks(KanbanColCap)
	col := CapKanbanCol(tasks)
	if col.Total != KanbanColCap || len(col.Tasks) != KanbanColCap || col.More() != 0 {
		t.Errorf("at cap: total=%d tasks=%d more=%d", col.Total, len(col.Tasks), col.More())
	}
}

func TestCapKanbanCol_OverCap(t *testing.T) {
	extra := 7
	tasks := makeTasks(KanbanColCap + extra)
	col := CapKanbanCol(tasks)
	if col.Total != KanbanColCap+extra {
		t.Errorf("total: got %d want %d", col.Total, KanbanColCap+extra)
	}
	if len(col.Tasks) != KanbanColCap {
		t.Errorf("tasks len: got %d want %d", len(col.Tasks), KanbanColCap)
	}
	if col.More() != extra {
		t.Errorf("more: got %d want %d", col.More(), extra)
	}
}

package views

import (
	"testing"
)

func TestBuildProjectTree_Empty(t *testing.T) {
	if got := BuildProjectTree(nil); got != nil {
		t.Errorf("expected nil for empty input, got %v", got)
	}
}

func TestBuildProjectTree_FlatNoHierarchy(t *testing.T) {
	input := []ProjectInput{
		{Name: "alpha", Pending: 3},
		{Name: "beta", Pending: 1},
	}
	roots := BuildProjectTree(input)
	if len(roots) != 2 {
		t.Fatalf("expected 2 roots, got %d: %+v", len(roots), roots)
	}
	// Sorted by TotalCount desc: alpha(3), beta(1)
	if roots[0].FullName != "alpha" || roots[0].TotalCount != 3 {
		t.Errorf("root[0]: got %+v, want alpha/3", roots[0])
	}
	if roots[1].FullName != "beta" || roots[1].TotalCount != 1 {
		t.Errorf("root[1]: got %+v, want beta/1", roots[1])
	}
	for _, r := range roots {
		if len(r.Children) != 0 {
			t.Errorf("%s should have no children", r.FullName)
		}
		if !r.HasOwnTasks {
			t.Errorf("%s should have HasOwnTasks=true", r.FullName)
		}
	}
}

func TestBuildProjectTree_TwoLevels(t *testing.T) {
	input := []ProjectInput{
		{Name: "work.a", Pending: 3},
		{Name: "work.b", Pending: 5},
		{Name: "home", Pending: 2},
	}
	roots := BuildProjectTree(input)

	// Roots: work(TotalCount=8), home(2) — sorted desc.
	if len(roots) != 2 {
		t.Fatalf("expected 2 roots, got %d: %+v", len(roots), roots)
	}
	work := roots[0]
	home := roots[1]
	if work.FullName != "work" {
		t.Errorf("roots[0]: expected work, got %s", work.FullName)
	}
	if home.FullName != "home" {
		t.Errorf("roots[1]: expected home, got %s", home.FullName)
	}
	if work.TotalCount != 8 {
		t.Errorf("work.TotalCount: got %d, want 8", work.TotalCount)
	}
	if work.SelfCount != 0 {
		t.Errorf("work.SelfCount: got %d, want 0 (no tasks on bare 'work')", work.SelfCount)
	}
	if work.HasOwnTasks {
		t.Error("work should not have HasOwnTasks (no tasks on bare 'work')")
	}
	if home.TotalCount != 2 {
		t.Errorf("home.TotalCount: got %d, want 2", home.TotalCount)
	}

	// work's children: b(5), a(3)
	if len(work.Children) != 2 {
		t.Fatalf("work children: got %d, want 2", len(work.Children))
	}
	if work.Children[0].FullName != "work.b" || work.Children[0].SelfCount != 5 {
		t.Errorf("work.Children[0]: got %+v, want work.b/5", work.Children[0])
	}
	if work.Children[1].FullName != "work.a" || work.Children[1].SelfCount != 3 {
		t.Errorf("work.Children[1]: got %+v, want work.a/3", work.Children[1])
	}
}

func TestBuildProjectTree_ThreeLevels(t *testing.T) {
	input := []ProjectInput{
		{Name: "work.eng.backend", Pending: 2},
		{Name: "work.eng.frontend", Pending: 1},
		{Name: "work.product", Pending: 4},
	}
	roots := BuildProjectTree(input)
	if len(roots) != 1 {
		t.Fatalf("expected 1 root, got %d", len(roots))
	}
	work := roots[0]
	if work.TotalCount != 7 {
		t.Errorf("work.TotalCount: got %d, want 7", work.TotalCount)
	}
	// work has two children: product(4), eng(3)
	if len(work.Children) != 2 {
		t.Fatalf("work.Children: got %d, want 2", len(work.Children))
	}
	product := work.Children[0]
	eng := work.Children[1]
	if product.FullName != "work.product" || product.TotalCount != 4 {
		t.Errorf("work.Children[0]: got %+v, want work.product/4", product)
	}
	if eng.FullName != "work.eng" || eng.TotalCount != 3 {
		t.Errorf("work.Children[1]: got %+v, want work.eng/3", eng)
	}
	// eng's children: backend(2), frontend(1)
	if len(eng.Children) != 2 {
		t.Fatalf("eng.Children: got %d, want 2", len(eng.Children))
	}
	if eng.Children[0].FullName != "work.eng.backend" {
		t.Errorf("eng.Children[0]: got %s, want work.eng.backend", eng.Children[0].FullName)
	}
}

func TestBuildProjectTree_BranchWithOwnTasks(t *testing.T) {
	// "work" itself has tasks AND children
	input := []ProjectInput{
		{Name: "work", Pending: 1},
		{Name: "work.sub", Pending: 3},
	}
	roots := BuildProjectTree(input)
	if len(roots) != 1 {
		t.Fatalf("expected 1 root, got %d", len(roots))
	}
	work := roots[0]
	if work.SelfCount != 1 {
		t.Errorf("work.SelfCount: got %d, want 1", work.SelfCount)
	}
	if !work.HasOwnTasks {
		t.Error("work should have HasOwnTasks=true")
	}
	if work.TotalCount != 4 {
		t.Errorf("work.TotalCount: got %d, want 4", work.TotalCount)
	}
}

func TestBuildProjectTree_SegmentIsLastPart(t *testing.T) {
	input := []ProjectInput{{Name: "a.b.c", Pending: 1}}
	roots := BuildProjectTree(input)
	if len(roots) != 1 {
		t.Fatalf("got %d roots, want 1", len(roots))
	}
	if roots[0].Segment != "a" {
		t.Errorf("root segment: got %q, want %q", roots[0].Segment, "a")
	}
	child := roots[0].Children[0]
	if child.Segment != "b" {
		t.Errorf("child segment: got %q, want %q", child.Segment, "b")
	}
	leaf := child.Children[0]
	if leaf.Segment != "c" {
		t.Errorf("leaf segment: got %q, want %q", leaf.Segment, "c")
	}
}

func TestBuildProjectTree_CompletedAndAge(t *testing.T) {
	const day = int64(86400)
	input := []ProjectInput{
		{Name: "work.a", Pending: 2, Completed: 2, TotalAgeSecs: 4 * day},
		{Name: "work.b", Pending: 0, Completed: 1, TotalAgeSecs: 0},
	}
	roots := BuildProjectTree(input)
	if len(roots) != 1 {
		t.Fatalf("expected 1 root, got %d", len(roots))
	}
	work := roots[0]

	// Subtree totals roll up correctly.
	if work.TotalCount != 2 {
		t.Errorf("work.TotalCount: got %d, want 2", work.TotalCount)
	}
	if work.TotalCompleted != 3 {
		t.Errorf("work.TotalCompleted: got %d, want 3", work.TotalCompleted)
	}

	// PercentComplete: 3/(2+3) = 60%
	if got := work.PercentComplete(); got != 60 {
		t.Errorf("work.PercentComplete(): got %d, want 60", got)
	}

	// AvgAgeDays: 4 days total / 2 pending = 2.0
	if got := work.AvgAgeDays(); got != 2.0 {
		t.Errorf("work.AvgAgeDays(): got %v, want 2.0", got)
	}

	// work.a has both pending and completed.
	wa := work.Children[0] // sorted by TotalCount desc: work.a(2) before work.b(0)
	if wa.FullName != "work.a" {
		t.Fatalf("expected work.a first, got %s", wa.FullName)
	}
	if wa.PercentComplete() != 50 {
		t.Errorf("work.a PercentComplete: got %d, want 50", wa.PercentComplete())
	}

	// work.b: 0 pending, 1 completed = 100%
	wb := work.Children[1]
	if wb.FullName != "work.b" {
		t.Fatalf("expected work.b second, got %s", wb.FullName)
	}
	if wb.PercentComplete() != 100 {
		t.Errorf("work.b PercentComplete: got %d, want 100", wb.PercentComplete())
	}
}

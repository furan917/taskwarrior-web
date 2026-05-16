package views

import (
	"strings"
	"testing"
)

func TestWeekSummaryRowClass_CurrentWeekHasBlue(t *testing.T) {
	got := weekSummaryRowClass(true)
	if !strings.Contains(got, "blue") {
		t.Errorf("current week row should have blue accent, got %q", got)
	}
}

func TestWeekSummaryRowClass_PastWeekHasNoBlue(t *testing.T) {
	got := weekSummaryRowClass(false)
	if strings.Contains(got, "blue") {
		t.Errorf("past week row should not have blue accent, got %q", got)
	}
}

func TestWeekSummaryLabelClass_CurrentWeekIsBold(t *testing.T) {
	got := weekSummaryLabelClass(true)
	if !strings.Contains(got, "font-medium") {
		t.Errorf("current week label should be font-medium, got %q", got)
	}
}

func TestWeekSummaryLabelClass_PastWeekIsMuted(t *testing.T) {
	current := weekSummaryLabelClass(false)
	past := weekSummaryLabelClass(true)
	if current == past {
		t.Errorf("current and past week labels should differ")
	}
	if !strings.Contains(current, "zinc-400") && !strings.Contains(current, "zinc-600") {
		t.Errorf("past week label should use muted zinc colour, got %q", current)
	}
}

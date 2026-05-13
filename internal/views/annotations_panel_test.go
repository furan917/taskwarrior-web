package views

import (
	"testing"

	"github.com/furan917/taskwarrior-web/internal/tw"
)

func TestUserNotes_FiltersJournalAnnotations(t *testing.T) {
	cases := []struct {
		name string
		in   []tw.Annotation
		want []string // descriptions that survive the filter
	}{
		{
			name: "empty input yields empty slice",
			in:   nil,
			want: nil,
		},
		{
			name: "TW3.x Started/Stopped annotations drop out",
			in: []tw.Annotation{
				{Description: "Started task"},
				{Description: "Stopped task"},
				{Description: "called supplier"},
			},
			want: []string{"called supplier"},
		},
		{
			name: "TW2.x timestamped Started/Stopped also drop out",
			in: []tw.Annotation{
				{Description: "Started 20260512T100000Z"},
				{Description: "Stopped 20260512T103000Z"},
				{Description: "remember to follow up"},
			},
			want: []string{"remember to follow up"},
		},
		{
			name: "user prose starting with 'Started ' is kept (no timestamp suffix)",
			in: []tw.Annotation{
				{Description: "Started the conversation with vendor"},
			},
			want: []string{"Started the conversation with vendor"},
		},
		{
			name: "all-journal input yields empty output",
			in: []tw.Annotation{
				{Description: "Started task"},
				{Description: "Stopped task"},
			},
			want: nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := userNotes(tc.in)
			if len(got) != len(tc.want) {
				t.Fatalf("len = %d, want %d (got %+v)", len(got), len(tc.want), got)
			}
			for i, want := range tc.want {
				if got[i].Description != want {
					t.Errorf("got[%d].Description = %q, want %q", i, got[i].Description, want)
				}
			}
		})
	}
}

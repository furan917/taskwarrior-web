package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/furan917/taskwarrior-web-portal/internal/tw"
)

func TestTimeReport_DisabledWhenJournalTimeOff(t *testing.T) {
	installFakeTaskWith(t, fakeTaskOpts{}) // journal.time not set → disabled
	v := &Views{TW: tw.NewClient(), Logger: discardLogger()}

	req := httptest.NewRequest(http.MethodGet, "/reports/time", nil)
	rr := httptest.NewRecorder()
	v.TimeReport(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "journal.time") {
		t.Errorf("disabled page should mention journal.time, got: %.200s", body)
	}
}

func TestTimeReport_DefaultsTo30Days(t *testing.T) {
	installFakeTaskWith(t, fakeTaskOpts{
		ExportJSON:    `[]`,
		JournalTimeRC: "yes",
	})
	v := &Views{TW: tw.NewClient(), Logger: discardLogger()}

	req := httptest.NewRequest(http.MethodGet, "/reports/time", nil)
	rr := httptest.NewRecorder()
	v.TimeReport(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "30d") {
		t.Errorf("default window should be 30d, body: %.200s", rr.Body.String())
	}
}

func TestTimeReport_ValidDaysParams(t *testing.T) {
	for _, days := range []string{"7", "14", "30", "90"} {
		t.Run("days="+days, func(t *testing.T) {
			installFakeTaskWith(t, fakeTaskOpts{
				ExportJSON:    `[]`,
				JournalTimeRC: "yes",
			})
			v := &Views{TW: tw.NewClient(), Logger: discardLogger()}

			req := httptest.NewRequest(http.MethodGet, "/reports/time?days="+days, nil)
			rr := httptest.NewRecorder()
			v.TimeReport(rr, req)

			if rr.Code != http.StatusOK {
				t.Errorf("days=%s: status = %d, want 200", days, rr.Code)
			}
			if !strings.Contains(rr.Body.String(), days+"d") {
				t.Errorf("days=%s: response should show selected window", days)
			}
		})
	}
}

func TestTimeReport_InvalidDaysFallsBackTo30(t *testing.T) {
	installFakeTaskWith(t, fakeTaskOpts{
		ExportJSON:    `[]`,
		JournalTimeRC: "yes",
	})
	v := &Views{TW: tw.NewClient(), Logger: discardLogger()}

	req := httptest.NewRequest(http.MethodGet, "/reports/time?days=999", nil)
	rr := httptest.NewRecorder()
	v.TimeReport(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "30d") {
		t.Errorf("invalid days should fall back to 30d, body: %.200s", rr.Body.String())
	}
}

package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/furan917/taskwarrior-web-portal/internal/tw"
)

func newSync() *Sync {
	return &Sync{TW: tw.NewClient(), Logger: discardLogger()}
}

func TestSync_ResultNilBeforeRun(t *testing.T) {
	s := newSync()
	if s.Result() != nil {
		t.Error("expected nil result before any sync")
	}
}

func TestSync_Run_Success(t *testing.T) {
	installFakeTaskWith(t, fakeTaskOpts{SyncOutput: "Sync complete.", SyncExitCode: 0})
	s := newSync()
	req := httptest.NewRequest(http.MethodPost, "/sync", nil)
	rr := httptest.NewRecorder()
	s.Run(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "Sync complete.") {
		t.Errorf("body missing sync output: %s", body)
	}
	res := s.Result()
	if res == nil {
		t.Fatal("Result() nil after successful sync")
	}
	if !res.OK {
		t.Errorf("Result.OK false after successful sync")
	}
	if !strings.Contains(res.Output, "Sync complete.") {
		t.Errorf("Result.Output missing sync output: %q", res.Output)
	}
	if !strings.Contains(res.Output, "UTC") {
		t.Errorf("Result.Output missing timestamp: %q", res.Output)
	}
}

func TestSync_Run_Failure(t *testing.T) {
	installFakeTaskWith(t, fakeTaskOpts{SyncOutput: "sync error: connection refused", SyncExitCode: 1})
	s := newSync()
	req := httptest.NewRequest(http.MethodPost, "/sync", nil)
	rr := httptest.NewRecorder()
	s.Run(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "sync error") {
		t.Errorf("body missing error output: %s", body)
	}
	res := s.Result()
	if res == nil {
		t.Fatal("Result() nil after failed sync")
	}
	if res.OK {
		t.Errorf("Result.OK true after failed sync")
	}
}

func TestSync_Run_OverwritesPreviousResult(t *testing.T) {
	installFakeTaskWith(t, fakeTaskOpts{SyncOutput: "first", SyncExitCode: 0})
	s := newSync()

	req := httptest.NewRequest(http.MethodPost, "/sync", nil)
	s.Run(httptest.NewRecorder(), req)
	if !strings.Contains(s.Result().Output, "first") {
		t.Fatalf("first sync output wrong: %s", s.Result().Output)
	}

	// second sync with different output
	installFakeTaskWith(t, fakeTaskOpts{SyncOutput: "second", SyncExitCode: 0})
	req2 := httptest.NewRequest(http.MethodPost, "/sync", nil)
	s.Run(httptest.NewRecorder(), req2)
	if !strings.Contains(s.Result().Output, "second") {
		t.Errorf("result not overwritten: got %q", s.Result().Output)
	}
}

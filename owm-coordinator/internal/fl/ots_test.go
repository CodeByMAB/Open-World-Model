package fl

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.uber.org/zap"
)

// fakeProof is a minimal stand-in for an OTS binary proof blob.
var fakeProof = []byte{0x00, 'O', 'p', 'e', 'n', 'T', 'i', 'm', 'e', 's', 't', 'a', 'm', 'p', 's'}

func newTestOrchestrator(client *http.Client, calendars []string) *Orchestrator {
	return &Orchestrator{
		otsCalendars: calendars,
		otsClient:    client,
		log:          zap.NewNop(),
	}
}

func TestSubmitToOTSCalendar_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method: got %q, want POST", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/octet-stream" {
			t.Errorf("Content-Type: got %q, want application/octet-stream", ct)
		}
		body, _ := io.ReadAll(r.Body)
		if len(body) != 32 {
			t.Errorf("body length: got %d, want 32", len(body))
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(fakeProof)
	}))
	defer srv.Close()

	o := newTestOrchestrator(srv.Client(), []string{srv.URL})
	proof, err := o.submitToOTSCalendar(srv.URL, make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	if string(proof) != string(fakeProof) {
		t.Errorf("proof mismatch: got %x, want %x", proof, fakeProof)
	}
}

func TestSubmitToOTSCalendar_NonOKStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	o := newTestOrchestrator(srv.Client(), nil)
	_, err := o.submitToOTSCalendar(srv.URL, make([]byte, 32))
	if err == nil {
		t.Fatal("expected error for non-OK status")
	}
	if !strings.Contains(err.Error(), "503") {
		t.Errorf("expected 503 in error, got: %v", err)
	}
}

func TestSubmitToOTSCalendar_EmptyBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		// deliberately write no body
	}))
	defer srv.Close()

	o := newTestOrchestrator(srv.Client(), nil)
	_, err := o.submitToOTSCalendar(srv.URL, make([]byte, 32))
	if err == nil {
		t.Fatal("expected error for empty proof body")
	}
}

func TestStampOTS_AllCalendarsFail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	o := newTestOrchestrator(srv.Client(), []string{srv.URL, srv.URL, srv.URL})
	// db and storage are nil; stampOTS returns early before touching them when all calendars fail.
	o.stampOTS(strings.Repeat("aa", 32), 1) // 64 hex chars = 32 bytes
}

func TestStampOTS_InvalidHash(t *testing.T) {
	o := newTestOrchestrator(http.DefaultClient, defaultOTSCalendars)
	// Should log and return without panicking or making any HTTP calls.
	o.stampOTS("not-valid-hex", 99)
	o.stampOTS(strings.Repeat("ff", 16), 99) // 16 bytes — too short for SHA-256
}

func TestOrchestratorDefaultCalendars(t *testing.T) {
	o := New(nil, nil, nil, zap.NewNop())
	if len(o.otsCalendars) != len(defaultOTSCalendars) {
		t.Errorf("expected %d default OTS calendars, got %d", len(defaultOTSCalendars), len(o.otsCalendars))
	}
	for _, cal := range o.otsCalendars {
		if !strings.HasPrefix(cal, "https://") {
			t.Errorf("default calendar URL should use HTTPS: %q", cal)
		}
	}
}

func TestOrchestratorCustomCalendars(t *testing.T) {
	custom := []string{"https://cal1.example.com/digest", "https://cal2.example.com/digest"}
	o := NewWithConfig(nil, nil, nil, &OrchestratorConfig{OTSCalendars: custom}, zap.NewNop())
	if len(o.otsCalendars) != len(custom) {
		t.Errorf("expected %d custom calendars, got %d", len(custom), len(o.otsCalendars))
	}
	for i, cal := range o.otsCalendars {
		if cal != custom[i] {
			t.Errorf("calendar[%d]: got %q, want %q", i, cal, custom[i])
		}
	}
}

func TestStampOTS_NoCalendarsConfigured(t *testing.T) {
	o := newTestOrchestrator(http.DefaultClient, nil)
	// Should log and return without panicking.
	o.stampOTS(strings.Repeat("ab", 32), 5)
}

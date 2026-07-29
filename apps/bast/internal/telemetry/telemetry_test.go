package telemetry

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestEnabledRespectsBAST_NO_TELEMETRY(t *testing.T) {
	t.Setenv("BAST_NO_TELEMETRY", "1")
	if Enabled() {
		t.Fatal("expected telemetry to be disabled")
	}

	t.Setenv("BAST_NO_TELEMETRY", "")
	if !Enabled() {
		t.Fatal("expected telemetry to be enabled")
	}
}

func TestTrackSendsCliPayload(t *testing.T) {
	t.Setenv("BAST_NO_TELEMETRY", "")

	got := make(chan payload, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %q", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("content-type = %q", r.Header.Get("Content-Type"))
		}

		var body payload
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		got <- body
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)

	original := endpoint
	endpoint = server.URL
	t.Cleanup(func() { endpoint = original })

	Track("connect", "v1.2.3")

	select {
	case body := <-got:
		if body.Event != "connect" || body.Version != "v1.2.3" || body.Source != "cli" {
			t.Fatalf("unexpected payload: %+v", body)
		}
		if body.OS == "" || body.Arch == "" {
			t.Fatalf("expected platform fields, got %+v", body)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for telemetry request")
	}
}

func TestTrackSkipsWhenDisabled(t *testing.T) {
	t.Setenv("BAST_NO_TELEMETRY", "1")

	called := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called <- struct{}{}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)

	original := endpoint
	endpoint = server.URL
	t.Cleanup(func() { endpoint = original })

	Track("tui_open", "v1.2.3")

	select {
	case <-called:
		t.Fatal("telemetry request sent while disabled")
	case <-time.After(100 * time.Millisecond):
	}
}

func TestReportErrorSendsPayload(t *testing.T) {
	t.Setenv("BAST_NO_TELEMETRY", "")

	got := make(chan Report, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body Report
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		got <- body
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)

	original := errorEndpoint
	errorEndpoint = server.URL
	t.Cleanup(func() { errorEndpoint = original })

	ReportError(Report{
		Message: "boom",
		Version: "v1.2.3",
		Code:    "operation_failed",
		Context: "cli",
	})

	select {
	case body := <-got:
		if body.Message != "boom" || body.Version != "v1.2.3" || body.Source != "cli" || body.Code != "operation_failed" || body.Context != "cli" {
			t.Fatalf("unexpected payload: %+v", body)
		}
		if body.OS == "" || body.Arch == "" {
			t.Fatalf("expected platform fields, got %+v", body)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for error report")
	}
}

func TestReportErrorSkipsWhenDisabled(t *testing.T) {
	t.Setenv("BAST_NO_TELEMETRY", "1")

	called := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called <- struct{}{}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)

	original := errorEndpoint
	errorEndpoint = server.URL
	t.Cleanup(func() { errorEndpoint = original })

	ReportError(Report{Message: "boom", Version: "v1.2.3"})

	select {
	case <-called:
		t.Fatal("error report sent while disabled")
	case <-time.After(100 * time.Millisecond):
	}
}

func TestOfferReportSendsOnSpace(t *testing.T) {
	t.Setenv("BAST_NO_TELEMETRY", "")

	got := make(chan Report, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body Report
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		got <- body
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)

	original := errorEndpoint
	errorEndpoint = server.URL
	t.Cleanup(func() { errorEndpoint = original })

	var out strings.Builder
	OfferReport(strings.NewReader(" "), &out, Report{Message: "fail", Version: "v1.2.3", Context: "tui"})
	if !strings.Contains(out.String(), "Press Space to send") {
		t.Fatalf("missing prompt: %q", out.String())
	}

	select {
	case body := <-got:
		if body.Message != "fail" || body.Context != "tui" {
			t.Fatalf("unexpected payload: %+v", body)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for error report")
	}
}

func TestOfferReportSkipsSendOnOtherKey(t *testing.T) {
	t.Setenv("BAST_NO_TELEMETRY", "")

	called := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called <- struct{}{}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)

	original := errorEndpoint
	errorEndpoint = server.URL
	t.Cleanup(func() { errorEndpoint = original })

	OfferReport(strings.NewReader("k"), ioDiscard{}, Report{Message: "fail", Version: "v1.2.3"})

	select {
	case <-called:
		t.Fatal("error report sent without Space")
	case <-time.After(100 * time.Millisecond):
	}
}

type ioDiscard struct{}

func (ioDiscard) Write(p []byte) (int, error) { return len(p), nil }

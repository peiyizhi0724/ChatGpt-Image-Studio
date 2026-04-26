package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"chatgpt2api/internal/accounts"
	"chatgpt2api/internal/config"
)

type admissionStressResponseForTest struct {
	Counters struct {
		Submitted    int `json:"submitted"`
		Attempted    int `json:"attempted"`
		Admitted     int `json:"admitted"`
		QueueFull    int `json:"queueFull"`
		QueueTimeout int `json:"queueTimeout"`
		Canceled     int `json:"canceled"`
		OtherErrors  int `json:"otherErrors"`
	} `json:"counters"`
	QueueWait struct {
		Samples int `json:"samples"`
	} `json:"queueWait"`
}

func TestAdmissionStressAllAdmitted(t *testing.T) {
	server := newAdmissionStressTestServer(t, 4, 16, 2)

	body := []byte(`{"total":20,"workers":8,"holdMs":30,"timeoutSeconds":10}`)
	req := httptest.NewRequest(http.MethodPost, "/api/tools/admission-stress", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer test-ui-key")
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var payload admissionStressResponseForTest
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if payload.Counters.Submitted != 20 {
		t.Fatalf("submitted = %d, want 20", payload.Counters.Submitted)
	}
	if payload.Counters.Attempted != 20 {
		t.Fatalf("attempted = %d, want 20", payload.Counters.Attempted)
	}
	if payload.Counters.Admitted != 20 {
		t.Fatalf("admitted = %d, want 20", payload.Counters.Admitted)
	}
	if payload.Counters.QueueFull != 0 {
		t.Fatalf("queueFull = %d, want 0", payload.Counters.QueueFull)
	}
	if payload.Counters.QueueTimeout != 0 {
		t.Fatalf("queueTimeout = %d, want 0", payload.Counters.QueueTimeout)
	}
	if payload.Counters.Canceled != 0 {
		t.Fatalf("canceled = %d, want 0", payload.Counters.Canceled)
	}
	if payload.Counters.OtherErrors != 0 {
		t.Fatalf("otherErrors = %d, want 0", payload.Counters.OtherErrors)
	}
	if payload.QueueWait.Samples != 20 {
		t.Fatalf("queueWait samples = %d, want 20", payload.QueueWait.Samples)
	}
}

func TestAdmissionStressQueueFullUnderContention(t *testing.T) {
	server := newAdmissionStressTestServer(t, 1, 1, 1)

	body := []byte(`{"total":20,"workers":20,"holdMs":200,"timeoutSeconds":8}`)
	req := httptest.NewRequest(http.MethodPost, "/api/tools/admission-stress", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer test-ui-key")
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var payload admissionStressResponseForTest
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if payload.Counters.Submitted != 20 {
		t.Fatalf("submitted = %d, want 20", payload.Counters.Submitted)
	}
	if payload.Counters.Attempted != 20 {
		t.Fatalf("attempted = %d, want 20", payload.Counters.Attempted)
	}
	if payload.Counters.QueueFull == 0 {
		t.Fatalf("queueFull = %d, want > 0", payload.Counters.QueueFull)
	}
	if payload.Counters.Admitted == 0 {
		t.Fatalf("admitted = %d, want > 0", payload.Counters.Admitted)
	}
}

func newAdmissionStressTestServer(t *testing.T, maxConcurrency, queueLimit, queueTimeoutSeconds int) *Server {
	t.Helper()

	cfg := config.New(t.TempDir())
	if err := cfg.Load(); err != nil {
		t.Fatalf("load config: %v", err)
	}
	cfg.App.AuthKey = "test-ui-key"
	cfg.Server.MaxImageConcurrency = maxConcurrency
	cfg.Server.ImageQueueLimit = queueLimit
	cfg.Server.ImageQueueTimeoutSeconds = queueTimeoutSeconds

	store, err := accounts.NewStore(cfg)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	return NewServer(cfg, store, nil)
}

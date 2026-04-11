package api_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/penche/router/internal/api"
	"github.com/penche/router/internal/auth"
	"github.com/penche/router/internal/config"
	"github.com/penche/router/internal/domain"
	"github.com/penche/router/internal/storage"
	"log/slog"
	"os"
)

const testSecret = "integration-test-secret"

func newTestRouter(t *testing.T) *chi.Mux {
	t.Helper()
	store, err := storage.New(":memory:")
	if err != nil {
		t.Fatalf("storage: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	verifier := auth.NewVerifier(testSecret)
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	handler := api.New(store, verifier,
		config.RoutesConfig{Default: "taiga"},
		config.WorkerConfig{MaxRetries: 5},
		log,
	)

	r := chi.NewRouter()
	handler.Mount(r)
	return r
}

func signedRequest(t *testing.T, method, path string, body []byte) *http.Request {
	t.Helper()
	ts := time.Now().Unix()
	sig := auth.Sign(testSecret, ts, body)
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Penche-Timestamp", strconv.FormatInt(ts, 10))
	req.Header.Set("X-Penche-Signature", sig)
	return req
}

func TestHealth(t *testing.T) {
	r := newTestRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/v1/health", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestPostEvent_HappyPath(t *testing.T) {
	r := newTestRouter(t)

	payload := domain.IncomingEvent{
		EventID:    "test-evt-001",
		CapturedAt: time.Now().UTC(),
		Domain:     "xss.is",
		PageTitle:  "Test Thread",
		PageURL:    "https://xss.is/threads/1234",
		Screenshot: domain.ScreenshotPayload{
			MIME:   "image/jpeg",
			Base64: base64.StdEncoding.EncodeToString([]byte("fake")),
		},
		Meta: domain.EventMeta{Browser: "firefox", Tags: []string{"cti"}},
	}
	body, _ := json.Marshal(payload)

	req := signedRequest(t, http.MethodPost, "/v1/events", body)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d — body: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]string
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["status"] != "accepted" {
		t.Errorf("expected accepted, got %q", resp["status"])
	}
}

func TestPostEvent_Duplicate(t *testing.T) {
	r := newTestRouter(t)

	payload := domain.IncomingEvent{
		EventID:    "dup-evt-001",
		CapturedAt: time.Now().UTC(),
		Domain:     "xss.is",
		PageTitle:  "Dup",
		PageURL:    "https://xss.is/1",
		Screenshot: domain.ScreenshotPayload{MIME: "image/jpeg", Base64: base64.StdEncoding.EncodeToString([]byte("x"))},
		Meta:       domain.EventMeta{},
	}
	body, _ := json.Marshal(payload)

	// First request.
	r.ServeHTTP(httptest.NewRecorder(), signedRequest(t, http.MethodPost, "/v1/events", body))

	// Duplicate.
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, signedRequest(t, http.MethodPost, "/v1/events", body))
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for duplicate, got %d", rr.Code)
	}
	var resp map[string]string
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["status"] != "duplicate" {
		t.Errorf("expected duplicate status, got %q", resp["status"])
	}
}

func TestPostEvent_NoAuth(t *testing.T) {
	r := newTestRouter(t)
	req := httptest.NewRequest(http.MethodPost, "/v1/events", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

// Dummy to satisfy unused import.
var _ = context.Background

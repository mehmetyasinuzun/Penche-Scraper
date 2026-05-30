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

func TestEventsAPI_ListImageStatsDelete(t *testing.T) {
	r := newTestRouter(t)

	img := base64.StdEncoding.EncodeToString([]byte("\xff\xd8\xff fake jpeg bytes"))
	payload := domain.IncomingEvent{
		EventID:    "api-evt-1",
		CapturedAt: time.Now().UTC(),
		Domain:     "exploit.in",
		PageTitle:  "RDP access EU",
		PageURL:    "https://exploit.in/threads/9",
		Screenshot: domain.ScreenshotPayload{MIME: "image/jpeg", Base64: img},
		Meta:       domain.EventMeta{Tags: []string{"cti"}},
	}
	body, _ := json.Marshal(payload)
	r.ServeHTTP(httptest.NewRecorder(), signedRequest(t, http.MethodPost, "/v1/events", body))

	var lst struct {
		Total  int `json:"total"`
		Events []struct {
			EventID  string `json:"event_id"`
			HasImage bool   `json:"has_image"`
		} `json:"events"`
	}

	// List is unauthenticated and returns summaries.
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/v1/events?domain=exploit.in", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("list: expected 200, got %d", rr.Code)
	}
	json.Unmarshal(rr.Body.Bytes(), &lst)
	if lst.Total != 1 || len(lst.Events) != 1 {
		t.Fatalf("expected 1 event, got total=%d len=%d", lst.Total, len(lst.Events))
	}
	if !lst.Events[0].HasImage {
		t.Error("expected has_image=true")
	}

	// Image bytes stream with the right content type.
	rr = httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/v1/events/api-evt-1/image", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("image: expected 200, got %d", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); ct != "image/jpeg" {
		t.Errorf("image content-type: got %q", ct)
	}

	// Unknown image → 404.
	rr = httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/v1/events/nope/image", nil))
	if rr.Code != http.StatusNotFound {
		t.Errorf("missing image: expected 404, got %d", rr.Code)
	}

	// Stats endpoint.
	rr = httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/v1/stats", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("stats: expected 200, got %d", rr.Code)
	}

	// Delete the capture.
	rr = httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest(http.MethodDelete, "/v1/events/api-evt-1", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("delete: expected 200, got %d", rr.Code)
	}

	rr = httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/v1/events", nil))
	json.Unmarshal(rr.Body.Bytes(), &lst)
	if lst.Total != 0 {
		t.Errorf("expected 0 events after delete, got %d", lst.Total)
	}
}

func TestGalleryUI(t *testing.T) {
	r := newTestRouter(t)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/ui", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("ui: expected 200, got %d", rr.Code)
	}
	if !bytes.Contains(rr.Body.Bytes(), []byte("Penche")) {
		t.Error("gallery HTML should mention Penche")
	}
}

// Dummy to satisfy unused import.
var _ = context.Background

package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/penche/router/internal/auth"
	"github.com/penche/router/internal/config"
	"github.com/penche/router/internal/domain"
	"github.com/penche/router/internal/storage"
)

// Store is the subset of storage operations needed by the API.
type Store interface {
	InsertEvent(ctx context.Context, evt *domain.IncomingEvent) (*domain.StoredEvent, error)
	CreateDeliveryJob(ctx context.Context, eventID, destination string, maxAttempts int) (*domain.DeliveryJob, error)
	CountEventsByStatus(ctx context.Context) (map[string]int64, error)
}

// Handler holds all HTTP handler dependencies.
type Handler struct {
	store    Store
	verifier *auth.Verifier
	routes   config.RoutesConfig
	worker   config.WorkerConfig
	log      *slog.Logger
}

// New creates the HTTP handler.
func New(store Store, verifier *auth.Verifier, routes config.RoutesConfig, worker config.WorkerConfig, log *slog.Logger) *Handler {
	return &Handler{
		store:    store,
		verifier: verifier,
		routes:   routes,
		worker:   worker,
		log:      log,
	}
}

// Mount registers all routes on a Chi router.
func (h *Handler) Mount(r chi.Router) {
	r.Use(requestLogger(h.log))
	r.Use(maxBodySize(10 * 1024 * 1024)) // 10 MB max

	r.Get("/v1/health", h.handleHealth)
	r.Get("/v1/metrics", h.handleMetrics)
	r.With(h.authMiddleware).Post("/v1/events", h.handlePostEvent)
}

// handleHealth returns 200 OK with a JSON body.
func (h *Handler) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"status": "ok",
		"time":   time.Now().UTC().Format(time.RFC3339),
	})
}

// handleMetrics returns basic event counts.
func (h *Handler) handleMetrics(w http.ResponseWriter, r *http.Request) {
	counts, err := h.store.CountEventsByStatus(r.Context())
	if err != nil {
		h.log.Error("metrics query failed", "error", err)
		writeError(w, http.StatusInternalServerError, "metrics unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"events": counts,
	})
}

// handlePostEvent accepts a new event from the extension.
func (h *Handler) handlePostEvent(w http.ResponseWriter, r *http.Request) {
	body, ok := r.Context().Value(bodyKey{}).([]byte)
	if !ok || body == nil {
		writeError(w, http.StatusBadRequest, "body missing")
		return
	}

	var evt domain.IncomingEvent
	if err := json.Unmarshal(body, &evt); err != nil {
		h.log.Warn("invalid event payload", "error", err)
		writeError(w, http.StatusBadRequest, "invalid JSON payload")
		return
	}

	if err := validateEvent(&evt); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	stored, err := h.store.InsertEvent(r.Context(), &evt)
	if err != nil {
		if errors.Is(err, storage.ErrDuplicate) {
			writeJSON(w, http.StatusOK, map[string]string{
				"status":   "duplicate",
				"event_id": evt.EventID,
			})
			return
		}
		h.log.Error("insert event failed", "error", err, "event_id", evt.EventID)
		writeError(w, http.StatusInternalServerError, "storage error")
		return
	}

	destination := h.resolveDestination(evt.Domain)
	maxAttempts := h.worker.MaxRetries
	if maxAttempts <= 0 {
		maxAttempts = 5
	}

	if _, err := h.store.CreateDeliveryJob(r.Context(), stored.EventID, destination, maxAttempts); err != nil {
		h.log.Error("create delivery job failed", "error", err, "event_id", stored.EventID)
		// Don't fail the request — event is persisted, worker will retry.
	}

	h.log.Info("event accepted",
		"event_id", stored.EventID,
		"domain", stored.Domain,
		"destination", destination,
	)

	writeJSON(w, http.StatusAccepted, map[string]string{
		"status":      "accepted",
		"event_id":    stored.EventID,
		"destination": destination,
	})
}

func (h *Handler) resolveDestination(domain string) string {
	if h.routes.DomainMap != nil {
		if dest, ok := h.routes.DomainMap[domain]; ok {
			return dest
		}
	}
	if h.routes.Default != "" {
		return h.routes.Default
	}
	return "taiga"
}

func validateEvent(evt *domain.IncomingEvent) error {
	if evt.EventID == "" {
		return errors.New("event_id is required")
	}
	if evt.PageURL == "" {
		return errors.New("page_url is required")
	}
	if evt.Domain == "" {
		return errors.New("domain is required")
	}
	if evt.Screenshot.Base64 == "" {
		return errors.New("screenshot.base64 is required")
	}
	return nil
}

// --- helpers ---

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

type bodyKey struct{}

func maxBodySize(limit int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, limit)
			body, err := io.ReadAll(r.Body)
			if err != nil {
				writeError(w, http.StatusRequestEntityTooLarge, "request too large")
				return
			}
			ctx := context.WithValue(r.Context(), bodyKey{}, body)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

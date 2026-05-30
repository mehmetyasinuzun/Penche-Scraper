package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
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
	QueryEvents(ctx context.Context, f domain.EventFilter) ([]*domain.EventSummary, int, error)
	GetEventImage(ctx context.Context, eventID string) (string, []byte, error)
	DeleteEvent(ctx context.Context, eventID string) (bool, error)
	StatsByDomain(ctx context.Context) ([]domain.DomainCount, error)
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
	r.Get("/v1/stats", h.handleStats)
	r.Get("/v1/events", h.handleListEvents)
	r.Get("/v1/events/{id}/image", h.handleEventImage)
	r.Delete("/v1/events/{id}", h.handleDeleteEvent)
	r.With(h.authMiddleware).Post("/v1/events", h.handlePostEvent)

	// Gallery UI — a static shell that loads data from the JSON API above.
	r.Get("/ui", serveGallery)
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/ui", http.StatusFound)
	})
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

// handleStats returns dashboard figures: total, per-status counts, per-domain counts.
func (h *Handler) handleStats(w http.ResponseWriter, r *http.Request) {
	statusCounts, err := h.store.CountEventsByStatus(r.Context())
	if err != nil {
		h.log.Error("stats: status query failed", "error", err)
		writeError(w, http.StatusInternalServerError, "stats unavailable")
		return
	}
	domains, err := h.store.StatsByDomain(r.Context())
	if err != nil {
		h.log.Error("stats: domain query failed", "error", err)
		writeError(w, http.StatusInternalServerError, "stats unavailable")
		return
	}
	var total int64
	for _, c := range statusCounts {
		total += c
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"total":   total,
		"status":  statusCounts,
		"domains": domains,
	})
}

// handleListEvents returns a filtered, paginated page of event summaries.
func (h *Handler) handleListEvents(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := domain.EventFilter{
		Domain: q.Get("domain"),
		Status: q.Get("status"),
		Search: strings.TrimSpace(q.Get("q")),
		Limit:  atoiDefault(q.Get("limit"), 60),
		Offset: atoiDefault(q.Get("offset"), 0),
	}
	items, total, err := h.store.QueryEvents(r.Context(), f)
	if err != nil {
		h.log.Error("list events failed", "error", err)
		writeError(w, http.StatusInternalServerError, "could not list events")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"total":  total,
		"limit":  f.Limit,
		"offset": f.Offset,
		"events": items,
	})
}

// handleEventImage streams the screenshot bytes for a single event.
func (h *Handler) handleEventImage(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	mime, data, err := h.store.GetEventImage(r.Context(), id)
	if err != nil {
		h.log.Error("get image failed", "error", err, "event_id", id)
		writeError(w, http.StatusInternalServerError, "could not load image")
		return
	}
	if len(data) == 0 {
		http.NotFound(w, r)
		return
	}
	if mime == "" {
		mime = "application/octet-stream"
	}
	w.Header().Set("Content-Type", mime)
	w.Header().Set("Cache-Control", "private, max-age=300")
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	_, _ = w.Write(data)
}

// handleDeleteEvent removes a single capture and its delivery history.
func (h *Handler) handleDeleteEvent(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	ok, err := h.store.DeleteEvent(r.Context(), id)
	if err != nil {
		h.log.Error("delete event failed", "error", err, "event_id", id)
		writeError(w, http.StatusInternalServerError, "could not delete event")
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "event not found")
		return
	}
	h.log.Info("event deleted", "event_id", id)
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted", "event_id": id})
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

func atoiDefault(s string, def int) int {
	if s == "" {
		return def
	}
	if n, err := strconv.Atoi(s); err == nil {
		return n
	}
	return def
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

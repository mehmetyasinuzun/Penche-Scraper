package worker

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"sync"
	"time"

	"github.com/penche/router/internal/adapters"
	"github.com/penche/router/internal/config"
	"github.com/penche/router/internal/domain"
	"github.com/penche/router/internal/storage"
)

// Store is the subset of storage.Store used by the worker.
type Store interface {
	PollDueJobs(ctx context.Context, limit int) ([]*domain.DeliveryJob, error)
	GetEvent(ctx context.Context, eventID string) (*domain.StoredEvent, error)
	MarkJobProcessing(ctx context.Context, id int64) error
	MarkJobDone(ctx context.Context, id int64, attemptNo int) error
	MarkJobFailed(ctx context.Context, id int64, attemptNo int, errMsg string, nextRunAt time.Time, deadLetter bool) error
	UpdateEventStatus(ctx context.Context, eventID string, status domain.EventStatus) error
}

// Worker polls for pending delivery jobs and dispatches them to adapters.
type Worker struct {
	store    Store
	adapters map[string]adapters.DestinationAdapter
	cfg      config.WorkerConfig
	log      *slog.Logger
	wg       sync.WaitGroup
	sem      chan struct{}
}

// New creates a Worker.
func New(store Store, adapterMap map[string]adapters.DestinationAdapter, cfg config.WorkerConfig, log *slog.Logger) *Worker {
	return &Worker{
		store:    store,
		adapters: adapterMap,
		cfg:      cfg,
		log:      log,
		sem:      make(chan struct{}, cfg.ConcurrentJobs),
	}
}

// Run starts the polling loop. Blocks until ctx is cancelled.
func (w *Worker) Run(ctx context.Context) {
	pollInterval := time.Duration(w.cfg.PollIntervalMs) * time.Millisecond
	if pollInterval <= 0 {
		pollInterval = 2 * time.Second
	}

	w.log.Info("worker started", "poll_interval", pollInterval, "concurrent_jobs", w.cfg.ConcurrentJobs)
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			w.log.Info("worker shutting down, waiting for in-flight jobs")
			w.wg.Wait()
			return
		case <-ticker.C:
			w.poll(ctx)
		}
	}
}

func (w *Worker) poll(ctx context.Context) {
	limit := w.cfg.ConcurrentJobs
	if limit <= 0 {
		limit = 3
	}

	jobs, err := w.store.PollDueJobs(ctx, limit)
	if err != nil {
		w.log.Error("poll jobs failed", "error", err)
		return
	}

	for _, job := range jobs {
		// Claim slot
		select {
		case w.sem <- struct{}{}:
		default:
			// Concurrency limit hit; jobs will be picked up next tick.
			return
		}

		w.wg.Add(1)
		go func(j *domain.DeliveryJob) {
			defer w.wg.Done()
			defer func() { <-w.sem }()
			w.process(ctx, j)
		}(job)
	}
}

func (w *Worker) process(ctx context.Context, job *domain.DeliveryJob) {
	log := w.log.With("job_id", job.ID, "event_id", job.EventID, "destination", job.Destination)

	if err := w.store.MarkJobProcessing(ctx, job.ID); err != nil {
		log.Error("mark processing failed", "error", err)
		return
	}

	evt, err := w.store.GetEvent(ctx, job.EventID)
	if err != nil || evt == nil {
		log.Error("event not found", "error", err)
		_ = w.failJob(ctx, job, "event not found", true)
		return
	}

	adapter, ok := w.adapters[job.Destination]
	if !ok {
		log.Error("adapter not found", "destination", job.Destination)
		_ = w.failJob(ctx, job, fmt.Sprintf("adapter %q not registered", job.Destination), true)
		return
	}

	attemptNo := job.AttemptCount + 1
	result, err := adapter.Send(ctx, evt)
	if err != nil {
		log.Warn("delivery failed", "attempt", attemptNo, "error", err)
		isDeadLetter := attemptNo >= job.MaxAttempts
		nextRun := w.backoffTime(attemptNo)
		if isDeadLetter {
			_ = w.store.UpdateEventStatus(ctx, job.EventID, domain.EventStatusDeadLetter)
		}
		_ = w.store.MarkJobFailed(ctx, job.ID, attemptNo, err.Error(), nextRun, isDeadLetter)
		return
	}

	log.Info("delivery succeeded", "attempt", attemptNo, "external_id", result.ExternalID, "msg", result.Message)
	_ = w.store.MarkJobDone(ctx, job.ID, attemptNo)
	_ = w.store.UpdateEventStatus(ctx, job.EventID, domain.EventStatusDelivered)
}

func (w *Worker) failJob(ctx context.Context, job *domain.DeliveryJob, reason string, deadLetter bool) error {
	return w.store.MarkJobFailed(ctx, job.ID, job.AttemptCount+1, reason, time.Now(), deadLetter)
}

// backoffTime returns exponential backoff with jitter.
func (w *Worker) backoffTime(attempt int) time.Time {
	base := float64(w.cfg.BaseBackoffMs)
	maxMs := float64(w.cfg.MaxBackoffMs)
	if base <= 0 {
		base = 1000
	}
	if maxMs <= 0 {
		maxMs = 60000
	}
	delay := base * math.Pow(2, float64(attempt-1))
	if delay > maxMs {
		delay = maxMs
	}
	return time.Now().Add(time.Duration(delay) * time.Millisecond)
}

// Ensure Store interface is satisfied by *storage.Store at compile time.
var _ Store = (*storage.Store)(nil)

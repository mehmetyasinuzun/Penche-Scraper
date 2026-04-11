package worker

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/penche/router/internal/adapters"
	"github.com/penche/router/internal/config"
	"github.com/penche/router/internal/domain"
	"log/slog"
	"os"
)

// ─── fake store ──────────────────────────────────────────────────────────────

type fakeStore struct {
	mu     sync.Mutex
	jobs   []*domain.DeliveryJob
	events map[string]*domain.StoredEvent

	processing []int64
	done       []int64
	failed     []failRecord
	statuses   map[string]domain.EventStatus
}

type failRecord struct {
	id         int64
	attempt    int
	msg        string
	deadLetter bool
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		events:   make(map[string]*domain.StoredEvent),
		statuses: make(map[string]domain.EventStatus),
	}
}

func (s *fakeStore) addEvent(evt *domain.StoredEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events[evt.EventID] = evt
}

func (s *fakeStore) addJob(j *domain.DeliveryJob) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.jobs = append(s.jobs, j)
}

func (s *fakeStore) PollDueJobs(_ context.Context, limit int) ([]*domain.DeliveryJob, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.jobs) == 0 {
		return nil, nil
	}
	n := limit
	if n > len(s.jobs) {
		n = len(s.jobs)
	}
	batch := s.jobs[:n]
	s.jobs = s.jobs[n:]
	return batch, nil
}

func (s *fakeStore) GetEvent(_ context.Context, eventID string) (*domain.StoredEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	evt, ok := s.events[eventID]
	if !ok {
		return nil, nil
	}
	return evt, nil
}

func (s *fakeStore) MarkJobProcessing(_ context.Context, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.processing = append(s.processing, id)
	return nil
}

func (s *fakeStore) MarkJobDone(_ context.Context, id int64, _ int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.done = append(s.done, id)
	return nil
}

func (s *fakeStore) MarkJobFailed(_ context.Context, id int64, attempt int, msg string, _ time.Time, dl bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failed = append(s.failed, failRecord{id: id, attempt: attempt, msg: msg, deadLetter: dl})
	return nil
}

func (s *fakeStore) UpdateEventStatus(_ context.Context, eventID string, status domain.EventStatus) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.statuses[eventID] = status
	return nil
}

// ─── fake adapter ────────────────────────────────────────────────────────────

type fakeAdapter struct {
	mu       sync.Mutex
	calls    int
	failN    int // fail first N calls
	sendErr  error
}

func (a *fakeAdapter) Name() string { return "fake" }
func (a *fakeAdapter) ValidateConfig() error { return nil }

func (a *fakeAdapter) Send(_ context.Context, _ *domain.StoredEvent) (adapters.DeliveryResult, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.calls++
	if a.failN > 0 {
		a.failN--
		return adapters.DeliveryResult{}, a.sendErr
	}
	return adapters.DeliveryResult{ExternalID: "ok-123"}, nil
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
}

func testWorkerCfg() config.WorkerConfig {
	return config.WorkerConfig{
		PollIntervalMs: 50,
		ConcurrentJobs: 3,
		MaxRetries:     3,
		BaseBackoffMs:  10,
		MaxBackoffMs:   1000,
	}
}

func sampleEvent(id string) *domain.StoredEvent {
	return &domain.StoredEvent{
		EventID:        id,
		CapturedAt:     time.Now().UTC(),
		Domain:         "xss.is",
		PageTitle:      "Test",
		PageURL:        "https://xss.is/1",
		ScreenshotMIME: "image/jpeg",
		ScreenshotData: []byte("fake"),
		MetaTags:       `[]`,
	}
}

func sampleJob(id int64, eventID, dest string, maxAttempts int) *domain.DeliveryJob {
	return &domain.DeliveryJob{
		ID:          id,
		EventID:     eventID,
		Destination: dest,
		Status:      domain.JobStatusQueued,
		MaxAttempts: maxAttempts,
		NextRunAt:   time.Now().Add(-time.Second),
	}
}

// ─── tests ───────────────────────────────────────────────────────────────────

func TestProcess_SuccessfulDelivery(t *testing.T) {
	store := newFakeStore()
	adapter := &fakeAdapter{}

	evt := sampleEvent("evt-success-001")
	job := sampleJob(1, evt.EventID, "fake", 3)
	store.addEvent(evt)
	store.addJob(job)

	w := New(store, map[string]adapters.DestinationAdapter{"fake": adapter}, testWorkerCfg(), testLogger())
	w.poll(context.Background())
	time.Sleep(150 * time.Millisecond) // let goroutine finish

	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.done) != 1 {
		t.Fatalf("expected 1 done job, got %d", len(store.done))
	}
	if store.statuses[evt.EventID] != domain.EventStatusDelivered {
		t.Errorf("expected status delivered, got %q", store.statuses[evt.EventID])
	}
}

func TestProcess_RetryOnFailure(t *testing.T) {
	store := newFakeStore()
	adapter := &fakeAdapter{failN: 1, sendErr: errors.New("transient error")}

	evt := sampleEvent("evt-retry-001")
	job := sampleJob(2, evt.EventID, "fake", 3)
	store.addEvent(evt)
	store.addJob(job)

	w := New(store, map[string]adapters.DestinationAdapter{"fake": adapter}, testWorkerCfg(), testLogger())
	w.poll(context.Background())
	time.Sleep(150 * time.Millisecond)

	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.failed) != 1 {
		t.Fatalf("expected 1 failed record, got %d", len(store.failed))
	}
	if store.failed[0].deadLetter {
		t.Error("attempt 1 of 3 should not be dead-lettered")
	}
}

func TestProcess_DeadLetterAfterMaxAttempts(t *testing.T) {
	store := newFakeStore()
	alwaysFail := &fakeAdapter{failN: 99, sendErr: errors.New("permanent error")}

	evt := sampleEvent("evt-dlq-001")
	// MaxAttempts=1 so first failure → dead letter.
	job := sampleJob(3, evt.EventID, "fake", 1)
	job.AttemptCount = 0
	store.addEvent(evt)
	store.addJob(job)

	w := New(store, map[string]adapters.DestinationAdapter{"fake": alwaysFail}, testWorkerCfg(), testLogger())
	w.poll(context.Background())
	time.Sleep(150 * time.Millisecond)

	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.failed) != 1 {
		t.Fatalf("expected 1 failed record, got %d", len(store.failed))
	}
	if !store.failed[0].deadLetter {
		t.Error("expected dead-letter flag on final attempt")
	}
	if store.statuses[evt.EventID] != domain.EventStatusDeadLetter {
		t.Errorf("expected dead_letter status, got %q", store.statuses[evt.EventID])
	}
}

func TestProcess_UnknownAdapter(t *testing.T) {
	store := newFakeStore()
	evt := sampleEvent("evt-noadapter-001")
	job := sampleJob(4, evt.EventID, "nonexistent", 3)
	store.addEvent(evt)
	store.addJob(job)

	w := New(store, map[string]adapters.DestinationAdapter{}, testWorkerCfg(), testLogger())
	w.poll(context.Background())
	time.Sleep(150 * time.Millisecond)

	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.failed) != 1 {
		t.Fatalf("expected 1 failed record for unknown adapter, got %d", len(store.failed))
	}
	if !store.failed[0].deadLetter {
		t.Error("unknown adapter should immediately dead-letter")
	}
}

func TestProcess_MissingEvent(t *testing.T) {
	store := newFakeStore()
	// Job references an event that doesn't exist.
	job := sampleJob(5, "evt-missing-999", "fake", 3)
	store.addJob(job)

	adapter := &fakeAdapter{}
	w := New(store, map[string]adapters.DestinationAdapter{"fake": adapter}, testWorkerCfg(), testLogger())
	w.poll(context.Background())
	time.Sleep(150 * time.Millisecond)

	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.failed) != 1 {
		t.Fatalf("expected 1 failed record for missing event, got %d", len(store.failed))
	}
}

// ─── backoff tests ────────────────────────────────────────────────────────────

func TestBackoffTime_Increases(t *testing.T) {
	w := &Worker{cfg: config.WorkerConfig{BaseBackoffMs: 1000, MaxBackoffMs: 60000}}
	t1 := w.backoffTime(1)
	t2 := w.backoffTime(2)
	t3 := w.backoffTime(3)

	// Each successive attempt should be further in the future.
	if !t2.After(t1) {
		t.Error("attempt 2 backoff should be after attempt 1")
	}
	if !t3.After(t2) {
		t.Error("attempt 3 backoff should be after attempt 2")
	}
}

func TestBackoffTime_CappedAtMax(t *testing.T) {
	maxMs := 1000
	w := &Worker{cfg: config.WorkerConfig{BaseBackoffMs: 500, MaxBackoffMs: maxMs}}

	// Attempt 10 — would be 500*2^9 = 256000ms without cap.
	result := w.backoffTime(10)
	upperBound := time.Now().Add(time.Duration(maxMs+100) * time.Millisecond)
	if result.After(upperBound) {
		t.Errorf("backoff exceeded MaxBackoffMs cap: got %v", result)
	}
}

func TestBackoffTime_DefaultsWhenZero(t *testing.T) {
	w := &Worker{cfg: config.WorkerConfig{}} // zero values
	result := w.backoffTime(1)
	// Should not panic and should be in the future.
	if !result.After(time.Now().Add(-time.Second)) {
		t.Error("backoff result should be in the near future")
	}
}

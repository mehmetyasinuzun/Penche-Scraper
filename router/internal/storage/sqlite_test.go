package storage_test

import (
	"context"
	"encoding/base64"
	"testing"
	"time"

	"github.com/penche/router/internal/domain"
	"github.com/penche/router/internal/storage"
)

func newTestStore(t *testing.T) *storage.Store {
	t.Helper()
	s, err := storage.New(":memory:")
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func sampleEvent() *domain.IncomingEvent {
	return &domain.IncomingEvent{
		EventID:    "evt-001",
		CapturedAt: time.Now().UTC(),
		Domain:     "xss.is",
		PageTitle:  "Test Thread",
		PageURL:    "https://xss.is/threads/1234",
		Screenshot: domain.ScreenshotPayload{
			MIME:   "image/jpeg",
			Base64: base64.StdEncoding.EncodeToString([]byte("fake-jpeg-data")),
		},
		Meta: domain.EventMeta{
			Browser:   "firefox",
			ProfileID: "xss-default",
			Tags:      []string{"cti", "forum"},
		},
	}
}

func TestInsertAndGetEvent(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	evt := sampleEvent()
	stored, err := s.InsertEvent(ctx, evt)
	if err != nil {
		t.Fatalf("insert event: %v", err)
	}
	if stored == nil {
		t.Fatal("expected stored event, got nil")
	}
	if stored.EventID != evt.EventID {
		t.Errorf("event_id mismatch: got %q want %q", stored.EventID, evt.EventID)
	}
	if stored.Status != domain.EventStatusPending {
		t.Errorf("expected pending status, got %q", stored.Status)
	}
}

func TestDuplicateEvent(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	evt := sampleEvent()
	if _, err := s.InsertEvent(ctx, evt); err != nil {
		t.Fatalf("first insert: %v", err)
	}

	_, err := s.InsertEvent(ctx, evt)
	if err == nil {
		t.Fatal("expected ErrDuplicate, got nil")
	}
	if err != storage.ErrDuplicate {
		t.Errorf("expected ErrDuplicate, got: %v", err)
	}
}

func TestCreateAndPollJob(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if _, err := s.InsertEvent(ctx, sampleEvent()); err != nil {
		t.Fatalf("insert: %v", err)
	}

	job, err := s.CreateDeliveryJob(ctx, "evt-001", "taiga", 5)
	if err != nil {
		t.Fatalf("create job: %v", err)
	}
	if job.Status != domain.JobStatusQueued {
		t.Errorf("expected queued, got %q", job.Status)
	}

	jobs, err := s.PollDueJobs(ctx, 10)
	if err != nil {
		t.Fatalf("poll: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(jobs))
	}
}

func TestMarkJobDone(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if _, err := s.InsertEvent(ctx, sampleEvent()); err != nil {
		t.Fatalf("insert: %v", err)
	}
	job, _ := s.CreateDeliveryJob(ctx, "evt-001", "taiga", 5)

	if err := s.MarkJobDone(ctx, job.ID, 1); err != nil {
		t.Fatalf("mark done: %v", err)
	}

	// Should no longer appear in poll.
	jobs, _ := s.PollDueJobs(ctx, 10)
	if len(jobs) != 0 {
		t.Errorf("expected 0 jobs after done, got %d", len(jobs))
	}
}

func TestMarkJobFailed_Retry(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if _, err := s.InsertEvent(ctx, sampleEvent()); err != nil {
		t.Fatalf("insert: %v", err)
	}
	job, _ := s.CreateDeliveryJob(ctx, "evt-001", "taiga", 5)

	nextRun := time.Now().Add(5 * time.Second)
	if err := s.MarkJobFailed(ctx, job.ID, 1, "connection refused", nextRun, false); err != nil {
		t.Fatalf("mark failed: %v", err)
	}

	// Not due yet.
	jobs, _ := s.PollDueJobs(ctx, 10)
	if len(jobs) != 0 {
		t.Errorf("expected 0 due jobs, got %d", len(jobs))
	}
}

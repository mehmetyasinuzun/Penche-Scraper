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

func TestQueryEvents_Filters(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	seed := []struct{ id, domain, title string }{
		{"q0", "xss.is", "Stealer logs for sale"},
		{"q1", "exploit.in", "RDP access EU"},
		{"q2", "xss.is", "Database leak 2026"},
	}
	for _, x := range seed {
		e := sampleEvent()
		e.EventID = x.id
		e.Domain = x.domain
		e.PageTitle = x.title
		if _, err := s.InsertEvent(ctx, e); err != nil {
			t.Fatalf("insert %s: %v", x.id, err)
		}
	}

	_, total, err := s.QueryEvents(ctx, domain.EventFilter{})
	if err != nil {
		t.Fatalf("query all: %v", err)
	}
	if total != 3 {
		t.Errorf("expected total 3, got %d", total)
	}

	items, total, _ := s.QueryEvents(ctx, domain.EventFilter{Domain: "xss.is"})
	if total != 2 || len(items) != 2 {
		t.Errorf("domain filter: expected 2, got total=%d len=%d", total, len(items))
	}
	if !items[0].HasImage || items[0].ImageMIME != "image/jpeg" {
		t.Errorf("expected has_image with jpeg mime, got %v %q", items[0].HasImage, items[0].ImageMIME)
	}

	_, total, _ = s.QueryEvents(ctx, domain.EventFilter{Search: "leak"})
	if total != 1 {
		t.Errorf("search filter: expected 1, got %d", total)
	}

	_, total, _ = s.QueryEvents(ctx, domain.EventFilter{Status: "delivered"})
	if total != 0 {
		t.Errorf("status filter (none delivered): expected 0, got %d", total)
	}

	// Pagination: page size 2 over 3 rows.
	page1, total, _ := s.QueryEvents(ctx, domain.EventFilter{Limit: 2, Offset: 0})
	page2, _, _ := s.QueryEvents(ctx, domain.EventFilter{Limit: 2, Offset: 2})
	if total != 3 || len(page1) != 2 || len(page2) != 1 {
		t.Errorf("pagination: total=%d page1=%d page2=%d", total, len(page1), len(page2))
	}

	dc, err := s.StatsByDomain(ctx)
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	if len(dc) != 2 || dc[0].Domain != "xss.is" || dc[0].Count != 2 {
		t.Errorf("stats by domain unexpected: %+v", dc)
	}
}

func TestGetImageAndDelete(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if _, err := s.InsertEvent(ctx, sampleEvent()); err != nil {
		t.Fatalf("insert: %v", err)
	}
	// A delivery job should be cascade-deleted with its event.
	if _, err := s.CreateDeliveryJob(ctx, "evt-001", "local", 5); err != nil {
		t.Fatalf("job: %v", err)
	}

	mime, data, err := s.GetEventImage(ctx, "evt-001")
	if err != nil {
		t.Fatalf("get image: %v", err)
	}
	if mime != "image/jpeg" || len(data) == 0 {
		t.Errorf("expected jpeg bytes, got %q len=%d", mime, len(data))
	}

	if m, _, _ := s.GetEventImage(ctx, "does-not-exist"); m != "" {
		t.Errorf("expected empty mime for missing event, got %q", m)
	}

	ok, err := s.DeleteEvent(ctx, "evt-001")
	if err != nil || !ok {
		t.Fatalf("delete: ok=%v err=%v", ok, err)
	}
	if again, _ := s.DeleteEvent(ctx, "evt-001"); again {
		t.Error("second delete should report not found")
	}
	jobs, _ := s.PollDueJobs(ctx, 10)
	if len(jobs) != 0 {
		t.Errorf("delivery job should cascade-delete, got %d", len(jobs))
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

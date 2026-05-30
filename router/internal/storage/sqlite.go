package storage

import (
	"context"
	"database/sql"
	"embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/penche/router/internal/domain"
	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// ErrDuplicate is returned when event_id already exists.
var ErrDuplicate = fmt.Errorf("duplicate event_id")

// Store wraps SQLite with all persistence operations.
type Store struct {
	db *sql.DB
}

// New opens (or creates) the SQLite DB and runs all migrations.
func New(dsn string) (*Store, error) {
	db, err := sql.Open("sqlite", dsn+"?_foreign_keys=on&_journal_mode=WAL")
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

// Close shuts down the DB connection.
func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) migrate() error {
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		data, err := migrationsFS.ReadFile("migrations/" + e.Name())
		if err != nil {
			return err
		}
		if _, err := s.db.Exec(string(data)); err != nil {
			return fmt.Errorf("exec %s: %w", e.Name(), err)
		}
	}
	return nil
}

// InsertEvent persists a new captured event.
// Returns ErrDuplicate if event_id already exists.
func (s *Store) InsertEvent(ctx context.Context, evt *domain.IncomingEvent) (*domain.StoredEvent, error) {
	tagsJSON, _ := json.Marshal(evt.Meta.Tags)

	screenshotBytes, err := base64.StdEncoding.DecodeString(evt.Screenshot.Base64)
	if err != nil {
		// try raw URL encoding
		screenshotBytes, err = base64.RawStdEncoding.DecodeString(evt.Screenshot.Base64)
		if err != nil {
			return nil, fmt.Errorf("decode screenshot base64: %w", err)
		}
	}

	now := time.Now().UTC()
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO events
			(event_id, captured_at, domain, page_title, page_url,
			 screenshot_mime, screenshot_data,
			 meta_browser, meta_profile_id, meta_tags,
			 status, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		evt.EventID,
		evt.CapturedAt.UTC().Format(time.RFC3339Nano),
		evt.Domain,
		evt.PageTitle,
		evt.PageURL,
		evt.Screenshot.MIME,
		screenshotBytes,
		evt.Meta.Browser,
		evt.Meta.ProfileID,
		string(tagsJSON),
		string(domain.EventStatusPending),
		now.Format(time.RFC3339Nano),
		now.Format(time.RFC3339Nano),
	)
	if err != nil {
		if isUniqueErr(err) {
			return nil, ErrDuplicate
		}
		return nil, fmt.Errorf("insert event: %w", err)
	}
	return s.GetEvent(ctx, evt.EventID)
}

// GetEvent fetches a stored event by event_id.
func (s *Store) GetEvent(ctx context.Context, eventID string) (*domain.StoredEvent, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, event_id, captured_at, domain, page_title, page_url,
		       screenshot_mime, screenshot_data,
		       meta_browser, meta_profile_id, meta_tags,
		       status, created_at, updated_at
		FROM events WHERE event_id = ?`, eventID)
	return scanEvent(row)
}

// UpdateEventStatus sets a new status on an event.
func (s *Store) UpdateEventStatus(ctx context.Context, eventID string, status domain.EventStatus) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE events SET status = ? WHERE event_id = ?`, string(status), eventID)
	return err
}

// CreateDeliveryJob enqueues a delivery job for the given event.
func (s *Store) CreateDeliveryJob(ctx context.Context, eventID, destination string, maxAttempts int) (*domain.DeliveryJob, error) {
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO delivery_jobs
			(event_id, destination, status, attempt_count, max_attempts,
			 next_run_at, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?)`,
		eventID, destination,
		string(domain.JobStatusQueued),
		0, maxAttempts,
		now.Format(time.RFC3339Nano),
		now.Format(time.RFC3339Nano),
		now.Format(time.RFC3339Nano),
	)
	if err != nil {
		return nil, fmt.Errorf("create delivery job: %w", err)
	}
	id, _ := res.LastInsertId()
	return s.GetJob(ctx, id)
}

// GetJob fetches a delivery job by ID.
func (s *Store) GetJob(ctx context.Context, id int64) (*domain.DeliveryJob, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, event_id, destination, status, attempt_count, max_attempts,
		       next_run_at, last_error, created_at, updated_at
		FROM delivery_jobs WHERE id = ?`, id)
	return scanJob(row)
}

// PollDueJobs returns up to limit jobs ready to be processed.
func (s *Store) PollDueJobs(ctx context.Context, limit int) ([]*domain.DeliveryJob, error) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, event_id, destination, status, attempt_count, max_attempts,
		       next_run_at, last_error, created_at, updated_at
		FROM delivery_jobs
		WHERE status IN ('queued','failed') AND next_run_at <= ?
		ORDER BY next_run_at ASC
		LIMIT ?`, now, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []*domain.DeliveryJob
	for rows.Next() {
		j, err := scanJobRows(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, j)
	}
	return jobs, rows.Err()
}

// MarkJobProcessing claims a job for the worker.
func (s *Store) MarkJobProcessing(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE delivery_jobs SET status = 'processing' WHERE id = ?`, id)
	return err
}

// MarkJobDone records a successful delivery.
func (s *Store) MarkJobDone(ctx context.Context, id int64, attemptNo int) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx,
		`UPDATE delivery_jobs SET status = 'done', attempt_count = attempt_count+1 WHERE id = ?`, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO job_attempts (job_id, attempt_no, status) VALUES (?,?,'done')`,
		id, attemptNo); err != nil {
		return err
	}
	return tx.Commit()
}

// MarkJobFailed records a failed attempt. Set deadLetter=true to move to dead-letter state.
func (s *Store) MarkJobFailed(ctx context.Context, id int64, attemptNo int, errMsg string, nextRunAt time.Time, deadLetter bool) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	status := "failed"
	if deadLetter {
		status = "dead_letter"
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE delivery_jobs
		SET status = ?, attempt_count = attempt_count+1,
		    next_run_at = ?, last_error = ?
		WHERE id = ?`,
		status,
		nextRunAt.UTC().Format(time.RFC3339Nano),
		errMsg, id,
	); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO job_attempts (job_id, attempt_no, status, error) VALUES (?,?,?,?)`,
		id, attemptNo, "failed", errMsg); err != nil {
		return err
	}
	return tx.Commit()
}

// ListEvents returns the most recent events for the gallery view.
func (s *Store) ListEvents(ctx context.Context, limit int) ([]*domain.StoredEvent, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, event_id, captured_at, domain, page_title, page_url,
		       screenshot_mime, screenshot_data,
		       meta_browser, meta_profile_id, meta_tags,
		       status, created_at, updated_at
		FROM events
		ORDER BY captured_at DESC
		LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []*domain.StoredEvent
	for rows.Next() {
		var e domain.StoredEvent
		var capturedAt, createdAt, updatedAt, status string
		if err := rows.Scan(
			&e.ID, &e.EventID, &capturedAt, &e.Domain, &e.PageTitle, &e.PageURL,
			&e.ScreenshotMIME, &e.ScreenshotData,
			&e.MetaBrowser, &e.MetaProfileID, &e.MetaTags,
			&status, &createdAt, &updatedAt,
		); err != nil {
			return nil, err
		}
		e.Status = domain.EventStatus(status)
		e.CapturedAt, _ = time.Parse(time.RFC3339Nano, capturedAt)
		e.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
		e.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
		events = append(events, &e)
	}
	return events, rows.Err()
}

// CountEventsByStatus returns per-status event counts for metrics.
func (s *Store) CountEventsByStatus(ctx context.Context) (map[string]int64, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT status, COUNT(*) FROM events GROUP BY status`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[string]int64)
	for rows.Next() {
		var status string
		var count int64
		if err := rows.Scan(&status, &count); err != nil {
			return nil, err
		}
		result[status] = count
	}
	return result, rows.Err()
}

// QueryEvents returns a filtered, paginated page of event summaries plus the
// total number of rows matching the filter (ignoring limit/offset).
// The screenshot binary is never loaded here — only its presence and MIME.
func (s *Store) QueryEvents(ctx context.Context, f domain.EventFilter) ([]*domain.EventSummary, int, error) {
	var where []string
	var args []any
	if f.Domain != "" {
		where = append(where, "domain = ?")
		args = append(args, f.Domain)
	}
	if f.Status != "" {
		where = append(where, "status = ?")
		args = append(args, f.Status)
	}
	if f.Search != "" {
		where = append(where, "(page_title LIKE ? OR page_url LIKE ? OR domain LIKE ?)")
		like := "%" + f.Search + "%"
		args = append(args, like, like, like)
	}
	clause := ""
	if len(where) > 0 {
		clause = "WHERE " + strings.Join(where, " AND ")
	}

	var total int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM events "+clause, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	limit := f.Limit
	if limit <= 0 || limit > 500 {
		limit = 60
	}
	offset := f.Offset
	if offset < 0 {
		offset = 0
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT event_id, captured_at, domain, page_title, page_url,
		       meta_browser, meta_profile_id, meta_tags, status,
		       screenshot_mime, LENGTH(screenshot_data)
		FROM events `+clause+`
		ORDER BY captured_at DESC
		LIMIT ? OFFSET ?`, append(args, limit, offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	items := make([]*domain.EventSummary, 0, limit)
	for rows.Next() {
		var e domain.EventSummary
		var capturedAt, tagsJSON, status string
		var imgLen int
		if err := rows.Scan(
			&e.EventID, &capturedAt, &e.Domain, &e.PageTitle, &e.PageURL,
			&e.Browser, &e.ProfileID, &tagsJSON, &status,
			&e.ImageMIME, &imgLen,
		); err != nil {
			return nil, 0, err
		}
		e.Status = domain.EventStatus(status)
		e.CapturedAt, _ = time.Parse(time.RFC3339Nano, capturedAt)
		e.HasImage = imgLen > 0
		if err := json.Unmarshal([]byte(tagsJSON), &e.Tags); err != nil || e.Tags == nil {
			e.Tags = []string{}
		}
		items = append(items, &e)
	}
	return items, total, rows.Err()
}

// GetEventImage returns the screenshot MIME and bytes for an event.
// Returns ("", nil, nil) when the event does not exist.
func (s *Store) GetEventImage(ctx context.Context, eventID string) (string, []byte, error) {
	var mime string
	var data []byte
	err := s.db.QueryRowContext(ctx,
		`SELECT screenshot_mime, screenshot_data FROM events WHERE event_id = ?`, eventID).
		Scan(&mime, &data)
	if err == sql.ErrNoRows {
		return "", nil, nil
	}
	if err != nil {
		return "", nil, err
	}
	return mime, data, nil
}

// DeleteEvent removes an event and (via ON DELETE CASCADE) its delivery jobs and
// attempts. Returns false when no matching event existed.
func (s *Store) DeleteEvent(ctx context.Context, eventID string) (bool, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM events WHERE event_id = ?`, eventID)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// StatsByDomain returns capture counts grouped by domain, busiest first.
func (s *Store) StatsByDomain(ctx context.Context) ([]domain.DomainCount, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT domain, COUNT(*) FROM events GROUP BY domain ORDER BY COUNT(*) DESC, domain ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]domain.DomainCount, 0)
	for rows.Next() {
		var d domain.DomainCount
		if err := rows.Scan(&d.Domain, &d.Count); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// --- scan helpers ---

func scanEvent(row *sql.Row) (*domain.StoredEvent, error) {
	var e domain.StoredEvent
	var capturedAt, createdAt, updatedAt, status string
	err := row.Scan(
		&e.ID, &e.EventID, &capturedAt, &e.Domain, &e.PageTitle, &e.PageURL,
		&e.ScreenshotMIME, &e.ScreenshotData,
		&e.MetaBrowser, &e.MetaProfileID, &e.MetaTags,
		&status, &createdAt, &updatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	e.Status = domain.EventStatus(status)
	e.CapturedAt, _ = time.Parse(time.RFC3339Nano, capturedAt)
	e.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	e.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
	return &e, nil
}

func scanJob(row *sql.Row) (*domain.DeliveryJob, error) {
	var j domain.DeliveryJob
	var nextRunAt, createdAt, updatedAt, status string
	err := row.Scan(
		&j.ID, &j.EventID, &j.Destination, &status,
		&j.AttemptCount, &j.MaxAttempts,
		&nextRunAt, &j.LastError, &createdAt, &updatedAt,
	)
	if err != nil {
		return nil, err
	}
	j.Status = domain.JobStatus(status)
	j.NextRunAt, _ = time.Parse(time.RFC3339Nano, nextRunAt)
	j.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	j.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
	return &j, nil
}

func scanJobRows(rows *sql.Rows) (*domain.DeliveryJob, error) {
	var j domain.DeliveryJob
	var nextRunAt, createdAt, updatedAt, status string
	err := rows.Scan(
		&j.ID, &j.EventID, &j.Destination, &status,
		&j.AttemptCount, &j.MaxAttempts,
		&nextRunAt, &j.LastError, &createdAt, &updatedAt,
	)
	if err != nil {
		return nil, err
	}
	j.Status = domain.JobStatus(status)
	j.NextRunAt, _ = time.Parse(time.RFC3339Nano, nextRunAt)
	j.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	j.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
	return &j, nil
}

func isUniqueErr(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}

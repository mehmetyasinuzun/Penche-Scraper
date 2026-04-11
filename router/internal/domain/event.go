package domain

import "time"

// EventStatus represents lifecycle of a captured event.
type EventStatus string

const (
	EventStatusPending    EventStatus = "pending"
	EventStatusDelivered  EventStatus = "delivered"
	EventStatusDeadLetter EventStatus = "dead_letter"
)

// JobStatus represents a single delivery attempt lifecycle.
type JobStatus string

const (
	JobStatusQueued     JobStatus = "queued"
	JobStatusProcessing JobStatus = "processing"
	JobStatusDone       JobStatus = "done"
	JobStatusFailed     JobStatus = "failed"
	JobStatusDeadLetter JobStatus = "dead_letter"
)

// ScreenshotPayload holds the captured image data.
type ScreenshotPayload struct {
	MIME   string `json:"mime"`
	Base64 string `json:"base64"`
}

// EventMeta carries browser and profile metadata.
type EventMeta struct {
	Browser   string   `json:"browser,omitempty"`
	ProfileID string   `json:"profile_id,omitempty"`
	Tags      []string `json:"tags,omitempty"`
}

// IncomingEvent is the raw payload received from the extension.
type IncomingEvent struct {
	EventID    string            `json:"event_id"`
	CapturedAt time.Time         `json:"captured_at"`
	Domain     string            `json:"domain"`
	PageTitle  string            `json:"page_title"`
	PageURL    string            `json:"page_url"`
	Screenshot ScreenshotPayload `json:"screenshot"`
	Meta       EventMeta         `json:"meta"`
}

// StoredEvent is the persisted representation.
type StoredEvent struct {
	ID             int64       `db:"id"`
	EventID        string      `db:"event_id"`
	CapturedAt     time.Time   `db:"captured_at"`
	Domain         string      `db:"domain"`
	PageTitle      string      `db:"page_title"`
	PageURL        string      `db:"page_url"`
	ScreenshotMIME string      `db:"screenshot_mime"`
	ScreenshotData []byte      `db:"screenshot_data"` // stored as binary, NOT logged as text
	MetaBrowser    string      `db:"meta_browser"`
	MetaProfileID  string      `db:"meta_profile_id"`
	MetaTags       string      `db:"meta_tags"` // JSON array string
	Status         EventStatus `db:"status"`
	CreatedAt      time.Time   `db:"created_at"`
	UpdatedAt      time.Time   `db:"updated_at"`
}

// DeliveryJob is a unit of work dispatched to an adapter.
type DeliveryJob struct {
	ID          int64     `db:"id"`
	EventID     string    `db:"event_id"`
	Destination string    `db:"destination"`
	Status      JobStatus `db:"status"`
	AttemptCount int      `db:"attempt_count"`
	MaxAttempts  int      `db:"max_attempts"`
	NextRunAt   time.Time `db:"next_run_at"`
	LastError   string    `db:"last_error"`
	CreatedAt   time.Time `db:"created_at"`
	UpdatedAt   time.Time `db:"updated_at"`
}

// JobAttempt records individual delivery attempts.
type JobAttempt struct {
	ID        int64     `db:"id"`
	JobID     int64     `db:"job_id"`
	AttemptNo int       `db:"attempt_no"`
	Status    JobStatus `db:"status"`
	Error     string    `db:"error"`
	CreatedAt time.Time `db:"created_at"`
}

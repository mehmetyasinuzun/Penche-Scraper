-- Penche Router: initial schema
PRAGMA journal_mode = WAL;
PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS events (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    event_id        TEXT    NOT NULL UNIQUE,
    captured_at     TEXT    NOT NULL, -- ISO8601
    domain          TEXT    NOT NULL,
    page_title      TEXT    NOT NULL,
    page_url        TEXT    NOT NULL,
    screenshot_mime TEXT    NOT NULL,
    screenshot_data BLOB    NOT NULL,
    meta_browser    TEXT    NOT NULL DEFAULT '',
    meta_profile_id TEXT    NOT NULL DEFAULT '',
    meta_tags       TEXT    NOT NULL DEFAULT '[]', -- JSON array
    status          TEXT    NOT NULL DEFAULT 'pending'
                    CHECK(status IN ('pending','delivered','dead_letter')),
    created_at      TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    updated_at      TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE INDEX IF NOT EXISTS idx_events_status      ON events(status);
CREATE INDEX IF NOT EXISTS idx_events_domain      ON events(domain);
CREATE INDEX IF NOT EXISTS idx_events_captured_at ON events(captured_at);

CREATE TABLE IF NOT EXISTS delivery_jobs (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    event_id      TEXT    NOT NULL REFERENCES events(event_id) ON DELETE CASCADE,
    destination   TEXT    NOT NULL,
    status        TEXT    NOT NULL DEFAULT 'queued'
                  CHECK(status IN ('queued','processing','done','failed','dead_letter')),
    attempt_count INTEGER NOT NULL DEFAULT 0,
    max_attempts  INTEGER NOT NULL DEFAULT 5,
    next_run_at   TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    last_error    TEXT    NOT NULL DEFAULT '',
    created_at    TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    updated_at    TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE INDEX IF NOT EXISTS idx_jobs_status      ON delivery_jobs(status, next_run_at);
CREATE INDEX IF NOT EXISTS idx_jobs_event_id    ON delivery_jobs(event_id);

CREATE TABLE IF NOT EXISTS job_attempts (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    job_id     INTEGER NOT NULL REFERENCES delivery_jobs(id) ON DELETE CASCADE,
    attempt_no INTEGER NOT NULL,
    status     TEXT    NOT NULL CHECK(status IN ('done','failed')),
    error      TEXT    NOT NULL DEFAULT '',
    created_at TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE INDEX IF NOT EXISTS idx_attempts_job_id ON job_attempts(job_id);

-- Trigger: update updated_at on events row change
CREATE TRIGGER IF NOT EXISTS trg_events_updated_at
AFTER UPDATE ON events
BEGIN
    UPDATE events SET updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now')
    WHERE id = NEW.id;
END;

-- Trigger: update updated_at on delivery_jobs row change
CREATE TRIGGER IF NOT EXISTS trg_jobs_updated_at
AFTER UPDATE ON delivery_jobs
BEGIN
    UPDATE delivery_jobs SET updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now')
    WHERE id = NEW.id;
END;

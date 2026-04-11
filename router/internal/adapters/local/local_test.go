package local_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/penche/router/internal/adapters/local"
	"github.com/penche/router/internal/config"
	"github.com/penche/router/internal/domain"
)

func TestSend_WritesFiles(t *testing.T) {
	dir := t.TempDir()
	a := local.New(config.LocalConfig{OutputDir: dir})

	evt := &domain.StoredEvent{
		EventID:        "abc12345-0000-0000-0000-000000000000",
		CapturedAt:     time.Date(2026, 4, 11, 14, 32, 1, 0, time.UTC),
		Domain:         "xss.is",
		PageTitle:      "Test Thread",
		PageURL:        "https://xss.is/threads/1",
		ScreenshotMIME: "image/jpeg",
		ScreenshotData: []byte("fake-jpeg-data"),
		MetaProfileID:  "xss-is",
		MetaTags:       `["cti","forum"]`,
	}

	result, err := a.Send(context.Background(), evt)
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}
	if result.ExternalID == "" {
		t.Error("expected non-empty ExternalID")
	}

	// Verify date/domain directory was created.
	dayDir := filepath.Join(dir, "2026-04-11", "xss.is")
	entries, err := os.ReadDir(dayDir)
	if err != nil {
		t.Fatalf("expected output dir to exist: %v", err)
	}

	// Should have exactly one .jpg and one .json.
	var jpgCount, jsonCount int
	for _, e := range entries {
		switch filepath.Ext(e.Name()) {
		case ".jpg":
			jpgCount++
		case ".json":
			jsonCount++
		}
	}
	if jpgCount != 1 {
		t.Errorf("expected 1 jpg, got %d", jpgCount)
	}
	if jsonCount != 1 {
		t.Errorf("expected 1 json, got %d", jsonCount)
	}
}

func TestSend_MetadataContent(t *testing.T) {
	dir := t.TempDir()
	a := local.New(config.LocalConfig{OutputDir: dir})

	ts := time.Date(2026, 4, 11, 9, 5, 0, 0, time.UTC)
	evt := &domain.StoredEvent{
		EventID:        "deadbeef-0000-0000-0000-000000000000",
		CapturedAt:     ts,
		Domain:         "exploit.in",
		PageTitle:      "Forum Post",
		PageURL:        "https://exploit.in/post/42",
		ScreenshotMIME: "image/png",
		ScreenshotData: []byte{0x89, 0x50, 0x4e, 0x47}, // PNG header
		MetaTags:       `["leak"]`,
	}

	if _, err := a.Send(context.Background(), evt); err != nil {
		t.Fatalf("Send: %v", err)
	}

	dayDir := filepath.Join(dir, "2026-04-11", "exploit.in")
	entries, _ := os.ReadDir(dayDir)

	var metaFile string
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".json" {
			metaFile = filepath.Join(dayDir, e.Name())
		}
	}
	if metaFile == "" {
		t.Fatal("metadata file not found")
	}

	raw, err := os.ReadFile(metaFile)
	if err != nil {
		t.Fatalf("read metadata: %v", err)
	}

	var meta local.Metadata
	if err := json.Unmarshal(raw, &meta); err != nil {
		t.Fatalf("unmarshal metadata: %v", err)
	}

	if meta.EventID != evt.EventID {
		t.Errorf("event_id: got %q, want %q", meta.EventID, evt.EventID)
	}
	if meta.Domain != "exploit.in" {
		t.Errorf("domain: got %q", meta.Domain)
	}
	if meta.PageTitle != "Forum Post" {
		t.Errorf("page_title: got %q", meta.PageTitle)
	}
	if len(meta.Tags) != 1 || meta.Tags[0] != "leak" {
		t.Errorf("tags: got %v", meta.Tags)
	}
}

func TestSend_DomainSanitization(t *testing.T) {
	dir := t.TempDir()
	a := local.New(config.LocalConfig{OutputDir: dir})

	evt := &domain.StoredEvent{
		EventID:        "sanitize-test-000000000000000000000",
		CapturedAt:     time.Now().UTC(),
		Domain:         "evil:domain/path",
		PageTitle:      "x",
		PageURL:        "http://evil/",
		ScreenshotMIME: "image/jpeg",
		ScreenshotData: []byte("x"),
		MetaTags:       `[]`,
	}

	if _, err := a.Send(context.Background(), evt); err != nil {
		t.Fatalf("Send: %v", err)
	}

	// Unsafe chars should be replaced — directory must exist.
	day := evt.CapturedAt.UTC().Format("2006-01-02")
	dayDir := filepath.Join(dir, day)
	entries, err := os.ReadDir(dayDir)
	if err != nil {
		t.Fatalf("read day dir: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("expected 1 domain dir, got %d", len(entries))
	}
	// The name must not contain `:` or `/`.
	name := entries[0].Name()
	for _, ch := range []string{":", "/", "\\"} {
		if filepath.Base(name) != name {
			t.Errorf("domain dir name %q is not clean", name)
		}
		_ = ch
	}
}

func TestValidateConfig_EmptyDir(t *testing.T) {
	a := local.New(config.LocalConfig{OutputDir: ""})
	if err := a.ValidateConfig(); err == nil {
		t.Error("expected error for empty output_dir")
	}
}

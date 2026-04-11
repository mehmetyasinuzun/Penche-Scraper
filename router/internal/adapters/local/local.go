// Package local saves each captured event to disk as a PNG screenshot
// and a JSON metadata file, organized by date and domain.
//
// Output structure:
//
//	output/
//	  2026-04-11/
//	    xss.is/
//	      143201_a1b2c3d4.jpg       ← screenshot
//	      143201_a1b2c3d4.json      ← metadata
//	    exploit.in/
//	      ...
package local

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/penche/router/internal/adapters"
	"github.com/penche/router/internal/config"
	"github.com/penche/router/internal/domain"
)

const adapterName = "local"

// Adapter writes captures to a local directory tree.
type Adapter struct {
	cfg config.LocalConfig
}

// New creates a local file adapter.
func New(cfg config.LocalConfig) *Adapter {
	return &Adapter{cfg: cfg}
}

func (a *Adapter) Name() string { return adapterName }

func (a *Adapter) ValidateConfig() error {
	if a.cfg.OutputDir == "" {
		return fmt.Errorf("local: output_dir is required")
	}
	return nil
}

// Metadata is the JSON written alongside each screenshot.
type Metadata struct {
	EventID    string    `json:"event_id"`
	CapturedAt time.Time `json:"captured_at"`
	Domain     string    `json:"domain"`
	PageTitle  string    `json:"page_title"`
	PageURL    string    `json:"page_url"`
	ProfileID  string    `json:"profile_id,omitempty"`
	Tags       []string  `json:"tags,omitempty"`
	Screenshot string    `json:"screenshot_file"`
}

func (a *Adapter) Send(ctx context.Context, evt *domain.StoredEvent) (adapters.DeliveryResult, error) {
	day := evt.CapturedAt.UTC().Format("2006-01-02")
	domainSafe := sanitizeName(evt.Domain)
	dir := filepath.Join(a.cfg.OutputDir, day, domainSafe)

	if err := os.MkdirAll(dir, 0755); err != nil {
		return adapters.DeliveryResult{}, fmt.Errorf("local: create dir %s: %w", dir, err)
	}

	// Base filename: HHMMSS_first8ofID
	ts := evt.CapturedAt.UTC().Format("150405")
	shortID := evt.EventID
	if len(shortID) > 8 {
		shortID = shortID[:8]
	}
	base := fmt.Sprintf("%s_%s", ts, shortID)

	ext := mimeToExt(evt.ScreenshotMIME)
	imgFile := filepath.Join(dir, base+ext)
	jsonFile := filepath.Join(dir, base+".json")

	// Write screenshot.
	if err := os.WriteFile(imgFile, evt.ScreenshotData, 0644); err != nil {
		return adapters.DeliveryResult{}, fmt.Errorf("local: write screenshot: %w", err)
	}

	// Write metadata JSON.
	var tags []string
	_ = json.Unmarshal([]byte(evt.MetaTags), &tags)

	meta := Metadata{
		EventID:    evt.EventID,
		CapturedAt: evt.CapturedAt,
		Domain:     evt.Domain,
		PageTitle:  evt.PageTitle,
		PageURL:    evt.PageURL,
		ProfileID:  evt.MetaProfileID,
		Tags:       tags,
		Screenshot: filepath.Base(imgFile),
	}
	metaBytes, _ := json.MarshalIndent(meta, "", "  ")
	if err := os.WriteFile(jsonFile, metaBytes, 0644); err != nil {
		return adapters.DeliveryResult{}, fmt.Errorf("local: write metadata: %w", err)
	}

	return adapters.DeliveryResult{
		ExternalID: base,
		Message:    fmt.Sprintf("saved to %s", dir),
	}, nil
}

func sanitizeName(s string) string {
	r := strings.NewReplacer(
		"/", "_", "\\", "_", ":", "_", "*", "_",
		"?", "_", "\"", "_", "<", "_", ">", "_", "|", "_",
	)
	return r.Replace(s)
}

func mimeToExt(mime string) string {
	switch mime {
	case "image/png":
		return ".png"
	case "image/webp":
		return ".webp"
	default:
		return ".jpg"
	}
}

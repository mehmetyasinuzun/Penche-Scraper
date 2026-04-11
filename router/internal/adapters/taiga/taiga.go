package taiga

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"

	"github.com/penche/router/internal/adapters"
	"github.com/penche/router/internal/config"
	"github.com/penche/router/internal/domain"
)

const adapterName = "taiga"

// Adapter sends events to Taiga as issues with screenshot attachments.
type Adapter struct {
	cfg       config.TaigaConfig
	client    *http.Client
	projectID int // resolved at startup from slug if needed
}

// New creates a Taiga adapter. Call ValidateConfig() before use.
func New(cfg config.TaigaConfig) *Adapter {
	return &Adapter{
		cfg:       cfg,
		projectID: cfg.ProjectID,
		client:    &http.Client{Timeout: 30 * time.Second},
	}
}

func (a *Adapter) Name() string { return adapterName }

func (a *Adapter) ValidateConfig() error {
	if a.cfg.BaseURL == "" {
		return fmt.Errorf("taiga: base_url is required")
	}
	if a.cfg.AuthToken == "" {
		return fmt.Errorf("taiga: auth_token is required (set PENCHE_TAIGA_TOKEN)")
	}
	if a.cfg.ProjectSlug == "" && a.cfg.ProjectID == 0 {
		return fmt.Errorf("taiga: project_slug or project_id is required (set project_slug to the part after /project/ in your Taiga URL)")
	}
	return nil
}

// ResolveProjectID fetches the numeric project ID from Taiga if only a slug was given.
// Call this once after ValidateConfig, before serving requests.
func (a *Adapter) ResolveProjectID(ctx context.Context) error {
	if a.projectID != 0 {
		return nil // already have it
	}
	if a.cfg.ProjectSlug == "" {
		return fmt.Errorf("taiga: no project_slug to resolve")
	}

	url := fmt.Sprintf("%s/api/v1/projects/by_slug?slug=%s",
		strings.TrimRight(a.cfg.BaseURL, "/"), a.cfg.ProjectSlug)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+a.cfg.AuthToken)

	resp, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("taiga: lookup project by slug: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return fmt.Errorf("taiga: project slug %q not found — check your project_slug in config", a.cfg.ProjectSlug)
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("taiga: project lookup returned %d", resp.StatusCode)
	}

	var result struct {
		ID int `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("taiga: parse project response: %w", err)
	}
	if result.ID == 0 {
		return fmt.Errorf("taiga: got project ID=0 from slug %q", a.cfg.ProjectSlug)
	}

	a.projectID = result.ID
	return nil
}

func (a *Adapter) Send(ctx context.Context, evt *domain.StoredEvent) (adapters.DeliveryResult, error) {
	if a.projectID == 0 {
		if err := a.ResolveProjectID(ctx); err != nil {
			return adapters.DeliveryResult{}, err
		}
	}

	issueRef, err := a.createIssue(ctx, evt)
	if err != nil {
		return adapters.DeliveryResult{}, fmt.Errorf("taiga create issue: %w", err)
	}

	if err := a.attachScreenshot(ctx, issueRef, evt); err != nil {
		return adapters.DeliveryResult{
			ExternalID: issueRef,
			Message:    fmt.Sprintf("issue created but screenshot attachment failed: %v", err),
		}, nil
	}

	return adapters.DeliveryResult{
		ExternalID: issueRef,
		Message:    "issue created with screenshot",
	}, nil
}

type issueCreateRequest struct {
	Project     int      `json:"project"`
	Subject     string   `json:"subject"`
	Description string   `json:"description"`
	Tags        []string `json:"tags,omitempty"`
	Status      *int     `json:"status,omitempty"`
}

func (a *Adapter) createIssue(ctx context.Context, evt *domain.StoredEvent) (string, error) {
	description := fmt.Sprintf(
		"**URL:** %s\n\n**Domain:** %s\n\n**Captured at:** %s\n\n**Profile:** %s",
		evt.PageURL,
		evt.Domain,
		evt.CapturedAt.Format(time.RFC3339),
		evt.MetaProfileID,
	)

	var tags []string
	if err := json.Unmarshal([]byte(evt.MetaTags), &tags); err != nil {
		tags = []string{evt.Domain}
	}

	req := issueCreateRequest{
		Project:     a.projectID,
		Subject:     truncate(evt.PageTitle, 200),
		Description: description,
		Tags:        tags,
	}
	if a.cfg.StatusID != 0 {
		req.Status = &a.cfg.StatusID
	}

	body, _ := json.Marshal(req)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimRight(a.cfg.BaseURL, "/")+"/api/v1/issues", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	a.setHeaders(httpReq)

	resp, err := a.client.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("taiga API %d: %s", resp.StatusCode, truncate(string(respBody), 300))
	}

	var result struct {
		Ref int `json:"ref"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("parse response: %w", err)
	}
	return fmt.Sprintf("%d", result.Ref), nil
}

func (a *Adapter) attachScreenshot(ctx context.Context, issueRef string, evt *domain.StoredEvent) error {
	if len(evt.ScreenshotData) == 0 {
		return nil
	}

	issueID, err := a.getIssueIDByRef(ctx, issueRef)
	if err != nil {
		return fmt.Errorf("lookup issue id: %w", err)
	}

	ext := mimeToExt(evt.ScreenshotMIME)
	filename := fmt.Sprintf("capture_%s%s", evt.EventID[:8], ext)

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	_ = writeField(mw, "project", fmt.Sprintf("%d", a.projectID))
	_ = writeField(mw, "object_id", fmt.Sprintf("%d", issueID))

	part, err := mw.CreateFormFile("attached_file", filename)
	if err != nil {
		return err
	}
	if _, err := part.Write(evt.ScreenshotData); err != nil {
		return err
	}
	mw.Close()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimRight(a.cfg.BaseURL, "/")+"/api/v1/issues/attachments", &body)
	if err != nil {
		return err
	}
	httpReq.Header.Set("Authorization", "Bearer "+a.cfg.AuthToken)
	httpReq.Header.Set("Content-Type", mw.FormDataContentType())

	resp, err := a.client.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		rb, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("attachment API %d: %s", resp.StatusCode, truncate(string(rb), 200))
	}
	return nil
}

func (a *Adapter) getIssueIDByRef(ctx context.Context, ref string) (int, error) {
	url := fmt.Sprintf("%s/api/v1/issues?project=%d&ref=%s",
		strings.TrimRight(a.cfg.BaseURL, "/"), a.projectID, ref)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, err
	}
	a.setHeaders(req)

	resp, err := a.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	var results []struct {
		ID int `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		return 0, err
	}
	if len(results) == 0 {
		return 0, fmt.Errorf("issue ref %s not found", ref)
	}
	return results[0].ID, nil
}

func (a *Adapter) setHeaders(r *http.Request) {
	r.Header.Set("Authorization", "Bearer "+a.cfg.AuthToken)
	r.Header.Set("Content-Type", "application/json")
}

func writeField(mw *multipart.Writer, key, val string) error {
	w, err := mw.CreateFormField(key)
	if err != nil {
		return err
	}
	_, err = w.Write([]byte(val))
	return err
}

func mimeToExt(mime string) string {
	switch mime {
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/webp":
		return ".webp"
	default:
		return ".jpg"
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

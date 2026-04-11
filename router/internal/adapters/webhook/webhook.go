package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/penche/router/internal/adapters"
	"github.com/penche/router/internal/config"
	"github.com/penche/router/internal/domain"
)

const adapterName = "webhook"

// Adapter POSTs a JSON payload to a configurable HTTP endpoint.
type Adapter struct {
	cfg    config.WebhookConfig
	client *http.Client
}

// New creates a webhook adapter.
func New(cfg config.WebhookConfig) *Adapter {
	timeout := time.Duration(cfg.TimeoutMs) * time.Millisecond
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &Adapter{
		cfg:    cfg,
		client: &http.Client{Timeout: timeout},
	}
}

func (a *Adapter) Name() string { return adapterName }

func (a *Adapter) ValidateConfig() error {
	if a.cfg.URL == "" {
		return fmt.Errorf("webhook: url is required")
	}
	return nil
}

// webhookPayload is the JSON body sent to the webhook endpoint.
// Screenshot data is intentionally omitted; only metadata is sent.
type webhookPayload struct {
	EventID    string    `json:"event_id"`
	CapturedAt time.Time `json:"captured_at"`
	Domain     string    `json:"domain"`
	PageTitle  string    `json:"page_title"`
	PageURL    string    `json:"page_url"`
	Browser    string    `json:"browser,omitempty"`
	ProfileID  string    `json:"profile_id,omitempty"`
	Tags       []string  `json:"tags,omitempty"`
}

func (a *Adapter) Send(ctx context.Context, evt *domain.StoredEvent) (adapters.DeliveryResult, error) {
	var tags []string
	_ = json.Unmarshal([]byte(evt.MetaTags), &tags)

	payload := webhookPayload{
		EventID:    evt.EventID,
		CapturedAt: evt.CapturedAt,
		Domain:     evt.Domain,
		PageTitle:  evt.PageTitle,
		PageURL:    evt.PageURL,
		Browser:    evt.MetaBrowser,
		ProfileID:  evt.MetaProfileID,
		Tags:       tags,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return adapters.DeliveryResult{}, fmt.Errorf("marshal payload: %w", err)
	}

	method := a.cfg.Method
	if method == "" {
		method = http.MethodPost
	}

	req, err := http.NewRequestWithContext(ctx, method, a.cfg.URL, bytes.NewReader(body))
	if err != nil {
		return adapters.DeliveryResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range a.cfg.Headers {
		req.Header.Set(k, v)
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return adapters.DeliveryResult{}, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode >= 300 {
		return adapters.DeliveryResult{}, fmt.Errorf("webhook returned %d: %s", resp.StatusCode, string(respBody))
	}

	return adapters.DeliveryResult{
		Message: fmt.Sprintf("webhook delivered, status=%d", resp.StatusCode),
	}, nil
}

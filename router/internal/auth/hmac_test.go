package auth_test

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/penche/router/internal/auth"
)

func TestVerify_ValidSignature(t *testing.T) {
	secret := "test-secret-1234"
	body := []byte(`{"event_id":"abc","page_url":"https://xss.is/threads/1"}`)
	ts := time.Now().Unix()

	sig := auth.Sign(secret, ts, body)

	r := httptest.NewRequest(http.MethodPost, "/v1/events", nil)
	r.Header.Set("X-Penche-Timestamp", strconv.FormatInt(ts, 10))
	r.Header.Set("X-Penche-Signature", sig)

	v := auth.NewVerifier(secret)
	if err := v.Verify(r, body); err != nil {
		t.Fatalf("expected valid, got error: %v", err)
	}
}

func TestVerify_WrongSecret(t *testing.T) {
	body := []byte(`{"event_id":"abc"}`)
	ts := time.Now().Unix()
	sig := auth.Sign("correct-secret", ts, body)

	r := httptest.NewRequest(http.MethodPost, "/", nil)
	r.Header.Set("X-Penche-Timestamp", strconv.FormatInt(ts, 10))
	r.Header.Set("X-Penche-Signature", sig)

	v := auth.NewVerifier("wrong-secret")
	if err := v.Verify(r, body); err == nil {
		t.Fatal("expected error for wrong secret, got nil")
	}
}

func TestVerify_TamperedBody(t *testing.T) {
	secret := "secret"
	originalBody := []byte(`{"event_id":"abc"}`)
	tamperedBody := []byte(`{"event_id":"xyz"}`)
	ts := time.Now().Unix()
	sig := auth.Sign(secret, ts, originalBody)

	r := httptest.NewRequest(http.MethodPost, "/", nil)
	r.Header.Set("X-Penche-Timestamp", strconv.FormatInt(ts, 10))
	r.Header.Set("X-Penche-Signature", sig)

	v := auth.NewVerifier(secret)
	if err := v.Verify(r, tamperedBody); err == nil {
		t.Fatal("expected error for tampered body, got nil")
	}
}

func TestVerify_ExpiredTimestamp(t *testing.T) {
	secret := "secret"
	body := []byte(`{}`)
	// 10 minutes in the past — outside the 5-minute window.
	ts := time.Now().Add(-10 * time.Minute).Unix()
	sig := auth.Sign(secret, ts, body)

	r := httptest.NewRequest(http.MethodPost, "/", nil)
	r.Header.Set("X-Penche-Timestamp", strconv.FormatInt(ts, 10))
	r.Header.Set("X-Penche-Signature", sig)

	v := auth.NewVerifier(secret)
	if err := v.Verify(r, body); err == nil {
		t.Fatal("expected error for expired timestamp, got nil")
	}
}

func TestVerify_MissingHeaders(t *testing.T) {
	v := auth.NewVerifier("secret")
	r := httptest.NewRequest(http.MethodPost, "/", nil)

	if err := v.Verify(r, []byte(`{}`)); err == nil {
		t.Fatal("expected error for missing headers, got nil")
	}
}

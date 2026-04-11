package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"time"
)

const (
	headerTimestamp = "X-Penche-Timestamp"
	headerSignature = "X-Penche-Signature"
	// maxClockSkew defines acceptable timestamp delta to prevent replay attacks.
	maxClockSkew = 5 * time.Minute
)

// Verifier validates HMAC-SHA256 signed requests from the extension.
type Verifier struct {
	secret []byte
}

// NewVerifier creates a Verifier with the given shared secret.
func NewVerifier(secret string) *Verifier {
	return &Verifier{secret: []byte(secret)}
}

// Verify checks headers and validates the HMAC signature over the body.
// Returns an error describing the first validation failure.
func (v *Verifier) Verify(r *http.Request, body []byte) error {
	tsStr := r.Header.Get(headerTimestamp)
	if tsStr == "" {
		return fmt.Errorf("missing %s header", headerTimestamp)
	}
	sig := r.Header.Get(headerSignature)
	if sig == "" {
		return fmt.Errorf("missing %s header", headerSignature)
	}

	tsUnix, err := strconv.ParseInt(tsStr, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid timestamp: %w", err)
	}

	ts := time.Unix(tsUnix, 0)
	skew := time.Since(ts)
	if math.Abs(float64(skew)) > float64(maxClockSkew) {
		return fmt.Errorf("timestamp out of acceptable range (skew=%v)", skew)
	}

	expected := computeSignature(v.secret, tsStr, body)
	if !hmac.Equal([]byte(expected), []byte(sig)) {
		return fmt.Errorf("signature mismatch")
	}

	return nil
}

// Sign produces the HMAC-SHA256 signature string for a given timestamp and body.
// Used in tests and for generating test requests.
func Sign(secret string, tsUnix int64, body []byte) string {
	return computeSignature([]byte(secret), strconv.FormatInt(tsUnix, 10), body)
}

// computeSignature creates HMAC-SHA256(secret, timestamp + "." + body) as hex.
func computeSignature(secret []byte, tsStr string, body []byte) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(tsStr))
	mac.Write([]byte("."))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

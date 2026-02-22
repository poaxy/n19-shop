package stripe

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const signatureTolerance = 300 * time.Second

// VerifyWebhookSignature verifies the Stripe-Signature header against the raw body.
// Returns nil if valid, or an error if invalid/missing.
func (c *Client) VerifyWebhookSignature(body []byte, signatureHeader string) error {
	if signatureHeader == "" {
		return errors.New("missing stripe signature header")
	}
	// Header format: "t=1234567890,v1=hexsignature" (may have multiple v1=)
	parts := strings.Split(signatureHeader, ",")
	var t int64
	var v1 string
	for _, p := range parts {
		kv := strings.SplitN(strings.TrimSpace(p), "=", 2)
		if len(kv) != 2 {
			continue
		}
		switch kv[0] {
		case "t":
			t, _ = strconv.ParseInt(kv[1], 10, 64)
		case "v1":
			v1 = kv[1]
		}
	}
	if v1 == "" || t == 0 {
		return errors.New("invalid stripe signature format")
	}
	// Reject old events
	if time.Since(time.Unix(t, 0)) > signatureTolerance {
		return errors.New("stripe signature timestamp too old")
	}
	payload := fmt.Sprintf("%d.%s", t, string(body))
	mac := hmac.New(sha256.New, []byte(c.webhookSecret))
	mac.Write([]byte(payload))
	expected := hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(expected), []byte(v1)) {
		return errors.New("stripe signature mismatch")
	}
	return nil
}

// checkoutSessionCompletedEvent is the subset of the webhook event we need.
type checkoutSessionCompletedEvent struct {
	Type string `json:"type"`
	Data struct {
		Object struct {
			PaymentStatus string            `json:"payment_status"`
			Metadata      map[string]string `json:"metadata"`
		} `json:"object"`
	} `json:"data"`
}

// FulfillFunc is called to fulfill an order after successful payment.
type FulfillFunc func(ctx context.Context, purchaseID int64) error

// WebhookHandler returns an HTTP handler for Stripe webhooks. It verifies the signature,
// handles checkout.session.completed, and calls fulfill for fulfillment.
func WebhookHandler(c *Client, fulfill FulfillFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			slog.Error("Stripe webhook: read body", "error", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		_ = r.Body.Close()

		if err := c.VerifyWebhookSignature(body, r.Header.Get("Stripe-Signature")); err != nil {
			slog.Warn("Stripe webhook: signature verification failed", "error", err)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		var ev checkoutSessionCompletedEvent
		if err := json.Unmarshal(body, &ev); err != nil {
			slog.Error("Stripe webhook: invalid json", "error", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		if ev.Type != "checkout.session.completed" {
			w.WriteHeader(http.StatusOK)
			return
		}

		if ev.Data.Object.PaymentStatus != "paid" {
			w.WriteHeader(http.StatusOK)
			return
		}

		purchaseIDStr, ok := ev.Data.Object.Metadata["purchase_id"]
		if !ok || purchaseIDStr == "" {
			slog.Error("Stripe webhook: missing purchase_id in metadata")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		purchaseID, err := strconv.ParseInt(purchaseIDStr, 10, 64)
		if err != nil {
			slog.Error("Stripe webhook: invalid purchase_id", "value", purchaseIDStr, "error", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		ctx := r.Context()
		if err := fulfill(ctx, purchaseID); err != nil {
			slog.Error("Stripe webhook: fulfill failed", "purchase_id", purchaseID, "error", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		slog.Info("Stripe webhook: fulfilled purchase", "purchase_id", purchaseID)
		w.WriteHeader(http.StatusOK)
	})
}

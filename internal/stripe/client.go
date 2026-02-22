package stripe

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const apiBase = "https://api.stripe.com/v1"

// Client creates Stripe Checkout Sessions and verifies webhooks.
type Client struct {
	secretKey     string
	webhookSecret string
	httpClient    *http.Client
}

// NewClient returns a Stripe client. webhookSecret is used for VerifyWebhookSignature.
func NewClient(secretKey, webhookSecret string) *Client {
	return &Client{
		secretKey:     secretKey,
		webhookSecret: webhookSecret,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// CreateCheckoutSession creates a Stripe Checkout Session for a one-time payment.
// amountCents is the price in cents (USD). Returns session ID and checkout URL.
func (c *Client) CreateCheckoutSession(ctx context.Context, amountCents int, month int, purchaseID int64, successURL, cancelURL string) (sessionID string, checkoutURL string, err error) {
	form := url.Values{}
	form.Set("mode", "payment")
	form.Set("success_url", successURL)
	form.Set("cancel_url", cancelURL)
	form.Set("metadata[purchase_id]", strconv.FormatInt(purchaseID, 10))
	form.Set("line_items[0][price_data][currency]", "usd")
	form.Set("line_items[0][price_data][unit_amount]", strconv.Itoa(amountCents))
	form.Set("line_items[0][price_data][product_data][name]", fmt.Sprintf("VPN Subscription — %d month(s)", month))
	form.Set("line_items[0][quantity]", "1")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiBase+"/checkout/sessions", strings.NewReader(form.Encode()))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.secretKey)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", err
	}

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("stripe api error %d: %s", resp.StatusCode, string(body))
	}

	// Stripe returns form-encoded or JSON depending on API version; v1 is form-encoded.
	// Try JSON first (newer API).
	var result struct {
		ID  string `json:"id"`
		URL string `json:"url"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		// Fallback: parse as form-encoded (id=...&url=...)
		parsed, _ := url.ParseQuery(string(body))
		if id := parsed.Get("id"); id != "" {
			return id, parsed.Get("url"), nil
		}
		return "", "", fmt.Errorf("parse stripe response: %w", err)
	}
	return result.ID, result.URL, nil
}

// WebhookSecret returns the secret used to verify webhook signatures.
func (c *Client) WebhookSecret() string {
	return c.webhookSecret
}

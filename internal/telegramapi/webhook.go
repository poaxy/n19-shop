package telegramapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type apiResponse[T any] struct {
	OK          bool   `json:"ok"`
	Result      T      `json:"result"`
	Description string `json:"description"`
	ErrorCode   int    `json:"error_code"`
}

func SetWebhook(ctx context.Context, token, webhookURL, secretToken string) error {
	if token == "" {
		return fmt.Errorf("telegram token is empty")
	}
	if webhookURL == "" {
		return fmt.Errorf("webhook url is empty")
	}

	form := url.Values{}
	form.Set("url", webhookURL)
	if secretToken != "" {
		form.Set("secret_token", secretToken)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiBase(token, "setWebhook"), strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var out apiResponse[bool]
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return fmt.Errorf("decode setWebhook response: %w", err)
	}
	if !out.OK {
		return fmt.Errorf("telegram setWebhook failed: %s (code=%d)", out.Description, out.ErrorCode)
	}
	return nil
}

func DeleteWebhook(ctx context.Context, token string) error {
	if token == "" {
		return fmt.Errorf("telegram token is empty")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiBase(token, "deleteWebhook"), nil)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var out apiResponse[bool]
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return fmt.Errorf("decode deleteWebhook response: %w", err)
	}
	if !out.OK {
		return fmt.Errorf("telegram deleteWebhook failed: %s (code=%d)", out.Description, out.ErrorCode)
	}
	return nil
}

func apiBase(token, method string) string {
	return fmt.Sprintf("https://api.telegram.org/bot%s/%s", token, method)
}


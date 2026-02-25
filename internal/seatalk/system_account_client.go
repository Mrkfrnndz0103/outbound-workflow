package seatalk

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type SystemAccountClient struct {
	webhookURL string
	http       *http.Client
}

type systemAccountMessageResponse struct {
	Code      int    `json:"code"`
	Msg       string `json:"msg"`
	MessageID string `json:"message_id"`
}

func NewSystemAccountClient(webhookURL string, timeout time.Duration) *SystemAccountClient {
	trimmed := strings.TrimSpace(webhookURL)
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &SystemAccountClient{
		webhookURL: trimmed,
		http:       &http.Client{Timeout: timeout},
	}
}

func (c *SystemAccountClient) SendText(ctx context.Context, content string, format int) error {
	if strings.TrimSpace(content) == "" {
		return errors.New("content is required")
	}
	if format == 0 {
		format = 1
	}
	if format != 1 && format != 2 {
		return fmt.Errorf("invalid text format %d (allowed: 1 markdown, 2 plain text)", format)
	}

	payload := map[string]any{
		"tag": "text",
		"text": map[string]any{
			"format":  format,
			"content": content,
		},
	}
	return c.send(ctx, payload)
}

func (c *SystemAccountClient) SendImageBase64(ctx context.Context, base64Content string) error {
	if strings.TrimSpace(base64Content) == "" {
		return errors.New("base64 image content is required")
	}

	payload := map[string]any{
		"tag": "image",
		"image_base64": map[string]string{
			"content": base64Content,
		},
	}
	return c.send(ctx, payload)
}

func (c *SystemAccountClient) send(ctx context.Context, payload any) error {
	if c.webhookURL == "" {
		return errors.New("system account webhook URL is required")
	}

	rawBody, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.webhookURL, bytes.NewReader(rawBody))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	res, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	rawResp, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("system account webhook status=%d body=%s", res.StatusCode, string(rawResp))
	}

	var parsed systemAccountMessageResponse
	if err = json.Unmarshal(rawResp, &parsed); err != nil {
		return fmt.Errorf("decode system account response: %w", err)
	}
	if parsed.Code != 0 {
		return fmt.Errorf("system account error code=%d msg=%s", parsed.Code, parsed.Msg)
	}
	return nil
}

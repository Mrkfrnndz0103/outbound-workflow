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
	"sync"
	"time"
)

const (
	openAPICodeOK                 = 0
	openAPICodeAccessTokenExpired = 100
)

type ClientConfig struct {
	AppID     string
	AppSecret string
	BaseURL   string
	Timeout   time.Duration
}

type Messenger interface {
	SendTextToEmployee(ctx context.Context, employeeCode string, content string) error
}

type JoinedGroup struct {
	GroupID   string
	GroupName string
}

type Client struct {
	appID     string
	appSecret string
	baseURL   string
	http      *http.Client

	mu          sync.Mutex
	accessToken string
	expiresAt   time.Time
}

func NewClient(cfg ClientConfig) *Client {
	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	if baseURL == "" {
		baseURL = "https://openapi.seatalk.io"
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &Client{
		appID:     cfg.AppID,
		appSecret: cfg.AppSecret,
		baseURL:   baseURL,
		http:      &http.Client{Timeout: timeout},
	}
}

func (c *Client) SendTextToEmployee(ctx context.Context, employeeCode string, content string) error {
	if employeeCode == "" {
		return errors.New("employeeCode is required")
	}

	requestBody := map[string]any{
		"employee_code": employeeCode,
		"message": map[string]any{
			"tag": "text",
			"text": map[string]string{
				"content": content,
			},
		},
	}

	return c.requestWithAuthRetry(ctx, http.MethodPost, "/messaging/v2/single_chat", requestBody)
}

func (c *Client) SendTextToGroup(ctx context.Context, groupID string, content string, format int) error {
	if strings.TrimSpace(groupID) == "" {
		return errors.New("groupID is required")
	}
	if strings.TrimSpace(content) == "" {
		return errors.New("content is required")
	}
	if format == 0 {
		format = 1
	}
	if format != 1 && format != 2 {
		return fmt.Errorf("invalid text format %d (allowed: 1 markdown, 2 plain text)", format)
	}

	requestBody := map[string]any{
		"group_id": groupID,
		"message": map[string]any{
			"tag": "text",
			"text": map[string]any{
				"format":  format,
				"content": content,
			},
		},
	}

	return c.requestWithAuthRetry(ctx, http.MethodPost, "/messaging/v2/group_chat", requestBody)
}

func (c *Client) SendImageToGroupBase64(ctx context.Context, groupID string, base64Content string) error {
	if strings.TrimSpace(groupID) == "" {
		return errors.New("groupID is required")
	}
	if strings.TrimSpace(base64Content) == "" {
		return errors.New("base64 image content is required")
	}

	requestBody := map[string]any{
		"group_id": groupID,
		"message": map[string]any{
			"tag": "image",
			"image": map[string]any{
				"content": base64Content,
			},
		},
	}

	return c.requestWithAuthRetry(ctx, http.MethodPost, "/messaging/v2/group_chat", requestBody)
}

func (c *Client) ListJoinedGroupChats(ctx context.Context) ([]JoinedGroup, error) {
	token, err := c.getAccessToken(ctx)
	if err != nil {
		return nil, err
	}

	result, err := c.listJoinedGroupChatsWithToken(ctx, token)
	if err != nil {
		return nil, err
	}
	if result.Code == openAPICodeOK {
		return result.Groups, nil
	}
	if result.Code == openAPICodeAccessTokenExpired {
		if err = c.refreshAccessToken(ctx); err != nil {
			return nil, err
		}
		token, err = c.getAccessToken(ctx)
		if err != nil {
			return nil, err
		}
		result, err = c.listJoinedGroupChatsWithToken(ctx, token)
		if err != nil {
			return nil, err
		}
		if result.Code == openAPICodeOK {
			return result.Groups, nil
		}
	}
	return nil, fmt.Errorf("seatalk api error code=%d message=%s", result.Code, result.Message)
}

type joinedGroupsAPIResult struct {
	Code    int
	Message string
	Groups  []JoinedGroup
}

func (c *Client) listJoinedGroupChatsWithToken(ctx context.Context, token string) (joinedGroupsAPIResult, error) {
	raw, err := c.requestRaw(ctx, http.MethodGet, "/messaging/v2/group_chat/joined", nil, token)
	if err != nil {
		return joinedGroupsAPIResult{}, err
	}

	var envelope struct {
		Code    int             `json:"code"`
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data"`
	}
	if err = json.Unmarshal(raw, &envelope); err != nil {
		return joinedGroupsAPIResult{}, fmt.Errorf("decode seatalk response: %w", err)
	}

	groups := extractJoinedGroups(raw, envelope.Data)
	return joinedGroupsAPIResult{
		Code:    envelope.Code,
		Message: envelope.Message,
		Groups:  groups,
	}, nil
}

func extractJoinedGroups(rawTopLevel []byte, rawData json.RawMessage) []JoinedGroup {
	items := make([]JoinedGroup, 0, 16)

	tryParse := func(raw json.RawMessage) bool {
		var rows []struct {
			GroupID   string `json:"group_id"`
			GroupName string `json:"group_name"`
		}
		if err := json.Unmarshal(raw, &rows); err != nil {
			return false
		}
		for _, row := range rows {
			groupID := strings.TrimSpace(row.GroupID)
			if groupID == "" {
				continue
			}
			items = append(items, JoinedGroup{
				GroupID:   groupID,
				GroupName: strings.TrimSpace(row.GroupName),
			})
		}
		return true
	}

	readCandidateArrays := func(raw json.RawMessage) {
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(raw, &obj); err != nil {
			return
		}
		candidates := []string{"group_chats", "groups", "group_chat_list", "list", "items"}
		for _, key := range candidates {
			if payload, ok := obj[key]; ok {
				_ = tryParse(payload)
			}
		}
	}

	readCandidateArrays(rawTopLevel)
	if len(rawData) > 0 {
		if !tryParse(rawData) {
			readCandidateArrays(rawData)
		}
	}

	seen := make(map[string]struct{}, len(items))
	out := make([]JoinedGroup, 0, len(items))
	for _, item := range items {
		key := item.GroupID + "\n" + item.GroupName
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, item)
	}
	return out
}

func (c *Client) requestWithAuthRetry(ctx context.Context, method string, path string, body any) error {
	token, err := c.getAccessToken(ctx)
	if err != nil {
		return err
	}

	resp, err := c.request(ctx, method, path, body, token)
	if err != nil {
		return err
	}
	if resp.Code == openAPICodeOK {
		return nil
	}
	if resp.Code == openAPICodeAccessTokenExpired {
		if err = c.refreshAccessToken(ctx); err != nil {
			return err
		}
		token, err = c.getAccessToken(ctx)
		if err != nil {
			return err
		}
		resp, err = c.request(ctx, method, path, body, token)
		if err != nil {
			return err
		}
		if resp.Code == openAPICodeOK {
			return nil
		}
	}
	return fmt.Errorf("seatalk api error code=%d message=%s", resp.Code, resp.Message)
}

func (c *Client) getAccessToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	needsRefresh := c.accessToken == "" || time.Until(c.expiresAt) < 10*time.Second
	c.mu.Unlock()

	if needsRefresh {
		if err := c.refreshAccessToken(ctx); err != nil {
			return "", err
		}
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	return c.accessToken, nil
}

func (c *Client) refreshAccessToken(ctx context.Context) error {
	reqBody := map[string]string{
		"app_id":     c.appID,
		"app_secret": c.appSecret,
	}

	var parsed authResponse
	if err := c.requestWithoutAuth(ctx, http.MethodPost, "/auth/app_access_token", reqBody, &parsed); err != nil {
		return err
	}
	if parsed.Code != openAPICodeOK {
		return fmt.Errorf(
			"failed to get app access token: code=%d message=%q app_id=%s base_url=%s",
			parsed.Code,
			strings.TrimSpace(parsed.Message),
			maskIdentifier(c.appID, 4),
			c.baseURL,
		)
	}
	if parsed.AppAccessToken == "" || parsed.Expire <= 0 {
		return errors.New("invalid access token response")
	}

	c.mu.Lock()
	c.accessToken = parsed.AppAccessToken
	c.expiresAt = time.Now().Add(time.Duration(parsed.Expire) * time.Second)
	c.mu.Unlock()
	return nil
}

type apiResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type authResponse struct {
	Code           int    `json:"code"`
	Message        string `json:"message"`
	AppAccessToken string `json:"app_access_token"`
	Expire         int    `json:"expire"`
}

func (c *Client) request(ctx context.Context, method string, path string, body any, token string) (apiResponse, error) {
	var parsed apiResponse
	reqBody, err := c.marshalRequestBody(body)
	if err != nil {
		return parsed, err
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bytes.NewReader(reqBody))
	if err != nil {
		return parsed, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "Bearer "+token)

	res, err := c.http.Do(req)
	if err != nil {
		return parsed, err
	}
	defer res.Body.Close()

	raw, err := io.ReadAll(res.Body)
	if err != nil {
		return parsed, err
	}
	if res.StatusCode != http.StatusOK {
		return parsed, fmt.Errorf("seatalk api status=%d body=%s", res.StatusCode, string(raw))
	}
	if err = json.Unmarshal(raw, &parsed); err != nil {
		return parsed, fmt.Errorf("decode seatalk response: %w", err)
	}
	return parsed, nil
}

func (c *Client) requestRaw(ctx context.Context, method string, path string, body any, token string) ([]byte, error) {
	reqBody, err := c.marshalRequestBody(body)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bytes.NewReader(reqBody))
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "Bearer "+token)

	res, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	raw, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("seatalk api status=%d body=%s", res.StatusCode, string(raw))
	}
	return raw, nil
}

func (c *Client) marshalRequestBody(body any) ([]byte, error) {
	if body == nil {
		return nil, nil
	}
	return json.Marshal(body)
}

func (c *Client) requestWithoutAuth(ctx context.Context, method string, path string, body any, out any) error {
	reqBody, err := json.Marshal(body)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bytes.NewReader(reqBody))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	res, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	raw, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("seatalk api status=%d body=%s", res.StatusCode, string(raw))
	}
	if err = json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("decode seatalk response: %w", err)
	}
	return nil
}

func maskIdentifier(v string, keep int) string {
	trimmed := strings.TrimSpace(v)
	if trimmed == "" {
		return trimmed
	}
	if keep < 1 {
		keep = 1
	}
	runes := []rune(trimmed)
	if len(runes) <= keep {
		return trimmed
	}
	return string(runes[:keep]) + "***"
}

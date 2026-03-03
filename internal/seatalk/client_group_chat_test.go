package seatalk

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestClientSendTextToGroup(t *testing.T) {
	var gotAuthHeader string
	var gotPayload map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/app_access_token":
			_, _ = w.Write([]byte(`{"code":0,"message":"ok","app_access_token":"token-1","expire":3600}`))
		case "/messaging/v2/group_chat":
			gotAuthHeader = r.Header.Get("Authorization")
			if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			_, _ = w.Write([]byte(`{"code":0,"message":"ok","message_id":"msg-1"}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := NewClient(ClientConfig{
		AppID:     "app-id",
		AppSecret: "app-secret",
		BaseURL:   server.URL,
		Timeout:   2 * time.Second,
	})

	if err := client.SendTextToGroup(context.Background(), "group-1", "hello", 0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if gotAuthHeader != "Bearer token-1" {
		t.Fatalf("unexpected auth header: %q", gotAuthHeader)
	}
	if gotPayload["group_id"] != "group-1" {
		t.Fatalf("expected group_id group-1, got %v", gotPayload["group_id"])
	}
	msg, ok := gotPayload["message"].(map[string]any)
	if !ok {
		t.Fatalf("message is not object: %T", gotPayload["message"])
	}
	if msg["tag"] != "text" {
		t.Fatalf("expected tag=text, got %v", msg["tag"])
	}
	textObj, ok := msg["text"].(map[string]any)
	if !ok {
		t.Fatalf("text is not object: %T", msg["text"])
	}
	if textObj["content"] != "hello" {
		t.Fatalf("expected content hello, got %v", textObj["content"])
	}
	if textObj["format"] != float64(1) {
		t.Fatalf("expected default format=1, got %v", textObj["format"])
	}
}

func TestClientSendImageToGroupBase64(t *testing.T) {
	var gotPayload map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/app_access_token":
			_, _ = w.Write([]byte(`{"code":0,"message":"ok","app_access_token":"token-1","expire":3600}`))
		case "/messaging/v2/group_chat":
			if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			_, _ = w.Write([]byte(`{"code":0,"message":"ok","message_id":"msg-1"}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := NewClient(ClientConfig{
		AppID:     "app-id",
		AppSecret: "app-secret",
		BaseURL:   server.URL,
		Timeout:   2 * time.Second,
	})

	if err := client.SendImageToGroupBase64(context.Background(), "group-2", "YWJj"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if gotPayload["group_id"] != "group-2" {
		t.Fatalf("expected group_id group-2, got %v", gotPayload["group_id"])
	}
	msg, ok := gotPayload["message"].(map[string]any)
	if !ok {
		t.Fatalf("message is not object: %T", gotPayload["message"])
	}
	if msg["tag"] != "image" {
		t.Fatalf("expected tag=image, got %v", msg["tag"])
	}
	imageObj, ok := msg["image"].(map[string]any)
	if !ok {
		t.Fatalf("image is not object: %T", msg["image"])
	}
	if imageObj["content"] != "YWJj" {
		t.Fatalf("expected image content YWJj, got %v", imageObj["content"])
	}
}

func TestClientListJoinedGroupChats(t *testing.T) {
	var gotAuthHeader string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/app_access_token":
			_, _ = w.Write([]byte(`{"code":0,"message":"ok","app_access_token":"token-1","expire":3600}`))
		case "/messaging/v2/group_chat/joined":
			gotAuthHeader = r.Header.Get("Authorization")
			_, _ = w.Write([]byte(`{"code":0,"message":"ok","data":{"group_chats":[{"group_id":"g-1","group_name":"Group 1"},{"group_id":"g-2","group_name":"Group 2"}]}}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := NewClient(ClientConfig{
		AppID:     "app-id",
		AppSecret: "app-secret",
		BaseURL:   server.URL,
		Timeout:   2 * time.Second,
	})

	groups, err := client.ListJoinedGroupChats(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotAuthHeader != "Bearer token-1" {
		t.Fatalf("unexpected auth header: %q", gotAuthHeader)
	}
	if len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(groups))
	}
	if groups[0].GroupID != "g-1" || groups[1].GroupName != "Group 2" {
		t.Fatalf("unexpected groups: %+v", groups)
	}
}

func TestClientListJoinedGroupChatsRefreshToken(t *testing.T) {
	authCalls := 0
	joinedCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/app_access_token":
			authCalls++
			if authCalls == 1 {
				_, _ = w.Write([]byte(`{"code":0,"message":"ok","app_access_token":"expired-token","expire":3600}`))
				return
			}
			_, _ = w.Write([]byte(`{"code":0,"message":"ok","app_access_token":"fresh-token","expire":3600}`))
		case "/messaging/v2/group_chat/joined":
			joinedCalls++
			if joinedCalls == 1 {
				_, _ = w.Write([]byte(`{"code":100,"message":"token expired"}`))
				return
			}
			_, _ = w.Write([]byte(`{"code":0,"message":"ok","group_chats":[{"group_id":"g-9","group_name":"Group 9"}]}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := NewClient(ClientConfig{
		AppID:     "app-id",
		AppSecret: "app-secret",
		BaseURL:   server.URL,
		Timeout:   2 * time.Second,
	})

	groups, err := client.ListJoinedGroupChats(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(groups) != 1 || groups[0].GroupID != "g-9" {
		t.Fatalf("unexpected groups: %+v", groups)
	}
	if authCalls < 2 {
		t.Fatalf("expected token refresh, authCalls=%d", authCalls)
	}
}

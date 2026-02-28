package seatalk

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSystemAccountClientSendText(t *testing.T) {
	var received map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		_, _ = w.Write([]byte(`{"code":0,"msg":"success","message_id":"id-1"}`))
	}))
	defer server.Close()

	client := NewSystemAccountClient(server.URL, 2*time.Second)
	if err := client.SendText(context.Background(), "hello", 0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if received["tag"] != "text" {
		t.Fatalf("expected tag=text, got %v", received["tag"])
	}
}

func TestSystemAccountClientSendTextWithAtAll(t *testing.T) {
	var received map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		_, _ = w.Write([]byte(`{"code":0,"msg":"success","message_id":"id-1"}`))
	}))
	defer server.Close()

	client := NewSystemAccountClient(server.URL, 2*time.Second)
	if err := client.SendTextWithAtAll(context.Background(), "hello all", 1, true); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if received["at_all"] != true {
		t.Fatalf("expected at_all=true, got %v", received["at_all"])
	}
}

func TestSystemAccountClientSendImageError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":1001,"msg":"bad payload"}`))
	}))
	defer server.Close()

	client := NewSystemAccountClient(server.URL, 2*time.Second)
	err := client.SendImageBase64(context.Background(), "abc")
	if err == nil {
		t.Fatal("expected error for non-zero response code")
	}
}

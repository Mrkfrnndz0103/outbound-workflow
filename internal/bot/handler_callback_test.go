package bot

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/spxph4227/go-bot-server/internal/workflow"
)

type noopRunner struct{}

func (noopRunner) Run(_ string, _ []string) (workflow.Result, error) { return workflow.Result{}, nil }
func (noopRunner) ListWorkflows() []string                           { return nil }

func TestHandleCallbackVerification(t *testing.T) {
	srv := New(Config{SigningSecret: "s3cr3t", CommandPrefix: "/"}, nil, noopRunner{}, log.New(io.Discard, "", 0))
	body := signedCallbackBody(t, "s3cr3t", "event_verification", map[string]any{
		"seatalk_challenge": "abc-123",
	})

	req := httptest.NewRequest(http.MethodPost, "/callback", bytes.NewReader(body.Raw))
	req.Header.Set("Signature", body.Signature)
	rec := httptest.NewRecorder()
	srv.HandleCallback(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["seatalk_challenge"] != "abc-123" {
		t.Fatalf("unexpected challenge response: %#v", resp)
	}
}

func TestHandleCallbackKnownEventTypes(t *testing.T) {
	srv := New(Config{SigningSecret: "s3cr3t", CommandPrefix: "/"}, nil, noopRunner{}, log.New(io.Discard, "", 0))

	tests := []struct {
		name      string
		eventType string
		event     map[string]any
	}{
		{
			name:      "single chat",
			eventType: "message_from_bot_subscriber",
			event: map[string]any{
				"employee_code": "e_1",
				"message": map[string]any{
					"tag":  "text",
					"text": map[string]any{"content": "hello"},
				},
			},
		},
		{
			name:      "new subscriber",
			eventType: "new_bot_subscriber",
			event:     map[string]any{"employee_code": "e_1"},
		},
		{
			name:      "user enter chatroom",
			eventType: "user_enter_chatroom_with_bot",
			event:     map[string]any{"employee_code": "e_1"},
		},
		{
			name:      "group mention",
			eventType: "new_mentioned_message_received_from_group_chat",
			event: map[string]any{
				"group_id": "g1",
				"message": map[string]any{
					"message_id": "m1",
					"thread_id":  "t1",
					"tag":        "text",
					"sender":     map[string]any{"employee_code": "e_1"},
					"text":       map[string]any{"plain_text": "@bot hello"},
				},
			},
		},
		{
			name:      "thread message",
			eventType: "new_message_received_from_thread",
			event: map[string]any{
				"group_id": "g1",
				"message": map[string]any{
					"message_id": "m1",
					"thread_id":  "t1",
					"tag":        "text",
					"sender":     map[string]any{"employee_code": "e_1"},
					"text":       map[string]any{"plain_text": "hello thread"},
				},
			},
		},
		{
			name:      "interactive click",
			eventType: "interactive_message_click",
			event: map[string]any{
				"group_id": "g1",
				"message":  map[string]any{"message_id": "m1", "thread_id": "t1"},
				"clicker":  map[string]any{"employee_code": "e_1"},
			},
		},
		{
			name:      "bot added",
			eventType: "bot_added_to_group_chat",
			event: map[string]any{
				"group":   map[string]any{"group_id": "g1", "group_name": "Ops"},
				"inviter": map[string]any{"employee_code": "e_1"},
			},
		},
		{
			name:      "bot removed",
			eventType: "bot_removed_from_group_chat",
			event: map[string]any{
				"group": map[string]any{"group_id": "g1", "group_name": "Ops"},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			body := signedCallbackBody(t, "s3cr3t", tc.eventType, tc.event)
			req := httptest.NewRequest(http.MethodPost, "/callback", bytes.NewReader(body.Raw))
			req.Header.Set("Signature", body.Signature)
			rec := httptest.NewRecorder()
			srv.HandleCallback(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("expected 200 for %s, got %d", tc.eventType, rec.Code)
			}
		})
	}
}

func TestHandleCallbackUnknownEventType(t *testing.T) {
	srv := New(Config{SigningSecret: "s3cr3t", CommandPrefix: "/"}, nil, noopRunner{}, log.New(io.Discard, "", 0))
	body := signedCallbackBody(t, "s3cr3t", "unknown_event", map[string]any{"foo": "bar"})
	req := httptest.NewRequest(http.MethodPost, "/callback", bytes.NewReader(body.Raw))
	req.Header.Set("Signature", body.Signature)
	rec := httptest.NewRecorder()
	srv.HandleCallback(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rec.Code)
	}
}

func TestHandleCallbackInvalidSignature(t *testing.T) {
	srv := New(Config{SigningSecret: "s3cr3t", CommandPrefix: "/"}, nil, noopRunner{}, log.New(io.Discard, "", 0))
	body := signedCallbackBody(t, "s3cr3t", "message_from_bot_subscriber", map[string]any{
		"employee_code": "e_1",
		"message":       map[string]any{"tag": "text", "text": map[string]any{"content": "hello"}},
	})
	req := httptest.NewRequest(http.MethodPost, "/callback", bytes.NewReader(body.Raw))
	req.Header.Set("Signature", "invalid")
	rec := httptest.NewRecorder()
	srv.HandleCallback(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

type callbackBody struct {
	Raw       []byte
	Signature string
}

func signedCallbackBody(t *testing.T, signingSecret string, eventType string, event map[string]any) callbackBody {
	t.Helper()
	payload := map[string]any{
		"event_id":   "evt-1",
		"event_type": eventType,
		"timestamp":  1700000000,
		"app_id":     "app-1",
		"event":      event,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	sum := sha256.Sum256(append(raw, []byte(signingSecret)...))
	return callbackBody{
		Raw:       raw,
		Signature: hex.EncodeToString(sum[:]),
	}
}

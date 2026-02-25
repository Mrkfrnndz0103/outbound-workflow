package seatalk

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"strings"
)

const (
	EventTypeSingleChat    = "message_from_bot_subscriber"
	EventTypeNewSubscriber = "new_bot_subscriber"
	EventTypeVerification  = "event_verification"
)

type CallbackRequest struct {
	EventID   string          `json:"event_id"`
	EventType string          `json:"event_type"`
	Timestamp int64           `json:"timestamp"`
	AppID     string          `json:"app_id"`
	Event     json.RawMessage `json:"event"`
}

type VerificationEvent struct {
	SeaTalkChallenge string `json:"seatalk_challenge"`
}

type SingleChatEvent struct {
	EmployeeCode string      `json:"employee_code"`
	Message      ChatMessage `json:"message"`
}

type NewSubscriberEvent struct {
	EmployeeCode string `json:"employee_code"`
}

type ChatMessage struct {
	Tag      string      `json:"tag"`
	Text     TextContent `json:"text"`
	Markdown TextContent `json:"markdown"`
}

type TextContent struct {
	Content string `json:"content"`
}

func VerifySignature(rawBody []byte, signingSecret string, signature string) bool {
	if signingSecret == "" || signature == "" {
		return false
	}
	payload := make([]byte, 0, len(rawBody)+len(signingSecret))
	payload = append(payload, rawBody...)
	payload = append(payload, signingSecret...)
	hash := sha256.Sum256(payload)
	expected := hex.EncodeToString(hash[:])
	actual := strings.ToLower(strings.TrimSpace(signature))
	return subtle.ConstantTimeCompare([]byte(expected), []byte(actual)) == 1
}

package seatalk

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"strings"
)

const (
	EventTypeSingleChat                        = "message_from_bot_subscriber"
	EventTypeNewSubscriber                     = "new_bot_subscriber"
	EventTypeVerification                      = "event_verification"
	EventTypeInteractiveMessageClick           = "interactive_message_click"
	EventTypeBotAddedToGroupChat               = "bot_added_to_group_chat"
	EventTypeBotRemovedFromGroupChat           = "bot_removed_from_group_chat"
	EventTypeMentionedMessageFromGroupChat     = "new_mentioned_message_from_group_chat"
	EventTypeMentionedMessageReceivedGroupChat = "new_mentioned_message_received_from_group_chat"
	EventTypeThreadMessageReceived             = "new_message_received_from_thread"
	EventTypeUserEnterChatroomWithBot          = "user_enter_chatroom_with_bot"
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
	SeatalkID    string      `json:"seatalk_id"`
	EmployeeCode string      `json:"employee_code"`
	Email        string      `json:"email"`
	Message      ChatMessage `json:"message"`
}

type NewSubscriberEvent struct {
	EmployeeCode string `json:"employee_code"`
}

type UserEnterChatroomWithBotEvent struct {
	SeatalkID    string `json:"seatalk_id"`
	EmployeeCode string `json:"employee_code"`
	Email        string `json:"email"`
}

type GroupChatActor struct {
	SeatalkID    string `json:"seatalk_id"`
	EmployeeCode string `json:"employee_code"`
	Email        string `json:"email"`
}

type GroupChatInfo struct {
	GroupID       string            `json:"group_id"`
	GroupName     string            `json:"group_name"`
	GroupSettings GroupChatSettings `json:"group_settings"`
}

type GroupChatSettings struct {
	ChatHistoryForNewMembers string `json:"chat_history_for_new_members"`
	CanNotifyWithAtAll       bool   `json:"can_notify_with_at_all"`
	CanViewMemberList        bool   `json:"can_view_member_list"`
}

type BotAddedToGroupChatEvent struct {
	Group   GroupChatInfo  `json:"group"`
	Inviter GroupChatActor `json:"inviter"`
}

type BotRemovedFromGroupChatEvent struct {
	Group GroupChatInfo `json:"group"`
}

type GroupChatMessageEvent struct {
	GroupID string           `json:"group_id"`
	Message GroupChatMessage `json:"message"`
}

type InteractiveMessageClickEvent struct {
	GroupID string            `json:"group_id"`
	Message GroupChatMessage  `json:"message"`
	Clicker GroupChatActor    `json:"clicker"`
	Sender  GroupChatActor    `json:"sender"`
	Action  map[string]any    `json:"action"`
	Extra   map[string]string `json:"extra"`
}

type GroupChatMessage struct {
	MessageID       string            `json:"message_id"`
	QuotedMessageID string            `json:"quoted_message_id"`
	ThreadID        string            `json:"thread_id"`
	Sender          GroupChatActor    `json:"sender"`
	MessageSentTime int64             `json:"message_sent_time"`
	Tag             string            `json:"tag"`
	Text            GroupTextContent  `json:"text"`
	Markdown        GroupTextContent  `json:"markdown"`
	Image           GroupMediaContent `json:"image"`
	Video           GroupMediaContent `json:"video"`
	File            GroupFileContent  `json:"file"`
}

type GroupMediaContent struct {
	Content string `json:"content"`
}

type GroupFileContent struct {
	Content  string `json:"content"`
	Filename string `json:"filename"`
}

type GroupTextContent struct {
	Content       string           `json:"content"`
	PlainText     string           `json:"plain_text"`
	MentionedList []MentionedEntry `json:"mentioned_list"`
}

type MentionedEntry struct {
	Username     string `json:"username"`
	SeatalkID    string `json:"seatalk_id"`
	EmployeeCode string `json:"employee_code"`
	Email        string `json:"email"`
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

package bot

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/spxph4227/go-bot-server/internal/seatalk"
	"github.com/spxph4227/go-bot-server/internal/workflow"
)

const (
	maxCallbackBodySize = 1 << 20
	maxReplyMessageLen  = 1900
)

type Config struct {
	SigningSecret string
	CommandPrefix string
}

type Server struct {
	cfg       Config
	messenger seatalk.Messenger
	runner    workflow.Runner
	logger    *log.Logger
}

func New(cfg Config, messenger seatalk.Messenger, runner workflow.Runner, logger *log.Logger) *Server {
	return &Server{
		cfg:       cfg,
		messenger: messenger,
		runner:    runner,
		logger:    logger,
	}
}

func (s *Server) HandleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (s *Server) HandleCallback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	raw, err := io.ReadAll(io.LimitReader(r.Body, maxCallbackBodySize))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	signature := r.Header.Get("Signature")
	if signature == "" {
		signature = r.Header.Get("X-SeaTalk-Signature")
	}
	if !seatalk.VerifySignature(raw, s.cfg.SigningSecret, signature) {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	var callback seatalk.CallbackRequest
	if err = json.Unmarshal(raw, &callback); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	switch callback.EventType {
	case seatalk.EventTypeVerification:
		s.handleVerification(w, callback.Event)
	case seatalk.EventTypeSingleChat:
		w.WriteHeader(http.StatusOK)
		go s.handleSingleChatEvent(callback.Event)
	case seatalk.EventTypeNewSubscriber:
		w.WriteHeader(http.StatusOK)
		go s.handleNewSubscriberEvent(callback.Event)
	case seatalk.EventTypeUserEnterChatroomWithBot:
		w.WriteHeader(http.StatusOK)
		go s.handleUserEnterChatroomWithBotEvent(callback.Event)
	case seatalk.EventTypeMentionedMessageFromGroupChat, seatalk.EventTypeMentionedMessageReceivedGroupChat:
		w.WriteHeader(http.StatusOK)
		go s.handleMentionedGroupMessageEvent(callback.EventType, callback.Event)
	case seatalk.EventTypeThreadMessageReceived:
		w.WriteHeader(http.StatusOK)
		go s.handleThreadMessageEvent(callback.Event)
	case seatalk.EventTypeInteractiveMessageClick:
		w.WriteHeader(http.StatusOK)
		go s.handleInteractiveMessageClickEvent(callback.Event)
	case seatalk.EventTypeBotAddedToGroupChat:
		w.WriteHeader(http.StatusOK)
		go s.handleBotAddedToGroupChatEvent(callback.Event)
	case seatalk.EventTypeBotRemovedFromGroupChat:
		w.WriteHeader(http.StatusOK)
		go s.handleBotRemovedFromGroupChatEvent(callback.Event)
	default:
		s.logger.Printf("ignored callback event_type=%q event_id=%q", callback.EventType, callback.EventID)
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *Server) handleVerification(w http.ResponseWriter, eventRaw json.RawMessage) {
	var event seatalk.VerificationEvent
	if err := json.Unmarshal(eventRaw, &event); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"seatalk_challenge": event.SeaTalkChallenge,
	})
}

func (s *Server) handleSingleChatEvent(eventRaw json.RawMessage) {
	var event seatalk.SingleChatEvent
	if err := json.Unmarshal(eventRaw, &event); err != nil {
		s.logger.Printf("invalid single chat event: %v", err)
		return
	}

	content := event.Message.Text.Content
	if strings.EqualFold(event.Message.Tag, "markdown") {
		content = event.Message.Markdown.Content
	}

	cmd, ok := parseCommand(content, s.cfg.CommandPrefix)
	if !ok {
		return
	}

	reply := s.executeCommand(cmd)
	if s.messenger == nil {
		s.logger.Printf("outbound reply disabled; command=%q employee_code=%s result=%q", cmd.name, event.EmployeeCode, truncate(reply, 200))
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := s.messenger.SendTextToEmployee(ctx, event.EmployeeCode, truncate(reply, maxReplyMessageLen)); err != nil {
		s.logger.Printf("send reply failed: %v", err)
	}
}

func (s *Server) handleNewSubscriberEvent(eventRaw json.RawMessage) {
	var event seatalk.NewSubscriberEvent
	if err := json.Unmarshal(eventRaw, &event); err != nil {
		s.logger.Printf("invalid new subscriber event: %v", err)
		return
	}
	s.sendWelcomeMessage(event.EmployeeCode, "new_bot_subscriber")
}

func (s *Server) handleUserEnterChatroomWithBotEvent(eventRaw json.RawMessage) {
	var event seatalk.UserEnterChatroomWithBotEvent
	if err := json.Unmarshal(eventRaw, &event); err != nil {
		s.logger.Printf("invalid user enter chatroom event: %v", err)
		return
	}
	s.sendWelcomeMessage(event.EmployeeCode, "user_enter_chatroom_with_bot")
}

func (s *Server) sendWelcomeMessage(employeeCode string, source string) {
	if strings.TrimSpace(employeeCode) == "" {
		s.logger.Printf("skip welcome message source=%s employee_code is empty", source)
		return
	}
	if s.messenger == nil {
		s.logger.Printf("outbound reply disabled; skip welcome message source=%s employee_code=%s", source, employeeCode)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	message := "Welcome. Commands: /help, /list, /run <workflow> [args...]"
	if s.cfg.CommandPrefix != "/" {
		message = fmt.Sprintf(
			"Welcome. Commands: %shelp, %slist, %srun <workflow> [args...]",
			s.cfg.CommandPrefix,
			s.cfg.CommandPrefix,
			s.cfg.CommandPrefix,
		)
	}

	if err := s.messenger.SendTextToEmployee(ctx, employeeCode, message); err != nil {
		s.logger.Printf("send welcome message failed: %v", err)
	}
}

func (s *Server) handleMentionedGroupMessageEvent(eventType string, eventRaw json.RawMessage) {
	var event seatalk.GroupChatMessageEvent
	if err := json.Unmarshal(eventRaw, &event); err != nil {
		s.logger.Printf("invalid %s event: %v", eventType, err)
		return
	}
	s.logger.Printf(
		"event=%s group_id=%s thread_id=%s sender=%s message_id=%s tag=%s text=%q",
		eventType,
		event.GroupID,
		event.Message.ThreadID,
		event.Message.Sender.EmployeeCode,
		event.Message.MessageID,
		event.Message.Tag,
		truncate(groupMessageText(event.Message), 200),
	)
}

func (s *Server) handleThreadMessageEvent(eventRaw json.RawMessage) {
	var event seatalk.GroupChatMessageEvent
	if err := json.Unmarshal(eventRaw, &event); err != nil {
		s.logger.Printf("invalid new_message_received_from_thread event: %v", err)
		return
	}
	s.logger.Printf(
		"event=new_message_received_from_thread group_id=%s thread_id=%s sender=%s message_id=%s tag=%s text=%q",
		event.GroupID,
		event.Message.ThreadID,
		event.Message.Sender.EmployeeCode,
		event.Message.MessageID,
		event.Message.Tag,
		truncate(groupMessageText(event.Message), 200),
	)
}

func (s *Server) handleInteractiveMessageClickEvent(eventRaw json.RawMessage) {
	var event seatalk.InteractiveMessageClickEvent
	if err := json.Unmarshal(eventRaw, &event); err != nil {
		s.logger.Printf("invalid interactive_message_click event: %v", err)
		return
	}
	actor := firstNonEmpty(
		strings.TrimSpace(event.Clicker.EmployeeCode),
		strings.TrimSpace(event.Sender.EmployeeCode),
	)
	s.logger.Printf(
		"event=interactive_message_click group_id=%s actor=%s thread_id=%s message_id=%s",
		event.GroupID,
		actor,
		event.Message.ThreadID,
		event.Message.MessageID,
	)
}

func (s *Server) handleBotAddedToGroupChatEvent(eventRaw json.RawMessage) {
	var event seatalk.BotAddedToGroupChatEvent
	if err := json.Unmarshal(eventRaw, &event); err != nil {
		s.logger.Printf("invalid bot_added_to_group_chat event: %v", err)
		return
	}
	s.logger.Printf(
		"event=bot_added_to_group_chat group_id=%s group_name=%q inviter_employee=%s inviter_seatalk_id=%s inviter_email=%s chat_history_for_new_members=%q can_notify_with_at_all=%t can_view_member_list=%t",
		event.Group.GroupID,
		event.Group.GroupName,
		event.Inviter.EmployeeCode,
		event.Inviter.SeatalkID,
		event.Inviter.Email,
		event.Group.GroupSettings.ChatHistoryForNewMembers,
		event.Group.GroupSettings.CanNotifyWithAtAll,
		event.Group.GroupSettings.CanViewMemberList,
	)
}

func (s *Server) handleBotRemovedFromGroupChatEvent(eventRaw json.RawMessage) {
	var event seatalk.BotRemovedFromGroupChatEvent
	if err := json.Unmarshal(eventRaw, &event); err != nil {
		s.logger.Printf("invalid bot_removed_from_group_chat event: %v", err)
		return
	}
	s.logger.Printf(
		"event=bot_removed_from_group_chat group_id=%s group_name=%q",
		event.Group.GroupID,
		event.Group.GroupName,
	)
}

func groupMessageText(message seatalk.GroupChatMessage) string {
	if strings.TrimSpace(message.Text.PlainText) != "" {
		return message.Text.PlainText
	}
	if strings.TrimSpace(message.Text.Content) != "" {
		return message.Text.Content
	}
	if strings.TrimSpace(message.Markdown.Content) != "" {
		return message.Markdown.Content
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func (s *Server) executeCommand(cmd command) string {
	switch cmd.name {
	case "help":
		return s.helpMessage()
	case "list":
		workflows := s.runner.ListWorkflows()
		if len(workflows) == 0 {
			return "No workflows configured."
		}
		return "Available workflows: " + strings.Join(workflows, ", ")
	case "run":
		if len(cmd.args) == 0 {
			return fmt.Sprintf("Usage: %srun <workflow> [args...]", s.cfg.CommandPrefix)
		}
		workflowName := cmd.args[0]
		extraArgs := cmd.args[1:]
		result, err := s.runner.Run(workflowName, extraArgs)
		if err != nil {
			output := strings.TrimSpace(result.Output)
			if output == "" {
				return fmt.Sprintf("Workflow failed: %v", err)
			}
			return fmt.Sprintf("Workflow failed: %v\nOutput:\n%s", err, output)
		}

		output := strings.TrimSpace(result.Output)
		if output == "" {
			return fmt.Sprintf("Workflow %q completed successfully.", result.Workflow)
		}
		return fmt.Sprintf("Workflow %q completed.\nOutput:\n%s", result.Workflow, output)
	default:
		return fmt.Sprintf("Unknown command. Use %shelp", s.cfg.CommandPrefix)
	}
}

func (s *Server) helpMessage() string {
	prefix := s.cfg.CommandPrefix
	return strings.Join([]string{
		"SeaTalk Workflow Bot commands:",
		prefix + "help",
		prefix + "list",
		prefix + "run <workflow> [args...]",
	}, "\n")
}

func truncate(input string, maxLen int) string {
	if len(input) <= maxLen {
		return input
	}
	return input[:maxLen-3] + "..."
}

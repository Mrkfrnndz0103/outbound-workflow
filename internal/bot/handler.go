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
	default:
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
	if s.messenger == nil {
		s.logger.Printf("outbound reply disabled; skip welcome message for employee_code=%s", event.EmployeeCode)
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

	if err := s.messenger.SendTextToEmployee(ctx, event.EmployeeCode, message); err != nil {
		s.logger.Printf("send welcome message failed: %v", err)
	}
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

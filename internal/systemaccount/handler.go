package systemaccount

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/spxph4227/go-bot-server/internal/seatalk"
)

const maxBodySize = 8 << 20

type Server struct {
	client *seatalk.SystemAccountClient
	logger *log.Logger
}

type sendTextRequest struct {
	Content string `json:"content"`
	Format  int    `json:"format"`
}

type sendImageRequest struct {
	Content       string `json:"content"`
	Base64Content string `json:"base64_content"`
}

func New(client *seatalk.SystemAccountClient, logger *log.Logger) *Server {
	return &Server{
		client: client,
		logger: logger,
	}
}

func (s *Server) HandleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (s *Server) HandleSendText(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var req sendTextRequest
	if err := decodeJSON(r, &req); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	if strings.TrimSpace(req.Content) == "" {
		writeJSONError(w, http.StatusBadRequest, "content is required")
		return
	}

	if err := s.client.SendText(r.Context(), req.Content, req.Format); err != nil {
		s.logger.Printf("system account send text failed: %v", err)
		writeJSONError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSONOK(w)
}

func (s *Server) HandleSendImage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var req sendImageRequest
	if err := decodeJSON(r, &req); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	content := strings.TrimSpace(req.Base64Content)
	if content == "" {
		content = strings.TrimSpace(req.Content)
	}
	if content == "" {
		writeJSONError(w, http.StatusBadRequest, "base64_content is required")
		return
	}

	if err := s.client.SendImageBase64(r.Context(), content); err != nil {
		s.logger.Printf("system account send image failed: %v", err)
		writeJSONError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSONOK(w)
}

func decodeJSON(r *http.Request, out any) error {
	raw, err := io.ReadAll(io.LimitReader(r.Body, maxBodySize))
	if err != nil {
		return err
	}
	if len(raw) == 0 {
		return errors.New("request body is required")
	}
	if err = json.Unmarshal(raw, out); err != nil {
		return errors.New("invalid JSON body")
	}
	return nil
}

func writeJSONOK(w http.ResponseWriter) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]any{
		"ok":    false,
		"error": message,
	})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

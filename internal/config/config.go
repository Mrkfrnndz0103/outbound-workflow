package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

const (
	ModeBot           = "bot"
	ModeSystemAccount = "system_account"
)

type Config struct {
	Mode                    string
	Port                    string
	SeaTalkAppID            string
	SeaTalkAppSecret        string
	SeaTalkSigningSecret    string
	SeaTalkSystemWebhookURL string
	SeaTalkBaseURL          string
	WorkflowsFile           string
	CommandPrefix           string
	HTTPTimeout             time.Duration
	DefaultWorkflowTimeout  time.Duration
}

func LoadFromEnv() (Config, error) {
	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		return Config{}, fmt.Errorf("load .env: %w", err)
	}

	cfg := Config{
		Mode:                    strings.ToLower(firstNonEmpty(os.Getenv("SEATALK_MODE"), ModeBot)),
		Port:                    firstNonEmpty(os.Getenv("PORT"), "8080"),
		SeaTalkAppID:            os.Getenv("SEATALK_APP_ID"),
		SeaTalkAppSecret:        os.Getenv("SEATALK_APP_SECRET"),
		SeaTalkSigningSecret:    os.Getenv("SEATALK_SIGNING_SECRET"),
		SeaTalkSystemWebhookURL: os.Getenv("SEATALK_SYSTEM_WEBHOOK_URL"),
		SeaTalkBaseURL:          firstNonEmpty(os.Getenv("SEATALK_BASE_URL"), "https://openapi.seatalk.io"),
		WorkflowsFile:           firstNonEmpty(os.Getenv("WORKFLOWS_FILE"), "workflows.yaml"),
		CommandPrefix:           firstNonEmpty(os.Getenv("BOT_COMMAND_PREFIX"), "/"),
		HTTPTimeout:             getDurationSeconds("SEATALK_HTTP_TIMEOUT_SECONDS", 10),
		DefaultWorkflowTimeout:  getDurationSeconds("WORKFLOW_DEFAULT_TIMEOUT_SECONDS", 120),
	}

	switch cfg.Mode {
	case ModeBot:
		if (cfg.SeaTalkAppID == "") != (cfg.SeaTalkAppSecret == "") {
			return Config{}, fmt.Errorf("set both SEATALK_APP_ID and SEATALK_APP_SECRET, or set neither")
		}
		if cfg.SeaTalkSigningSecret == "" {
			return Config{}, errors.New("SEATALK_SIGNING_SECRET is required in bot mode")
		}
	case ModeSystemAccount:
		if strings.TrimSpace(cfg.SeaTalkSystemWebhookURL) == "" {
			return Config{}, errors.New("SEATALK_SYSTEM_WEBHOOK_URL is required in system_account mode")
		}
	default:
		return Config{}, fmt.Errorf("unsupported SEATALK_MODE %q (allowed: %s, %s)", cfg.Mode, ModeBot, ModeSystemAccount)
	}

	return cfg, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func getDurationSeconds(envKey string, fallback int) time.Duration {
	raw := os.Getenv(envKey)
	if raw == "" {
		return time.Duration(fallback) * time.Second
	}
	seconds, err := strconv.Atoi(raw)
	if err != nil || seconds <= 0 {
		return time.Duration(fallback) * time.Second
	}
	return time.Duration(seconds) * time.Second
}

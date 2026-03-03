package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"github.com/spxph4227/go-bot-server/internal/botconfig"
	"github.com/spxph4227/go-bot-server/internal/seatalk"
	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"
)

const (
	defaultSyncHTTPTimeout = 10 * time.Second
	defaultSyncBaseURL     = "https://openapi.seatalk.io"
)

type syncConfig struct {
	SheetID               string
	Tab                   string
	GoogleCredentialsFile string
	GoogleCredentialsJSON string
	SeaTalkBaseURL        string
	HTTPTimeout           time.Duration
}

type syncTarget struct {
	Workflow  string
	AppID     string
	AppSecret string
}

func main() {
	logger := log.New(os.Stdout, "[bot-config-group-sync] ", log.LstdFlags|log.Lmsgprefix)

	cfg, err := loadConfig()
	if err != nil {
		logger.Fatalf("config error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.HTTPTimeout)
	defer cancel()

	sheetsSvc, err := newSheetsService(ctx, cfg.GoogleCredentialsFile, cfg.GoogleCredentialsJSON)
	if err != nil {
		logger.Fatalf("google sheets init error: %v", err)
	}

	rows, err := botconfig.LoadRowsFromSheet(ctx, sheetsSvc, cfg.SheetID, cfg.Tab)
	if err != nil {
		logger.Fatalf("load bot_config rows error: %v", err)
	}
	targets, err := buildSyncTargets(rows)
	if err != nil {
		logger.Fatalf("build sync targets error: %v", err)
	}

	owned := make([]botconfig.OwnedGroup, 0, 64)
	for _, target := range targets {
		client := seatalk.NewClient(seatalk.ClientConfig{
			AppID:     target.AppID,
			AppSecret: target.AppSecret,
			BaseURL:   cfg.SeaTalkBaseURL,
			Timeout:   cfg.HTTPTimeout,
		})
		groups, listErr := client.ListJoinedGroupChats(ctx)
		if listErr != nil {
			logger.Fatalf("list joined groups workflow=%s error: %v", target.Workflow, listErr)
		}
		for _, group := range groups {
			groupID := strings.TrimSpace(group.GroupID)
			if groupID == "" {
				continue
			}
			owned = append(owned, botconfig.OwnedGroup{
				GroupID: groupID,
				BotName: target.Workflow,
			})
		}
	}

	finalRows := botconfig.BuildOwnedGroupRows(owned)
	if err = replaceOwnedGroupInventory(ctx, sheetsSvc, cfg.SheetID, cfg.Tab, finalRows); err != nil {
		logger.Fatalf("write inventory error: %v", err)
	}

	logger.Printf("sync completed targets=%d rows_written=%d", len(targets), len(finalRows))
}

func loadConfig() (syncConfig, error) {
	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		return syncConfig{}, fmt.Errorf("load .env: %w", err)
	}

	sheetID := strings.TrimSpace(os.Getenv("BOT_CONFIG_SHEET_ID"))
	if sheetID == "" {
		return syncConfig{}, errors.New("BOT_CONFIG_SHEET_ID is required")
	}

	credsJSON := strings.TrimSpace(firstNonEmpty(
		os.Getenv("WF21_GOOGLE_CREDENTIALS_JSON"),
		os.Getenv("WF1_GOOGLE_CREDENTIALS_JSON"),
		os.Getenv("WF2_GOOGLE_CREDENTIALS_JSON"),
	))
	credsFile := firstNonEmpty(
		strings.TrimSpace(os.Getenv("WF21_GOOGLE_CREDENTIALS_FILE")),
		strings.TrimSpace(os.Getenv("WF1_GOOGLE_CREDENTIALS_FILE")),
		strings.TrimSpace(os.Getenv("WF2_GOOGLE_CREDENTIALS_FILE")),
		strings.TrimSpace(os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")),
	)
	if credsJSON == "" && credsFile == "" {
		return syncConfig{}, errors.New("set one of WF21/WF1/WF2 google credentials json/file or GOOGLE_APPLICATION_CREDENTIALS")
	}

	return syncConfig{
		SheetID:               sheetID,
		Tab:                   botconfig.NormalizeTabName(os.Getenv("BOT_CONFIG_TAB")),
		GoogleCredentialsFile: credsFile,
		GoogleCredentialsJSON: credsJSON,
		SeaTalkBaseURL:        strings.TrimSpace(firstNonEmpty(os.Getenv("BOT_CONFIG_SYNC_BASE_URL"), defaultSyncBaseURL)),
		HTTPTimeout:           getDurationSeconds("BOT_CONFIG_SYNC_HTTP_TIMEOUT_SECONDS", int(defaultSyncHTTPTimeout/time.Second)),
	}, nil
}

func buildSyncTargets(rows []botconfig.Row) ([]syncTarget, error) {
	targets := make([]syncTarget, 0, len(rows))
	seen := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		if row.Mode != "bot" {
			continue
		}
		workflow := strings.ToLower(strings.TrimSpace(row.Workflow))
		if workflow == "" || strings.TrimSpace(row.AppID) == "" || strings.TrimSpace(row.AppSecret) == "" {
			continue
		}
		if _, ok := seen[workflow]; ok {
			return nil, fmt.Errorf("duplicate bot_config workflow for sync target: %s", workflow)
		}
		seen[workflow] = struct{}{}
		targets = append(targets, syncTarget{
			Workflow:  workflow,
			AppID:     row.AppID,
			AppSecret: row.AppSecret,
		})
	}
	return targets, nil
}

func replaceOwnedGroupInventory(ctx context.Context, sheetsSvc *sheets.Service, sheetID, tab string, groups []botconfig.OwnedGroup) error {
	rangeDE := fmt.Sprintf("%s!D2:E", botconfig.NormalizeTabName(tab))
	if _, err := sheetsSvc.Spreadsheets.Values.Clear(sheetID, rangeDE, &sheets.ClearValuesRequest{}).Context(ctx).Do(); err != nil {
		return fmt.Errorf("clear %s: %w", rangeDE, err)
	}
	if len(groups) == 0 {
		return nil
	}

	values := make([][]interface{}, 0, len(groups))
	for _, item := range groups {
		values = append(values, []interface{}{item.GroupID, item.BotName})
	}

	body := &sheets.ValueRange{
		Range:  fmt.Sprintf("%s!D2:E", botconfig.NormalizeTabName(tab)),
		Values: values,
	}
	if _, err := sheetsSvc.Spreadsheets.Values.Update(sheetID, fmt.Sprintf("%s!D2", botconfig.NormalizeTabName(tab)), body).
		ValueInputOption("RAW").
		Context(ctx).
		Do(); err != nil {
		return fmt.Errorf("write owned group inventory: %w", err)
	}
	return nil
}

func newSheetsService(ctx context.Context, credsFile, credsJSON string) (*sheets.Service, error) {
	options := []option.ClientOption{
		option.WithScopes(sheets.SpreadsheetsScope),
	}
	if strings.TrimSpace(credsJSON) != "" {
		options = append(options, option.WithCredentialsJSON([]byte(credsJSON)))
	} else {
		options = append(options, option.WithCredentialsFile(credsFile))
	}
	return sheets.NewService(ctx, options...)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func getDurationSeconds(key string, fallback int) time.Duration {
	if fallback <= 0 {
		fallback = 10
	}
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return time.Duration(fallback) * time.Second
	}
	parsed, err := time.ParseDuration(raw + "s")
	if err != nil || parsed <= 0 {
		return time.Duration(fallback) * time.Second
	}
	return parsed
}

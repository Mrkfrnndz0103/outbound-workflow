package main

import (
	"context"
	"log"
	"os"
	"testing"

	"github.com/spxph4227/go-bot-server/internal/seatalk"
)

const wf3ManualTestGroupID = "NTMwNTQyNTc2OTg4"

func TestManualWF3Send(t *testing.T) {
	if os.Getenv("WF3_RUN_MANUAL_SEND_TEST") != "1" {
		t.Skip("set WF3_RUN_MANUAL_SEND_TEST=1 to run manual send")
	}

	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	cfg.Continuous = false
	cfg.TestSendOnce = true
	cfg.DryRun = false
	cfg.SeaTalkGroupID = wf3ManualTestGroupID

	state := workflowState{}
	stateExists := false

	sheetsSvc, err := newSheetsService(context.Background(), cfg.GoogleCredentialsFile, cfg.GoogleCredentialsJSON)
	if err != nil {
		t.Fatalf("new sheets service: %v", err)
	}
	authHTTPClient, err := newGoogleAuthenticatedHTTPClient(context.Background(), cfg.GoogleCredentialsFile, cfg.GoogleCredentialsJSON)
	if err != nil {
		t.Fatalf("new google auth http client: %v", err)
	}
	seaTalkClient := seatalk.NewClient(seatalk.ClientConfig{
		AppID:     cfg.SeaTalkAppID,
		AppSecret: cfg.SeaTalkAppSecret,
		BaseURL:   cfg.SeaTalkBaseURL,
		Timeout:   cfg.HTTPTimeout,
	})

	logger := log.New(os.Stdout, "[wf3-manual-test] ", log.LstdFlags|log.Lmsgprefix)
	if err = runCycle(context.Background(), cfg, sheetsSvc, authHTTPClient, seaTalkClient, &state, &stateExists, logger, 1); err != nil {
		t.Fatalf("run cycle: %v", err)
	}
}

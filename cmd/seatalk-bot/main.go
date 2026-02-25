package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spxph4227/go-bot-server/internal/bot"
	"github.com/spxph4227/go-bot-server/internal/config"
	"github.com/spxph4227/go-bot-server/internal/seatalk"
	"github.com/spxph4227/go-bot-server/internal/systemaccount"
	"github.com/spxph4227/go-bot-server/internal/workflow"
)

func main() {
	logger := log.New(os.Stdout, "[seatalk-bot] ", log.LstdFlags|log.Lmsgprefix)

	cfg, err := config.LoadFromEnv()
	if err != nil {
		logger.Fatalf("config error: %v", err)
	}

	mux := http.NewServeMux()
	switch cfg.Mode {
	case config.ModeBot:
		workflowDefs, loadErr := workflow.LoadDefinitions(cfg.WorkflowsFile)
		if loadErr != nil {
			logger.Fatalf("workflows error: %v", loadErr)
		}

		runner := workflow.NewRunner(workflowDefs, cfg.DefaultWorkflowTimeout)
		var messenger seatalk.Messenger
		if cfg.SeaTalkAppID != "" && cfg.SeaTalkAppSecret != "" {
			logger.Printf(
				"mode=bot auth=app_access_token app_id=%s workflows=%d",
				cfg.SeaTalkAppID,
				len(workflowDefs),
			)
			messenger = seatalk.NewClient(seatalk.ClientConfig{
				AppID:     cfg.SeaTalkAppID,
				AppSecret: cfg.SeaTalkAppSecret,
				BaseURL:   cfg.SeaTalkBaseURL,
				Timeout:   cfg.HTTPTimeout,
			})
		} else {
			logger.Printf("mode=bot auth=webhook-only outbound_reply=disabled workflows=%d", len(workflowDefs))
		}

		botServer := bot.New(bot.Config{
			SigningSecret: cfg.SeaTalkSigningSecret,
			CommandPrefix: cfg.CommandPrefix,
		}, messenger, runner, logger)

		mux.HandleFunc("/healthz", botServer.HandleHealthz)
		mux.HandleFunc("/callback", botServer.HandleCallback)
	case config.ModeSystemAccount:
		logger.Printf("mode=system_account outbound=group_webhook")
		systemClient := seatalk.NewSystemAccountClient(cfg.SeaTalkSystemWebhookURL, cfg.HTTPTimeout)
		systemServer := systemaccount.New(systemClient, logger)
		mux.HandleFunc("/healthz", systemServer.HandleHealthz)
		mux.HandleFunc("/send/text", systemServer.HandleSendText)
		mux.HandleFunc("/send/image", systemServer.HandleSendImage)
	default:
		logger.Fatalf("unsupported mode: %s", cfg.Mode)
	}

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		logger.Printf("server listening on %s", srv.Addr)
		if serveErr := srv.ListenAndServe(); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			logger.Fatalf("server stopped unexpectedly: %v", serveErr)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err = srv.Shutdown(ctx); err != nil {
		logger.Printf("shutdown error: %v", err)
	}
}

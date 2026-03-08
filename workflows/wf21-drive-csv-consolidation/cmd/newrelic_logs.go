package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

type newRelicLogEntry struct {
	tsMs int64
	line string
}

type newRelicLogRequestBody []newRelicLogBatch

type newRelicLogBatch struct {
	Common newRelicCommon `json:"common"`
	Logs   []newRelicLog  `json:"logs"`
}

type newRelicCommon struct {
	Attributes map[string]string `json:"attributes"`
}

type newRelicLog struct {
	Timestamp int64  `json:"timestamp"`
	Message   string `json:"message"`
}

type newRelicLogsWriter struct {
	apiURL     string
	licenseKey string
	common     newRelicCommon
	queue      chan newRelicLogEntry
	batchSize  int
	batchWait  time.Duration
	httpClient *http.Client

	closeOnce sync.Once
	stop      chan struct{}
	done      chan struct{}
}

func newWorkflowLogger(cfg workflowConfig) (*log.Logger, func(), error) {
	const (
		prefix = "[workflow-drive-csv-consolidation] "
		flags  = log.LstdFlags | log.Lmsgprefix
	)

	if !cfg.NewRelicLogsEnabled {
		return log.New(os.Stdout, prefix, flags), nil, nil
	}

	sink, err := newNewRelicLogsWriter(cfg)
	if err != nil {
		return nil, nil, err
	}
	logger := log.New(io.MultiWriter(os.Stdout, sink), prefix, flags)
	logger.Printf(
		"new relic logs enabled api_url=%s source=%s service=%s environment=%s batch_size=%d batch_wait=%s queue_size=%d timeout=%s",
		redactNewRelicLogAPIURL(cfg.NewRelicLogAPIURL),
		cfg.NewRelicSource,
		cfg.NewRelicService,
		cfg.NewRelicEnvironment,
		cfg.NewRelicLogsBatchSize,
		cfg.NewRelicLogsBatchWait,
		cfg.NewRelicLogsQueueSize,
		cfg.NewRelicLogsTimeout,
	)
	return logger, sink.Close, nil
}

func newNewRelicLogsWriter(cfg workflowConfig) (*newRelicLogsWriter, error) {
	if strings.TrimSpace(cfg.NewRelicLicenseKey) == "" {
		return nil, errors.New("WF21_NEWRELIC_LICENSE_KEY is required when WF21_NEWRELIC_LOGS_ENABLED=true")
	}
	apiURL, err := normalizeNewRelicLogAPIURL(cfg.NewRelicLogAPIURL)
	if err != nil {
		return nil, err
	}

	commonAttributes := map[string]string{
		"workflow":    workflowName,
		"source":      cfg.NewRelicSource,
		"service":     cfg.NewRelicService,
		"environment": cfg.NewRelicEnvironment,
	}

	writer := &newRelicLogsWriter{
		apiURL:     apiURL,
		licenseKey: cfg.NewRelicLicenseKey,
		common: newRelicCommon{
			Attributes: commonAttributes,
		},
		queue:     make(chan newRelicLogEntry, cfg.NewRelicLogsQueueSize),
		batchSize: cfg.NewRelicLogsBatchSize,
		batchWait: cfg.NewRelicLogsBatchWait,
		httpClient: &http.Client{
			Timeout: cfg.NewRelicLogsTimeout,
		},
		stop: make(chan struct{}),
		done: make(chan struct{}),
	}

	go writer.run()
	return writer, nil
}

func (w *newRelicLogsWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}

	select {
	case <-w.stop:
		return len(p), nil
	default:
	}

	chunk := strings.ReplaceAll(string(p), "\r\n", "\n")
	lines := strings.Split(chunk, "\n")
	for _, line := range lines {
		clean := strings.TrimRight(line, "\r")
		if strings.TrimSpace(clean) == "" {
			continue
		}
		entry := newRelicLogEntry{
			tsMs: time.Now().UTC().UnixMilli(),
			line: clean,
		}
		select {
		case <-w.stop:
			return len(p), nil
		case w.queue <- entry:
		default:
			// Drop logs when queue is full to keep workflow processing non-blocking.
		}
	}
	return len(p), nil
}

func (w *newRelicLogsWriter) Close() {
	w.closeOnce.Do(func() {
		close(w.stop)
		<-w.done
	})
}

func (w *newRelicLogsWriter) run() {
	defer close(w.done)

	ticker := time.NewTicker(w.batchWait)
	defer ticker.Stop()

	batch := make([]newRelicLogEntry, 0, w.batchSize)
	flush := func() {
		if len(batch) == 0 {
			return
		}
		if err := w.pushBatch(batch); err != nil {
			fmt.Fprintf(os.Stderr, "[workflow-drive-csv-consolidation] new relic log push failed err=%v\n", err)
		}
		batch = batch[:0]
	}

	for {
		select {
		case <-w.stop:
			for {
				select {
				case entry := <-w.queue:
					batch = append(batch, entry)
					if len(batch) >= w.batchSize {
						flush()
					}
				default:
					flush()
					return
				}
			}
		case entry := <-w.queue:
			batch = append(batch, entry)
			if len(batch) >= w.batchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		}
	}
}

func (w *newRelicLogsWriter) pushBatch(entries []newRelicLogEntry) error {
	if len(entries) == 0 {
		return nil
	}

	logs := make([]newRelicLog, 0, len(entries))
	for _, entry := range entries {
		logs = append(logs, newRelicLog{
			Timestamp: entry.tsMs,
			Message:   entry.line,
		})
	}

	payload := newRelicLogRequestBody{
		{
			Common: w.common,
			Logs:   logs,
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode new relic payload: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, w.apiURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build new relic request: %w", err)
	}
	req.Header.Set("Api-Key", w.licenseKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "wf21-newrelic-logs/1.0")

	resp, err := w.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send new relic request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		rawBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		msg := strings.TrimSpace(string(rawBody))
		return fmt.Errorf("new relic log push status=%d body=%q", resp.StatusCode, msg)
	}
	return nil
}

func normalizeNewRelicLogAPIURL(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return defaultNewRelicLogAPIURL, nil
	}

	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "", fmt.Errorf("invalid WF21_NEWRELIC_LOG_API_URL: %w", err)
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return "", errors.New("WF21_NEWRELIC_LOG_API_URL must start with https:// or http://")
	}
	if strings.TrimSpace(parsed.Host) == "" {
		return "", errors.New("WF21_NEWRELIC_LOG_API_URL must include a host")
	}
	if strings.TrimSpace(parsed.Path) == "" || parsed.Path == "/" {
		parsed.Path = "/log/v1"
	}
	return parsed.String(), nil
}

func redactNewRelicLogAPIURL(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return raw
	}
	parsed.User = nil
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String()
}

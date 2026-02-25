package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"
)

const (
	defaultSheetID      = "1mhzIfYfF1VSA9sPiqnLw7OgY1S_gI0wEzkXBQ1CCuDg"
	defaultSheetTab     = "MM LH Provided"
	defaultSheetRange   = "A2:M"
	defaultStateFile    = "data/workflow1-mm-lh-provided-state.json"
	defaultStatusFile   = "data/workflow1-mm-lh-provided-status.json"
	defaultHTTPTimeout  = 10 * time.Second
	defaultTextFormat   = 1
	systemAccountOKCode = 0
)

type workflowConfig struct {
	SheetID               string
	SheetTab              string
	SheetRange            string
	GoogleCredentialsFile string
	GoogleCredentialsJSON string
	SeaTalkWebhookURL     string
	StateFile             string
	StatusFile            string
	EnableHealthServer    bool
	HealthListenAddr      string
	SelfPingURL           string
	SelfPingInterval      time.Duration
	BootstrapSendExisting bool
	AtAll                 bool
	DryRun                bool
	DebugLogSkips         bool
	Continuous            bool
	PollInterval          time.Duration
	ForceSendAfter        time.Duration
	GroupDefer            time.Duration
	SendMinInterval       time.Duration
	SendRetryMaxAttempts  int
	SendRetryBaseDelay    time.Duration
	SendRetryMaxDelay     time.Duration
	HTTPTimeout           time.Duration
}

type sheetRow struct {
	RowNumber         int
	Status            string
	RequestTime       string
	Cluster           string
	TruckSize         string
	RequestedBy       string
	PlateNumber       string
	FleetSizeProvided string
	LHType            string
	ProvideTime       string
	DockLabel         string
}

type workflowState struct {
	RowPlates         map[string]string `json:"row_plates"`
	RowFirstSeenAt    map[string]string `json:"row_first_seen_at"`
	RowReadyAt        map[string]string `json:"row_ready_at"`
	RowSentForPlate   map[string]string `json:"row_sent_for_plate"`
	RowForcedForPlate map[string]string `json:"row_forced_for_plate"`
}

type systemAccountResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
}

type processSummary struct {
	Sent                 int
	Failed               int
	Ignored              int
	BaselineSkipped      int
	NotReadySkipped      int
	GroupHoldSkipped     int
	AlreadySentSkipped   int
	EmptyPlateResetCount int
	LastSentRow          int
	LastSentPlate        string
}

type sendCandidate struct {
	Key           string
	Row           sheetRow
	CurrPlate     string
	ReadyComplete bool
	ReadyForce    bool
	IsDoubleReq   bool
}

type workflowStatus struct {
	Workflow              string `json:"workflow"`
	Continuous            bool   `json:"continuous"`
	DryRun                bool   `json:"dry_run"`
	Cycle                 int    `json:"cycle"`
	StartedAt             string `json:"started_at"`
	LastCycleAt           string `json:"last_cycle_at"`
	RowsRead              int    `json:"rows_read"`
	Sent                  int    `json:"sent"`
	Failed                int    `json:"failed"`
	Ignored               int    `json:"ignored"`
	BaselineSkipped       int    `json:"baseline_skipped"`
	NotReadySkipped       int    `json:"not_ready_skipped"`
	AlreadySentSkipped    int    `json:"already_sent_skipped"`
	EmptyPlateResetCount  int    `json:"empty_plate_reset_count"`
	PendingForceSendCount int    `json:"pending_force_send_count"`
	LastSentRow           int    `json:"last_sent_row,omitempty"`
	LastSentPlate         string `json:"last_sent_plate,omitempty"`
	StateFile             string `json:"state_file"`
	StatusFile            string `json:"status_file,omitempty"`
}

func main() {
	logger := log.New(os.Stdout, "[workflow-mm-lh-provided] ", log.LstdFlags|log.Lmsgprefix)
	startedAt := time.Now().UTC()

	cfg, err := loadConfig()
	if err != nil {
		logger.Fatalf("config error: %v", err)
	}

	state, stateExists, err := loadState(cfg.StateFile)
	if err != nil {
		logger.Fatalf("load state error: %v", err)
	}

	httpClient := &http.Client{Timeout: cfg.HTTPTimeout}

	runCycle := func(ctx context.Context, cycle int) error {
		cycleAt := time.Now().UTC()
		rows, loadErr := loadSheetRows(ctx, cfg)
		if loadErr != nil {
			return loadErr
		}

		summary := processRows(ctx, cfg, httpClient, rows, state, stateExists, logger)
		if err = saveState(cfg.StateFile, state); err != nil {
			return err
		}
		stateExists = true

		status := workflowStatus{
			Workflow:              "workflow_1_mm_lh_provided",
			Continuous:            cfg.Continuous,
			DryRun:                cfg.DryRun,
			Cycle:                 cycle,
			StartedAt:             startedAt.Format(time.RFC3339),
			LastCycleAt:           cycleAt.Format(time.RFC3339),
			RowsRead:              len(rows),
			Sent:                  summary.Sent,
			Failed:                summary.Failed,
			Ignored:               summary.Ignored,
			BaselineSkipped:       summary.BaselineSkipped,
			NotReadySkipped:       summary.NotReadySkipped,
			AlreadySentSkipped:    summary.AlreadySentSkipped,
			EmptyPlateResetCount:  summary.EmptyPlateResetCount,
			PendingForceSendCount: countPendingForceDue(rows, state, cfg.ForceSendAfter, time.Now().UTC()),
			LastSentRow:           summary.LastSentRow,
			LastSentPlate:         summary.LastSentPlate,
			StateFile:             cfg.StateFile,
			StatusFile:            cfg.StatusFile,
		}
		if cfg.StatusFile != "" {
			if statusErr := saveStatus(cfg.StatusFile, status); statusErr != nil {
				logger.Printf("status write failed path=%s err=%v", cfg.StatusFile, statusErr)
			}
		}

		if cfg.Continuous {
			logger.Printf(
				"cycle=%d rows=%d sent=%d failed=%d ignored=%d baseline_skipped=%d not_ready_skipped=%d group_hold_skipped=%d already_sent_skipped=%d empty_plate_resets=%d pending_force_send=%d state_file=%s status_file=%s",
				cycle,
				len(rows),
				summary.Sent,
				summary.Failed,
				summary.Ignored,
				summary.BaselineSkipped,
				summary.NotReadySkipped,
				summary.GroupHoldSkipped,
				summary.AlreadySentSkipped,
				summary.EmptyPlateResetCount,
				status.PendingForceSendCount,
				cfg.StateFile,
				cfg.StatusFile,
			)
			return nil
		}

		logger.Printf(
			"completed rows=%d sent=%d failed=%d ignored=%d baseline_skipped=%d not_ready_skipped=%d group_hold_skipped=%d already_sent_skipped=%d empty_plate_resets=%d pending_force_send=%d state_file=%s status_file=%s",
			len(rows),
			summary.Sent,
			summary.Failed,
			summary.Ignored,
			summary.BaselineSkipped,
			summary.NotReadySkipped,
			summary.GroupHoldSkipped,
			summary.AlreadySentSkipped,
			summary.EmptyPlateResetCount,
			status.PendingForceSendCount,
			cfg.StateFile,
			cfg.StatusFile,
		)
		return nil
	}

	if !cfg.Continuous {
		if err = runCycle(context.Background(), 1); err != nil {
			logger.Fatalf("workflow error: %v", err)
		}
		return
	}

	logger.Printf("watch mode enabled poll_interval=%s", cfg.PollInterval)
	sigCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if cfg.EnableHealthServer {
		startHealthServer(sigCtx, cfg, logger)
	}
	if cfg.SelfPingURL != "" {
		startSelfPing(sigCtx, cfg, logger)
	}

	cycle := 1
	for {
		if err = runCycle(sigCtx, cycle); err != nil {
			logger.Printf("workflow cycle error: %v", err)
		}
		cycle++

		select {
		case <-sigCtx.Done():
			logger.Printf("watch mode stopped")
			return
		case <-time.After(cfg.PollInterval):
		}
	}
}

func loadConfig() (workflowConfig, error) {
	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		return workflowConfig{}, fmt.Errorf("load .env: %w", err)
	}

	bootstrapSendExisting, err := getBoolEnv("WF1_BOOTSTRAP_SEND_EXISTING", false)
	if err != nil {
		return workflowConfig{}, err
	}
	atAll, err := getBoolEnv("WF1_AT_ALL", false)
	if err != nil {
		return workflowConfig{}, err
	}
	dryRun, err := getBoolEnv("WF1_DRY_RUN", false)
	if err != nil {
		return workflowConfig{}, err
	}
	debugLogSkips, err := getBoolEnv("WF1_DEBUG_LOG_SKIPS", false)
	if err != nil {
		return workflowConfig{}, err
	}
	continuous, err := getBoolEnv("WF1_CONTINUOUS", false)
	if err != nil {
		return workflowConfig{}, err
	}
	enableHealthServer, err := getBoolEnv("WF1_ENABLE_HEALTH_SERVER", false)
	if err != nil {
		return workflowConfig{}, err
	}

	timeout := getDurationSeconds("WF1_HTTP_TIMEOUT_SECONDS", int(defaultHTTPTimeout/time.Second))
	pollInterval := getDurationSeconds("WF1_POLL_INTERVAL_SECONDS", 10)
	forceAfter := getDurationSeconds("WF1_FORCE_SEND_AFTER_SECONDS", 300)
	groupDefer := getDurationSeconds("WF1_GROUP_DEFER_SECONDS", 20)
	sendMinInterval := getDurationMillis("WF1_SEND_MIN_INTERVAL_MS", 1200)
	sendRetryMaxAttempts := getIntEnv("WF1_SEND_RETRY_MAX_ATTEMPTS", 5)
	sendRetryBaseDelay := getDurationMillis("WF1_SEND_RETRY_BASE_MS", 1000)
	sendRetryMaxDelay := getDurationMillis("WF1_SEND_RETRY_MAX_MS", 30000)
	selfPingInterval := getDurationSeconds("WF1_SELF_PING_INTERVAL_SECONDS", 300)
	credsFile := firstNonEmpty(
		strings.TrimSpace(os.Getenv("WF1_GOOGLE_CREDENTIALS_FILE")),
		strings.TrimSpace(os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")),
	)
	credsJSON := strings.TrimSpace(os.Getenv("WF1_GOOGLE_CREDENTIALS_JSON"))
	if credsFile == "" && credsJSON == "" {
		return workflowConfig{}, errors.New("set WF1_GOOGLE_CREDENTIALS_FILE/GOOGLE_APPLICATION_CREDENTIALS or WF1_GOOGLE_CREDENTIALS_JSON")
	}

	webhook := strings.TrimSpace(os.Getenv("SEATALK_SYSTEM_WEBHOOK_URL"))
	if webhook == "" {
		return workflowConfig{}, errors.New("SEATALK_SYSTEM_WEBHOOK_URL is required")
	}
	statusFile := strings.TrimSpace(os.Getenv("WF1_STATUS_FILE"))
	switch strings.ToLower(statusFile) {
	case "none", "off":
		statusFile = ""
	case "":
		statusFile = defaultStatusFile
	}
	healthPort := firstNonEmpty(
		strings.TrimSpace(os.Getenv("WF1_HEALTH_PORT")),
		strings.TrimSpace(os.Getenv("PORT")),
		"8080",
	)
	selfPingURL := strings.TrimSpace(os.Getenv("WF1_SELF_PING_URL"))

	return workflowConfig{
		SheetID:               firstNonEmpty(strings.TrimSpace(os.Getenv("WF1_SHEET_ID")), defaultSheetID),
		SheetTab:              firstNonEmpty(strings.TrimSpace(os.Getenv("WF1_SHEET_TAB")), defaultSheetTab),
		SheetRange:            firstNonEmpty(strings.TrimSpace(os.Getenv("WF1_SHEET_RANGE")), defaultSheetRange),
		GoogleCredentialsFile: credsFile,
		GoogleCredentialsJSON: credsJSON,
		SeaTalkWebhookURL:     webhook,
		StateFile:             firstNonEmpty(strings.TrimSpace(os.Getenv("WF1_STATE_FILE")), defaultStateFile),
		StatusFile:            statusFile,
		EnableHealthServer:    enableHealthServer,
		HealthListenAddr:      normalizeListenAddr(healthPort),
		SelfPingURL:           selfPingURL,
		SelfPingInterval:      selfPingInterval,
		BootstrapSendExisting: bootstrapSendExisting,
		AtAll:                 atAll,
		DryRun:                dryRun,
		DebugLogSkips:         debugLogSkips,
		Continuous:            continuous,
		PollInterval:          pollInterval,
		ForceSendAfter:        forceAfter,
		GroupDefer:            groupDefer,
		SendMinInterval:       sendMinInterval,
		SendRetryMaxAttempts:  sendRetryMaxAttempts,
		SendRetryBaseDelay:    sendRetryBaseDelay,
		SendRetryMaxDelay:     sendRetryMaxDelay,
		HTTPTimeout:           timeout,
	}, nil
}

func normalizeListenAddr(raw string) string {
	val := strings.TrimSpace(raw)
	if val == "" {
		return ":8080"
	}
	if strings.Contains(val, ":") {
		return val
	}
	return ":" + val
}

func startHealthServer(ctx context.Context, cfg workflowConfig, logger *log.Logger) {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/status", func(w http.ResponseWriter, _ *http.Request) {
		if strings.TrimSpace(cfg.StatusFile) == "" {
			http.Error(w, "status output disabled", http.StatusNotFound)
			return
		}
		raw, err := os.ReadFile(cfg.StatusFile)
		if err != nil {
			http.Error(w, fmt.Sprintf("status unavailable: %v", err), http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(raw)
	})

	server := &http.Server{
		Addr:              cfg.HealthListenAddr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		logger.Printf("health server listening on %s", cfg.HealthListenAddr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Printf("health server stopped unexpectedly: %v", err)
		}
	}()

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			logger.Printf("health server shutdown error: %v", err)
		}
	}()
}

func startSelfPing(ctx context.Context, cfg workflowConfig, logger *log.Logger) {
	client := &http.Client{Timeout: 10 * time.Second}
	go func() {
		ticker := time.NewTicker(cfg.SelfPingInterval)
		defer ticker.Stop()
		logger.Printf("self ping enabled interval=%s url=%s", cfg.SelfPingInterval, cfg.SelfPingURL)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				req, err := http.NewRequestWithContext(ctx, http.MethodGet, cfg.SelfPingURL, nil)
				if err != nil {
					logger.Printf("self ping request build failed: %v", err)
					continue
				}
				res, err := client.Do(req)
				if err != nil {
					logger.Printf("self ping failed: %v", err)
					continue
				}
				_ = res.Body.Close()
			}
		}
	}()
}

func processRows(
	ctx context.Context,
	cfg workflowConfig,
	httpClient *http.Client,
	rows []sheetRow,
	state workflowState,
	stateExists bool,
	logger *log.Logger,
) processSummary {
	summary := processSummary{}
	candidates := make([]sendCandidate, 0)

	for _, row := range rows {
		key := strconv.Itoa(row.RowNumber)
		currPlate := strings.TrimSpace(row.PlateNumber)
		prevPlateRaw, keySeen := state.RowPlates[key]
		prevPlate := strings.TrimSpace(prevPlateRaw)

		if currPlate == "" {
			state.RowPlates[key] = ""
			delete(state.RowFirstSeenAt, key)
			delete(state.RowReadyAt, key)
			delete(state.RowSentForPlate, key)
			delete(state.RowForcedForPlate, key)
			summary.EmptyPlateResetCount++
			continue
		}

		now := time.Now().UTC()
		if !keySeen {
			state.RowPlates[key] = currPlate
			state.RowFirstSeenAt[key] = now.Format(time.RFC3339)
			if !cfg.BootstrapSendExisting {
				summary.Ignored++
				summary.BaselineSkipped++
				if cfg.DebugLogSkips {
					logger.Printf("skip row=%d reason=bootstrap_baseline plate=%s", row.RowNumber, currPlate)
				}
				continue
			}
		}

		if prevPlate != currPlate {
			state.RowPlates[key] = currPlate
			state.RowFirstSeenAt[key] = now.Format(time.RFC3339)
			delete(state.RowReadyAt, key)
			delete(state.RowSentForPlate, key)
			delete(state.RowForcedForPlate, key)

			// First-ever run baseline: avoid sending historical already-filled rows unless explicitly enabled.
			if !stateExists && !cfg.BootstrapSendExisting {
				summary.Ignored++
				summary.BaselineSkipped++
				if cfg.DebugLogSkips {
					logger.Printf("skip row=%d reason=first_run_baseline plate=%s", row.RowNumber, currPlate)
				}
				continue
			}
		}

		if _, ok := state.RowFirstSeenAt[key]; !ok {
			state.RowFirstSeenAt[key] = now.Format(time.RFC3339)
		}

		if state.RowSentForPlate[key] == currPlate {
			summary.AlreadySentSkipped++
			continue
		}

		readyComplete := row.hasRequiredMessageFields()
		readyForce := row.hasForceSendFields() && isForceDue(state.RowFirstSeenAt[key], cfg.ForceSendAfter, now)
		isDoubleReq := isDoubleRequestText(currPlate)

		if isDoubleReq {
			readyComplete = row.hasDoubleRequestFields()
			readyForce = false
		}

		if !readyComplete && !readyForce {
			summary.Ignored++
			summary.NotReadySkipped++
			state.RowPlates[key] = currPlate
			delete(state.RowReadyAt, key)
			if cfg.DebugLogSkips {
				logger.Printf(
					"skip row=%d reason=not_ready plate=%s has_request_time=%t has_cluster=%t has_fleet_size=%t has_lh_type=%t has_provide_time=%t force_due=%t",
					row.RowNumber,
					currPlate,
					row.RequestTime != "",
					row.Cluster != "",
					row.FleetSizeProvided != "",
					row.LHType != "",
					row.ProvideTime != "",
					readyForce,
				)
			}
			continue
		}
		if readyComplete {
			if _, ok := state.RowReadyAt[key]; !ok {
				state.RowReadyAt[key] = now.Format(time.RFC3339)
			}
		} else {
			delete(state.RowReadyAt, key)
		}

		candidates = append(candidates, sendCandidate{
			Key:           key,
			Row:           row,
			CurrPlate:     currPlate,
			ReadyComplete: readyComplete,
			ReadyForce:    readyForce,
			IsDoubleReq:   isDoubleReq,
		})
	}

	dispatchCandidates(ctx, cfg, httpClient, candidates, &summary, state, logger)
	return summary
}

func dispatchCandidates(
	ctx context.Context,
	cfg workflowConfig,
	httpClient *http.Client,
	candidates []sendCandidate,
	summary *processSummary,
	state workflowState,
	logger *log.Logger,
) {
	if len(candidates) == 0 {
		return
	}

	lastSendAttemptAt := time.Time{}
	sendMessage := func(content string, atAll bool, meta string) error {
		if cfg.SendMinInterval > 0 && !lastSendAttemptAt.IsZero() {
			wait := cfg.SendMinInterval - time.Since(lastSendAttemptAt)
			if wait > 0 {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(wait):
				}
			}
		}
		lastSendAttemptAt = time.Now()
		return sendSeaTalkTextWithRetry(ctx, httpClient, cfg, content, atAll, meta, logger)
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Row.RowNumber < candidates[j].Row.RowNumber
	})

	groups := make(map[string][]sendCandidate)
	groupTimes := make(map[string]time.Time)
	singles := make([]sendCandidate, 0)

	for _, candidate := range candidates {
		if candidate.IsDoubleReq {
			singles = append(singles, candidate)
			continue
		}

		// Only complete rows with parseable Provide Time can be grouped.
		if !candidate.ReadyComplete {
			singles = append(singles, candidate)
			continue
		}

		provideTS, ok := parseProvideTime(candidate.Row.ProvideTime)
		if !ok {
			singles = append(singles, candidate)
			continue
		}

		minuteTS := provideTS.Truncate(time.Minute)
		groupKey := minuteTS.Format("2006-01-02 15:04")
		groups[groupKey] = append(groups[groupKey], candidate)
		groupTimes[groupKey] = minuteTS
	}

	groupKeys := make([]string, 0, len(groups))
	for key := range groups {
		groupKeys = append(groupKeys, key)
	}
	sort.Strings(groupKeys)

	for _, key := range groupKeys {
		group := groups[key]
		sort.Slice(group, func(i, j int) bool {
			return group[i].Row.RowNumber < group[j].Row.RowNumber
		})

		if len(group) <= 1 {
			candidate := group[0]
			if shouldHoldForGrouping(candidate, state, cfg.GroupDefer, time.Now().UTC()) {
				summary.Ignored++
				summary.GroupHoldSkipped++
				continue
			}
			singles = append(singles, candidate)
			continue
		}

		rows := make([]sheetRow, 0, len(group))
		for _, candidate := range group {
			rows = append(rows, candidate.Row)
		}
		content := buildMergedMessage(rows, formatProvideTimeMinute(groupTimes[key]))
		if cfg.DryRun {
			logger.Printf(
				"dry_run=true grouped=true rows=%d provide_time=%q content=%q",
				len(group),
				formatProvideTimeMinute(groupTimes[key]),
				content,
			)
			continue
		}

		if sendErr := sendMessage(content, cfg.AtAll, "grouped rows="+candidateRowList(group)); sendErr != nil {
			summary.Failed += len(group)
			logger.Printf("send failed grouped=true rows=%s err=%v", candidateRowList(group), sendErr)
			if isRateLimitError(sendErr) {
				logger.Printf("rate limit active; stopping further sends for this cycle")
				return
			}
			continue
		}

		for _, candidate := range group {
			summary.Sent++
			summary.LastSentRow = candidate.Row.RowNumber
			summary.LastSentPlate = candidate.CurrPlate
			state.RowSentForPlate[candidate.Key] = candidate.CurrPlate
			delete(state.RowReadyAt, candidate.Key)
			state.RowPlates[candidate.Key] = candidate.CurrPlate
		}
	}

	sort.Slice(singles, func(i, j int) bool {
		return singles[i].Row.RowNumber < singles[j].Row.RowNumber
	})

	for _, candidate := range singles {
		content := buildMessage(candidate.Row)
		atAll := cfg.AtAll
		if candidate.IsDoubleReq {
			content = buildDoubleRequestMessage(candidate.Row)
			atAll = true
		}
		if cfg.DryRun {
			logger.Printf("dry_run=true row=%d content=%q", candidate.Row.RowNumber, content)
			continue
		}

		if sendErr := sendMessage(content, atAll, "row="+strconv.Itoa(candidate.Row.RowNumber)); sendErr != nil {
			summary.Failed++
			logger.Printf("send failed row=%d: %v", candidate.Row.RowNumber, sendErr)
			if isRateLimitError(sendErr) {
				logger.Printf("rate limit active; stopping further sends for this cycle")
				return
			}
			continue
		}

		summary.Sent++
		summary.LastSentRow = candidate.Row.RowNumber
		summary.LastSentPlate = candidate.CurrPlate
		state.RowSentForPlate[candidate.Key] = candidate.CurrPlate
		delete(state.RowReadyAt, candidate.Key)
		if candidate.ReadyForce && !candidate.ReadyComplete {
			state.RowForcedForPlate[candidate.Key] = candidate.CurrPlate
			logger.Printf("force_send row=%d plate=%s age=%s", candidate.Row.RowNumber, candidate.CurrPlate, cfg.ForceSendAfter)
		}
		state.RowPlates[candidate.Key] = candidate.CurrPlate
	}
}

func shouldHoldForGrouping(candidate sendCandidate, state workflowState, deferDuration time.Duration, now time.Time) bool {
	if deferDuration <= 0 || !candidate.ReadyComplete {
		return false
	}
	readyAtRaw := strings.TrimSpace(state.RowReadyAt[candidate.Key])
	if readyAtRaw == "" {
		return false
	}
	readyAt, err := time.Parse(time.RFC3339, readyAtRaw)
	if err != nil {
		return false
	}
	return now.Sub(readyAt) < deferDuration
}

func candidateRowList(candidates []sendCandidate) string {
	parts := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		parts = append(parts, strconv.Itoa(candidate.Row.RowNumber))
	}
	return strings.Join(parts, ",")
}

func countPendingForceDue(rows []sheetRow, state workflowState, forceAfter time.Duration, now time.Time) int {
	count := 0
	for _, row := range rows {
		key := strconv.Itoa(row.RowNumber)
		currPlate := strings.TrimSpace(row.PlateNumber)
		if currPlate == "" {
			continue
		}
		if strings.TrimSpace(state.RowSentForPlate[key]) == currPlate {
			continue
		}
		if row.hasRequiredMessageFields() {
			continue
		}
		if !row.hasForceSendFields() {
			continue
		}
		if !isForceDue(state.RowFirstSeenAt[key], forceAfter, now) {
			continue
		}
		count++
	}
	return count
}

func isForceDue(firstSeenRaw string, forceAfter time.Duration, now time.Time) bool {
	if forceAfter <= 0 {
		return true
	}

	firstSeenAt, err := time.Parse(time.RFC3339, strings.TrimSpace(firstSeenRaw))
	if err != nil {
		return false
	}
	return now.Sub(firstSeenAt) >= forceAfter
}

func (r sheetRow) hasRequiredMessageFields() bool {
	return r.RequestTime != "" && r.Cluster != "" && r.FleetSizeProvided != "" && r.LHType != "" && r.ProvideTime != ""
}

func (r sheetRow) hasDoubleRequestFields() bool {
	return strings.TrimSpace(r.Cluster) != ""
}

func (r sheetRow) hasForceSendFields() bool {
	return r.RequestTime != "" && r.Cluster != "" && r.FleetSizeProvided != ""
}

func isDoubleRequestText(value string) bool {
	raw := strings.ToUpper(strings.TrimSpace(value))
	if raw == "" {
		return false
	}
	normalized := strings.Join(strings.Fields(raw), " ")
	if normalized == "DOUBLE REQUEST" {
		return true
	}
	re := regexp.MustCompile(`\bDOUBLE\b`)
	return re.MatchString(normalized)
}

func valueOrPending(value string) string {
	if strings.TrimSpace(value) == "" {
		return "PENDING"
	}
	return value
}

func buildMessage(row sheetRow) string {
	return fmt.Sprintf(
		"<mention-tag target=\"seatalk://user?id=0\"/> For Docking\n\n      **%s**\n      **Plate #: %s**\n      %s - %s\n      pvd_tme: %s",
		valueOrPending(clusterWithDock(row.Cluster, row.DockLabel)),
		valueOrPending(row.PlateNumber),
		valueOrPending(row.FleetSizeProvided),
		valueOrPending(row.LHType),
		formatProvideTime(valueOrPending(row.ProvideTime)),
	)
}

func buildDoubleRequestMessage(row sheetRow) string {
	return fmt.Sprintf(
		"Double Request!\n%s",
		valueOrPending(clusterWithDock(row.Cluster, row.DockLabel)),
	)
}

func buildMergedMessage(rows []sheetRow, providedTime string) string {
	var builder strings.Builder
	builder.WriteString("<mention-tag target=\"seatalk://user?id=0\"/> For Docking\n\n")

	for idx, row := range rows {
		if idx > 0 {
			builder.WriteString("\n\n")
		}
		builder.WriteString(fmt.Sprintf(
			"      **%s**\n      **Plate_#: %s**\n      %s-%s",
			valueOrPending(clusterWithDock(row.Cluster, row.DockLabel)),
			valueOrPending(row.PlateNumber),
			valueOrPending(row.FleetSizeProvided),
			valueOrPending(row.LHType),
		))
	}

	builder.WriteString("\n\nProvided Time: ")
	builder.WriteString(valueOrPending(providedTime))
	return builder.String()
}

func formatProvideTime(raw string) string {
	parsed, ok := parseProvideTime(raw)
	if !ok {
		val := strings.TrimSpace(raw)
		if val == "" {
			return "PENDING"
		}
		return val
	}
	return parsed.Format("15:04:05 Jan-02")
}

func formatProvideTimeMinute(ts time.Time) string {
	return ts.Format("1/2/2006 3:04 PM")
}

func parseProvideTime(raw string) (time.Time, bool) {
	val := strings.TrimSpace(raw)
	if val == "" {
		return time.Time{}, false
	}

	layouts := []string{
		"1/2/2006 15:04:05",
		"1/2/2006 15:04",
		"1/2/2006 3:04:05 PM",
		"1/2/2006 3:04 PM",
		"2006-01-02 15:04:05",
		"2006-01-02 15:04",
		time.RFC3339,
		time.RFC3339Nano,
	}
	for _, layout := range layouts {
		if ts, err := time.Parse(layout, val); err == nil {
			return ts, true
		}
	}

	return time.Time{}, false
}

func clusterWithDock(cluster string, dock string) string {
	clusterVal := strings.TrimSpace(cluster)
	dockVal := strings.TrimSpace(dock)
	if clusterVal == "" {
		return ""
	}
	if dockVal == "" {
		return clusterVal
	}
	return clusterVal + " - " + dockVal
}

func loadState(path string) (workflowState, bool, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return workflowState{
			RowPlates:         map[string]string{},
			RowFirstSeenAt:    map[string]string{},
			RowReadyAt:        map[string]string{},
			RowSentForPlate:   map[string]string{},
			RowForcedForPlate: map[string]string{},
		}, false, nil
	}
	if err != nil {
		return workflowState{}, false, err
	}

	var parsed workflowState
	if err = json.Unmarshal(raw, &parsed); err != nil {
		return workflowState{}, false, fmt.Errorf("decode state file %s: %w", path, err)
	}
	if parsed.RowPlates == nil {
		parsed.RowPlates = map[string]string{}
	}
	if parsed.RowFirstSeenAt == nil {
		parsed.RowFirstSeenAt = map[string]string{}
	}
	if parsed.RowReadyAt == nil {
		parsed.RowReadyAt = map[string]string{}
	}
	if parsed.RowSentForPlate == nil {
		parsed.RowSentForPlate = map[string]string{}
	}
	if parsed.RowForcedForPlate == nil {
		parsed.RowForcedForPlate = map[string]string{}
	}
	return parsed, true, nil
}

func saveState(path string, state workflowState) error {
	if state.RowPlates == nil {
		state.RowPlates = map[string]string{}
	}
	if state.RowFirstSeenAt == nil {
		state.RowFirstSeenAt = map[string]string{}
	}
	if state.RowReadyAt == nil {
		state.RowReadyAt = map[string]string{}
	}
	if state.RowSentForPlate == nil {
		state.RowSentForPlate = map[string]string{}
	}
	if state.RowForcedForPlate == nil {
		state.RowForcedForPlate = map[string]string{}
	}

	dir := filepath.Dir(path)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}

	raw, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o644)
}

func saveStatus(path string, status workflowStatus) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}

	dir := filepath.Dir(path)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}

	raw, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o644)
}

type seatalkHTTPError struct {
	StatusCode int
	Body       string
}

func (e *seatalkHTTPError) Error() string {
	return fmt.Sprintf("status=%d body=%s", e.StatusCode, e.Body)
}

type seatalkAPIError struct {
	Code int
	Msg  string
}

func (e *seatalkAPIError) Error() string {
	return fmt.Sprintf("error_code=%d msg=%s", e.Code, e.Msg)
}

func isRateLimitError(err error) bool {
	var httpErr *seatalkHTTPError
	if errors.As(err, &httpErr) {
		if httpErr.StatusCode == http.StatusTooManyRequests {
			return true
		}
		if strings.Contains(strings.ToLower(httpErr.Body), "rate limit") {
			return true
		}
	}
	var apiErr *seatalkAPIError
	if errors.As(err, &apiErr) {
		return apiErr.Code == 8
	}
	return strings.Contains(strings.ToLower(err.Error()), "rate limit")
}

func sendSeaTalkTextWithRetry(
	ctx context.Context,
	client *http.Client,
	cfg workflowConfig,
	content string,
	atAll bool,
	meta string,
	logger *log.Logger,
) error {
	maxAttempts := cfg.SendRetryMaxAttempts
	if maxAttempts < 1 {
		maxAttempts = 1
	}
	delay := cfg.SendRetryBaseDelay
	if delay <= 0 {
		delay = 1 * time.Second
	}
	maxDelay := cfg.SendRetryMaxDelay
	if maxDelay <= 0 {
		maxDelay = 30 * time.Second
	}

	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		lastErr = sendSeaTalkText(ctx, client, cfg.SeaTalkWebhookURL, content, atAll)
		if lastErr == nil {
			return nil
		}

		if attempt == maxAttempts || !isRateLimitError(lastErr) {
			return lastErr
		}

		logger.Printf(
			"rate_limited meta=%s attempt=%d/%d backoff=%s err=%v",
			meta,
			attempt,
			maxAttempts,
			delay,
			lastErr,
		)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
		delay *= 2
		if delay > maxDelay {
			delay = maxDelay
		}
	}
	return lastErr
}

func sendSeaTalkText(ctx context.Context, client *http.Client, webhookURL, content string, atAll bool) error {
	textBody := map[string]any{
		"format":  defaultTextFormat,
		"content": content,
	}

	payload := map[string]any{
		"tag":  "text",
		"text": textBody,
	}
	if atAll {
		payload["at_all"] = true
	}

	rawBody, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(rawBody))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	rawResp, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}
	if res.StatusCode != http.StatusOK {
		return &seatalkHTTPError{
			StatusCode: res.StatusCode,
			Body:       string(rawResp),
		}
	}

	var parsed systemAccountResponse
	if err = json.Unmarshal(rawResp, &parsed); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	if parsed.Code != systemAccountOKCode {
		return &seatalkAPIError{
			Code: parsed.Code,
			Msg:  parsed.Msg,
		}
	}
	return nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func getBoolEnv(key string, fallback bool) (bool, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback, nil
	}

	parsed, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("%s must be a boolean value", key)
	}
	return parsed, nil
}

func getDurationSeconds(key string, fallback int) time.Duration {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return time.Duration(fallback) * time.Second
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil || parsed <= 0 {
		return time.Duration(fallback) * time.Second
	}
	return time.Duration(parsed) * time.Second
}

func getDurationMillis(key string, fallback int) time.Duration {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return time.Duration(fallback) * time.Millisecond
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil || parsed <= 0 {
		return time.Duration(fallback) * time.Millisecond
	}
	return time.Duration(parsed) * time.Millisecond
}

func getIntEnv(key string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func loadSheetRows(ctx context.Context, cfg workflowConfig) ([]sheetRow, error) {
	options := []option.ClientOption{
		option.WithScopes(sheets.SpreadsheetsReadonlyScope),
	}
	if strings.TrimSpace(cfg.GoogleCredentialsJSON) != "" {
		options = append(options, option.WithCredentialsJSON([]byte(cfg.GoogleCredentialsJSON)))
	} else {
		options = append(options, option.WithCredentialsFile(cfg.GoogleCredentialsFile))
	}

	service, err := sheets.NewService(ctx, options...)
	if err != nil {
		return nil, fmt.Errorf("create sheets service: %w", err)
	}

	rangeRef := fmt.Sprintf("%s!%s", cfg.SheetTab, cfg.SheetRange)
	resp, err := service.Spreadsheets.Values.Get(cfg.SheetID, rangeRef).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("read sheet range %s: %w", rangeRef, err)
	}

	rows := make([]sheetRow, 0, len(resp.Values))
	startRow := rangeStartRow(cfg.SheetRange, 2)
	for idx, raw := range resp.Values {
		row := parseRow(raw, idx+startRow)
		if !row.hasAnyData() {
			continue
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func rangeStartRow(rangeRef string, fallback int) int {
	base := strings.TrimSpace(rangeRef)
	if base == "" {
		return fallback
	}
	if parts := strings.SplitN(base, ":", 2); len(parts) > 0 {
		base = strings.TrimSpace(parts[0])
	}
	re := regexp.MustCompile(`(?i)[A-Z]+(\d+)`)
	matches := re.FindStringSubmatch(base)
	if len(matches) < 2 {
		return fallback
	}
	row, err := strconv.Atoi(matches[1])
	if err != nil || row <= 0 {
		return fallback
	}
	return row
}

func parseRow(values []interface{}, rowNumber int) sheetRow {
	return sheetRow{
		RowNumber:         rowNumber,
		Status:            cell(values, 0),
		RequestTime:       cell(values, 1),
		Cluster:           cell(values, 2),
		TruckSize:         cell(values, 3),
		RequestedBy:       cell(values, 4),
		PlateNumber:       cell(values, 5),
		FleetSizeProvided: cell(values, 6),
		LHType:            cell(values, 7),
		ProvideTime:       cell(values, 8),
		DockLabel:         cell(values, 12),
	}
}

func cell(values []interface{}, idx int) string {
	if idx < 0 || idx >= len(values) {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(values[idx]))
}

func (r sheetRow) hasAnyData() bool {
	return strings.TrimSpace(
		r.Status+r.RequestTime+r.Cluster+r.TruckSize+r.RequestedBy+r.PlateNumber+r.FleetSizeProvided+r.LHType+r.ProvideTime+r.DockLabel,
	) != ""
}

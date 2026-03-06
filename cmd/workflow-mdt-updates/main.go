package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/spxph4227/go-bot-server/internal/seatalk"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"
)

const (
	workflowName = "workflow_3_mdt_updates"

	defaultSheetID      = "1pLN46ZKWJIsidswMeoxhZwoacuFMR08sCaTFG6mLytc"
	defaultSheetTab     = "mdt"
	defaultMonitorRange = "G1:O227"
	defaultImageRanges  = "B1:P42,B44:P108,B109:P166,B167:P196,B198:P231"

	defaultImage1FixedHideRows = "16-26,28,30,32-37,39-40"

	defaultPollInterval = 3 * time.Second
	defaultDebounce     = 180 * time.Second
	defaultHTTPTimeout  = 90 * time.Second
	defaultSendMinGap   = 1 * time.Second
	defaultTimezone     = "Asia/Manila"

	defaultPDFDPI       = 180
	defaultPDFConverter = "auto"
	defaultMaxImageB64  = 5 * 1024 * 1024

	defaultStateFile  = "data/workflow3-mdt-updates-state.json"
	defaultStatusFile = "data/workflow3-mdt-updates-status.json"

	defaultStabilityRuns = 3
	defaultStabilityWait = 2 * time.Second

	pdfExportRetryMax    = 5
	pdfExportRetryBase   = 1 * time.Second
	pdfExportRetryMaxDur = 8 * time.Second

	sendRetryMax    = 5
	sendRetryBase   = 1 * time.Second
	sendRetryMaxDur = 8 * time.Second
)

type workflowConfig struct {
	SheetID      string
	SheetTab     string
	MonitorRange string
	ImageRanges  []imageRangeSpec

	Image1FixedHideRows map[int]struct{}

	GoogleCredentialsFile string
	GoogleCredentialsJSON string

	SeaTalkAppID     string
	SeaTalkAppSecret string
	SeaTalkBaseURL   string
	SeaTalkGroupID   string

	Continuous            bool
	BootstrapSendExisting bool
	DryRun                bool
	TestSendOnce          bool
	PollInterval          time.Duration
	SendDebounce          time.Duration
	SendMinGap            time.Duration
	HTTPTimeout           time.Duration
	TimeZone              string
	Location              *time.Location
	StabilityRuns         int
	StabilityWait         time.Duration

	PDFDPI       int
	PDFConverter string
	TempDir      string

	MaxImageB64Size int

	StateFile  string
	StatusFile string

	EnableHealthServer bool
	HealthListenAddr   string
}

type imageRangeSpec struct {
	Label    string
	StartCol int
	EndCol   int
	StartRow int
	EndRow   int
}

type workflowState struct {
	LastSeenDigest string `json:"last_seen_digest"`
	LastSeenAt     string `json:"last_seen_at,omitempty"`
	PendingDigest  string `json:"pending_digest,omitempty"`
	PendingSince   string `json:"pending_since,omitempty"`
	LastSentDigest string `json:"last_sent_digest,omitempty"`
	LastSentAt     string `json:"last_sent_at,omitempty"`
}

type workflowStatus struct {
	Workflow        string `json:"workflow"`
	Continuous      bool   `json:"continuous"`
	DryRun          bool   `json:"dry_run"`
	Cycle           int    `json:"cycle"`
	LastCycleAt     string `json:"last_cycle_at"`
	Changed         bool   `json:"changed"`
	ShouldSend      bool   `json:"should_send"`
	MonitorRange    string `json:"monitor_range"`
	CurrentDigest   string `json:"current_digest"`
	LastSeenDigest  string `json:"last_seen_digest"`
	PendingDigest   string `json:"pending_digest,omitempty"`
	PendingSince    string `json:"pending_since,omitempty"`
	LastSentDigest  string `json:"last_sent_digest,omitempty"`
	LastSentAt      string `json:"last_sent_at,omitempty"`
	ImagesPrepared  int    `json:"images_prepared"`
	ImagesSent      int    `json:"images_sent"`
	LastImageFormat string `json:"last_image_format,omitempty"`
	LastImageBytes  int    `json:"last_image_bytes,omitempty"`
	StateFile       string `json:"state_file"`
	StatusFile      string `json:"status_file,omitempty"`
}

type encodedImage struct {
	Label      string
	Base64Data string
	Format     string
	RawBytes   int
}

type sheetRange struct {
	startCol int
	endCol   int
	startRow int
	endRow   int
}

func main() {
	logger := log.New(os.Stdout, "[workflow-mdt-updates] ", log.LstdFlags|log.Lmsgprefix)

	cfg, err := loadConfig()
	if err != nil {
		logger.Fatalf("config error: %v", err)
	}

	state, stateExists, err := loadState(cfg.StateFile)
	if err != nil {
		logger.Fatalf("state load error: %v", err)
	}

	sheetsSvc, err := newSheetsService(context.Background(), cfg.GoogleCredentialsFile, cfg.GoogleCredentialsJSON)
	if err != nil {
		logger.Fatalf("google sheets init error: %v", err)
	}
	authHTTPClient, err := newGoogleAuthenticatedHTTPClient(context.Background(), cfg.GoogleCredentialsFile, cfg.GoogleCredentialsJSON)
	if err != nil {
		logger.Fatalf("google auth http client error: %v", err)
	}

	seaTalkClient := seatalk.NewClient(seatalk.ClientConfig{
		AppID:     cfg.SeaTalkAppID,
		AppSecret: cfg.SeaTalkAppSecret,
		BaseURL:   cfg.SeaTalkBaseURL,
		Timeout:   cfg.HTTPTimeout,
	})

	if !cfg.Continuous {
		if err = runCycle(context.Background(), cfg, sheetsSvc, authHTTPClient, seaTalkClient, &state, &stateExists, logger, 1); err != nil {
			logger.Fatalf("workflow failed: %v", err)
		}
		return
	}

	sigCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if cfg.EnableHealthServer {
		startHealthServer(sigCtx, cfg, logger)
	}

	logger.Printf("watch mode enabled poll_interval=%s monitor=%s!%s debounce=%s", cfg.PollInterval, cfg.SheetTab, cfg.MonitorRange, cfg.SendDebounce)
	cycle := 1
	for {
		if err = runCycle(sigCtx, cfg, sheetsSvc, authHTTPClient, seaTalkClient, &state, &stateExists, logger, cycle); err != nil {
			if sigCtx.Err() != nil {
				logger.Printf("watch mode stopped")
				return
			}
			logger.Printf("cycle=%d error=%v", cycle, err)
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

func runCycle(
	ctx context.Context,
	cfg workflowConfig,
	sheetsSvc *sheets.Service,
	authHTTPClient *http.Client,
	seaTalkClient *seatalk.Client,
	state *workflowState,
	stateExists *bool,
	logger *log.Logger,
	cycle int,
) error {
	now := time.Now().UTC()
	digest, err := captureValuesDigest(ctx, sheetsSvc, cfg.SheetID, cfg.SheetTab, cfg.MonitorRange)
	if err != nil {
		return err
	}

	if !*stateExists {
		state.LastSeenDigest = digest
		state.LastSeenAt = now.Format(time.RFC3339)
		if cfg.BootstrapSendExisting || cfg.TestSendOnce {
			state.PendingDigest = digest
			state.PendingSince = now.Format(time.RFC3339)
		}
		if err = saveState(cfg.StateFile, *state); err != nil {
			return err
		}
		*stateExists = true
		if !cfg.BootstrapSendExisting && !cfg.TestSendOnce {
			logger.Printf("baseline set monitor=%s value=%s", cfg.MonitorRange, shortDigest(digest))
		}
	}

	changed := digest != strings.TrimSpace(state.LastSeenDigest)
	if changed {
		state.LastSeenDigest = digest
		state.LastSeenAt = now.Format(time.RFC3339)
		state.PendingDigest = digest
		state.PendingSince = now.Format(time.RFC3339)
		if err = saveState(cfg.StateFile, *state); err != nil {
			return err
		}
		logger.Printf("change detected monitor=%s digest=%s", cfg.MonitorRange, shortDigest(digest))
	}

	shouldSend := false
	pendingDigest := strings.TrimSpace(state.PendingDigest)
	if pendingDigest != "" && pendingDigest == digest && digest != strings.TrimSpace(state.LastSentDigest) {
		pendingSince := parseRFC3339OrNow(state.PendingSince, now)
		if cfg.TestSendOnce || now.Sub(pendingSince) >= cfg.SendDebounce {
			shouldSend = true
		}
	}

	imagesPrepared := 0
	imagesSent := 0
	lastImageFmt := ""
	lastImageBytes := 0

	if shouldSend {
		stable, stableErr := waitForStableRangeDigest(ctx, sheetsSvc, cfg.SheetID, cfg.SheetTab, cfg.MonitorRange, cfg.StabilityRuns, cfg.StabilityWait)
		if stableErr != nil {
			return stableErr
		}
		if !stable && !cfg.TestSendOnce {
			logger.Printf("pending send skipped monitor not stable monitor=%s digest=%s", cfg.MonitorRange, shortDigest(digest))
			return writeStatus(cfg, workflowStatus{
				Workflow:       workflowName,
				Continuous:     cfg.Continuous,
				DryRun:         cfg.DryRun,
				Cycle:          cycle,
				LastCycleAt:    now.Format(time.RFC3339),
				Changed:        changed,
				ShouldSend:     false,
				MonitorRange:   cfg.MonitorRange,
				CurrentDigest:  digest,
				LastSeenDigest: state.LastSeenDigest,
				PendingDigest:  state.PendingDigest,
				PendingSince:   state.PendingSince,
				LastSentDigest: state.LastSentDigest,
				LastSentAt:     state.LastSentAt,
				StateFile:      cfg.StateFile,
				StatusFile:     cfg.StatusFile,
			}, logger)
		}

		images, prepErr := prepareImages(ctx, cfg, sheetsSvc, authHTTPClient)
		if prepErr != nil {
			return prepErr
		}
		imagesPrepared = len(images)
		if len(images) == 0 {
			return errors.New("no visible image content available to send")
		}
		lastImageFmt = images[len(images)-1].Format
		lastImageBytes = images[len(images)-1].RawBytes

		caption := buildCaption(cfg.Location, time.Now())
		if cfg.DryRun {
			logger.Printf("dry_run=true send digest=%s caption=%q images=%d", shortDigest(digest), caption, len(images))
		} else {
			if seaTalkClient == nil {
				return errors.New("seatalk bot client is not configured")
			}
			if err = sendWithRetry(ctx, "send wf3 caption", func() error {
				return seaTalkClient.SendTextToGroup(ctx, cfg.SeaTalkGroupID, caption, 1)
			}); err != nil {
				return err
			}
			for _, img := range images {
				if waitErr := waitWithContext(ctx, cfg.SendMinGap); waitErr != nil {
					return waitErr
				}
				label := img.Label
				base64Data := img.Base64Data
				if err = sendWithRetry(ctx, "send "+label, func() error {
					return seaTalkClient.SendImageToGroupBase64(ctx, cfg.SeaTalkGroupID, base64Data)
				}); err != nil {
					return err
				}
				imagesSent++
			}
			logger.Printf("sent digest=%s images=%d", shortDigest(digest), imagesSent)
		}

		state.LastSentDigest = digest
		state.LastSentAt = now.Format(time.RFC3339)
		state.PendingDigest = ""
		state.PendingSince = ""
		if err = saveState(cfg.StateFile, *state); err != nil {
			return err
		}

	}

	if !changed && !shouldSend {
		logger.Printf("no action monitor=%s digest=%s", cfg.MonitorRange, shortDigest(digest))
	}

	return writeStatus(cfg, workflowStatus{
		Workflow:        workflowName,
		Continuous:      cfg.Continuous,
		DryRun:          cfg.DryRun,
		Cycle:           cycle,
		LastCycleAt:     now.Format(time.RFC3339),
		Changed:         changed,
		ShouldSend:      shouldSend,
		MonitorRange:    cfg.MonitorRange,
		CurrentDigest:   digest,
		LastSeenDigest:  state.LastSeenDigest,
		PendingDigest:   state.PendingDigest,
		PendingSince:    state.PendingSince,
		LastSentDigest:  state.LastSentDigest,
		LastSentAt:      state.LastSentAt,
		ImagesPrepared:  imagesPrepared,
		ImagesSent:      imagesSent,
		LastImageFormat: lastImageFmt,
		LastImageBytes:  lastImageBytes,
		StateFile:       cfg.StateFile,
		StatusFile:      cfg.StatusFile,
	}, logger)
}

func writeStatus(cfg workflowConfig, status workflowStatus, logger *log.Logger) error {
	if strings.TrimSpace(cfg.StatusFile) == "" {
		return nil
	}
	if err := saveStatus(cfg.StatusFile, status); err != nil {
		logger.Printf("status write failed path=%s err=%v", cfg.StatusFile, err)
	}
	return nil
}

func prepareImages(
	ctx context.Context,
	cfg workflowConfig,
	sheetsSvc *sheets.Service,
	exportHTTPClient *http.Client,
) ([]encodedImage, error) {
	out := make([]encodedImage, 0, len(cfg.ImageRanges))
	for idx, spec := range cfg.ImageRanges {
		hiddenRows := map[int]struct{}{}
		if idx == 0 {
			for row := range cfg.Image1FixedHideRows {
				hiddenRows[row] = struct{}{}
			}
		}

		pngRaw, skipped, renderErr := renderImageWithTemporaryHiddenRows(ctx, cfg, sheetsSvc, exportHTTPClient, spec, hiddenRows)
		if renderErr != nil {
			return nil, fmt.Errorf("render %s: %w", spec.Label, renderErr)
		}
		if skipped || len(pngRaw) == 0 {
			continue
		}

		base64Data, fmtName, rawSize, encErr := encodeImageWithinLimit(pngRaw, cfg.MaxImageB64Size)
		if encErr != nil {
			return nil, fmt.Errorf("encode %s: %w", spec.Label, encErr)
		}
		out = append(out, encodedImage{
			Label:      spec.Label,
			Base64Data: base64Data,
			Format:     fmtName,
			RawBytes:   rawSize,
		})
	}
	return out, nil
}

func renderImageWithTemporaryHiddenRows(
	ctx context.Context,
	cfg workflowConfig,
	sheetsSvc *sheets.Service,
	exportHTTPClient *http.Client,
	spec imageRangeSpec,
	additionalHiddenRows map[int]struct{},
) ([]byte, bool, error) {
	sheetGID, currentHidden, err := readRowHiddenStates(ctx, sheetsSvc, cfg.SheetID, cfg.SheetTab, spec.StartRow, spec.EndRow)
	if err != nil {
		return nil, false, err
	}

	targetHidden := make(map[int]bool, len(currentHidden))
	visibleRows := 0
	for row := spec.StartRow; row <= spec.EndRow; row++ {
		rowHidden := currentHidden[row]
		if _, shouldHide := additionalHiddenRows[row]; shouldHide {
			rowHidden = true
		}
		targetHidden[row] = rowHidden
		if !rowHidden {
			visibleRows++
		}
	}
	if visibleRows == 0 {
		return nil, true, nil
	}

	restoreNeeded := !rowHiddenStateEqual(currentHidden, targetHidden, spec.StartRow, spec.EndRow)
	if restoreNeeded {
		if err = setRowHiddenState(ctx, sheetsSvc, cfg.SheetID, sheetGID, spec.StartRow, spec.EndRow, targetHidden); err != nil {
			return nil, false, fmt.Errorf("apply temporary hidden rows: %w", err)
		}
		defer func() {
			restoreCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			_ = setRowHiddenState(restoreCtx, sheetsSvc, cfg.SheetID, sheetGID, spec.StartRow, spec.EndRow, currentHidden)
		}()
	}

	rangeRef := fmt.Sprintf("%s%d:%s%d",
		columnIndexToName(spec.StartCol),
		spec.StartRow,
		columnIndexToName(spec.EndCol),
		spec.EndRow,
	)
	pngRaw, err := renderRangeViaPDFPNG(ctx, cfg, exportHTTPClient, sheetGID, rangeRef)
	if err != nil {
		return nil, false, err
	}
	return pngRaw, false, nil
}

func readRowHiddenStates(
	ctx context.Context,
	sheetsSvc *sheets.Service,
	sheetID, tab string,
	startRow, endRow int,
) (int64, map[int]bool, error) {
	rangeRef := buildSheetRangeRef(tab, fmt.Sprintf("A%d:A%d", startRow, endRow))
	resp, err := sheetsSvc.Spreadsheets.Get(sheetID).
		Ranges(rangeRef).
		IncludeGridData(true).
		Fields("sheets(properties(sheetId,title),data(startRow,rowMetadata(hiddenByUser)))").
		Context(ctx).
		Do()
	if err != nil {
		return 0, nil, fmt.Errorf("read row metadata: %w", err)
	}

	hidden := make(map[int]bool, endRow-startRow+1)
	for row := startRow; row <= endRow; row++ {
		hidden[row] = false
	}

	targetTab := normalizeSheetTabName(tab)
	for _, sh := range resp.Sheets {
		if sh == nil || sh.Properties == nil {
			continue
		}
		if normalizeSheetTabName(sh.Properties.Title) != targetTab {
			continue
		}

		for _, grid := range sh.Data {
			if grid == nil {
				continue
			}
			gridStart := int(grid.StartRow) + 1
			for i, meta := range grid.RowMetadata {
				row := gridStart + i
				if row < startRow || row > endRow {
					continue
				}
				hidden[row] = meta != nil && meta.HiddenByUser
			}
		}
		return sh.Properties.SheetId, hidden, nil
	}
	return 0, nil, fmt.Errorf("sheet tab %q not found", tab)
}

func setRowHiddenState(
	ctx context.Context,
	sheetsSvc *sheets.Service,
	sheetID string,
	sheetGID int64,
	startRow, endRow int,
	target map[int]bool,
) error {
	requests := make([]*sheets.Request, 0, endRow-startRow+1)
	runStart := startRow
	runState := target[startRow]
	for row := startRow + 1; row <= endRow; row++ {
		state := target[row]
		if state == runState {
			continue
		}
		requests = append(requests, buildHiddenRowRequest(sheetGID, runStart, row-1, runState))
		runStart = row
		runState = state
	}
	requests = append(requests, buildHiddenRowRequest(sheetGID, runStart, endRow, runState))
	if len(requests) == 0 {
		return nil
	}

	body := &sheets.BatchUpdateSpreadsheetRequest{
		Requests: requests,
	}
	_, err := sheetsSvc.Spreadsheets.BatchUpdate(sheetID, body).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("batch update row hidden state: %w", err)
	}
	return nil
}

func buildHiddenRowRequest(sheetGID int64, startRow, endRow int, hidden bool) *sheets.Request {
	return &sheets.Request{
		UpdateDimensionProperties: &sheets.UpdateDimensionPropertiesRequest{
			Range: &sheets.DimensionRange{
				SheetId:    sheetGID,
				Dimension:  "ROWS",
				StartIndex: int64(startRow - 1),
				EndIndex:   int64(endRow),
			},
			Properties: &sheets.DimensionProperties{
				HiddenByUser: hidden,
			},
			Fields: "hiddenByUser",
		},
	}
}

func rowHiddenStateEqual(a, b map[int]bool, startRow, endRow int) bool {
	for row := startRow; row <= endRow; row++ {
		if a[row] != b[row] {
			return false
		}
	}
	return true
}

func buildCaption(loc *time.Location, now time.Time) string {
	if loc == nil {
		loc = time.Local
	}
	yesterday := now.In(loc).AddDate(0, 0, -1)
	return fmt.Sprintf(
		"<mention-tag target=\"seatalk://user?id=0\"/>\nOutbound Packed to Depart Compliance for %s\n\n`- This Report is auto generated once mdt dashboard updates.`",
		yesterday.Format("Monday, Jan 02, 2006"),
	)
}

func sendWithRetry(ctx context.Context, op string, send func() error) error {
	delay := sendRetryBase
	var lastErr error
	for attempt := 1; attempt <= sendRetryMax; attempt++ {
		lastErr = send()
		if lastErr == nil {
			return nil
		}
		if attempt == sendRetryMax || !isRetryableSendError(lastErr) {
			return fmt.Errorf("%s: %w", op, lastErr)
		}
		if err := waitWithContext(ctx, delay); err != nil {
			return fmt.Errorf("%s canceled while waiting to retry: %w", op, err)
		}
		delay *= 2
		if delay > sendRetryMaxDur {
			delay = sendRetryMaxDur
		}
	}
	return fmt.Errorf("%s: %w", op, lastErr)
}

func isRetryableSendError(err error) bool {
	if err == nil {
		return false
	}
	low := strings.ToLower(err.Error())
	if strings.Contains(low, "status=429") || strings.Contains(low, "code=8") || strings.Contains(low, "rate limit") {
		return true
	}
	if strings.Contains(low, "status=5") || strings.Contains(low, "timeout") || strings.Contains(low, "temporar") {
		return true
	}
	return false
}

func renderRangeViaPDFPNG(
	ctx context.Context,
	cfg workflowConfig,
	exportHTTPClient *http.Client,
	sheetGID int64,
	captureRange string,
) ([]byte, error) {
	if exportHTTPClient == nil {
		return nil, errors.New("google authenticated http client is required")
	}
	exportURL := buildSheetsPDFExportURL(cfg.SheetID, sheetGID, captureRange)
	delay := pdfExportRetryBase
	for attempt := 1; attempt <= pdfExportRetryMax; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, exportURL, nil)
		if err != nil {
			return nil, fmt.Errorf("build sheets export request: %w", err)
		}
		resp, err := exportHTTPClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("request sheets export pdf: %w", err)
		}

		pdfRaw, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			return nil, fmt.Errorf("read sheets export pdf body: %w", readErr)
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			statusErr := fmt.Errorf("sheets export pdf status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(pdfRaw)))
			if attempt == pdfExportRetryMax || !isRetryablePDFExportStatus(resp.StatusCode) {
				return nil, statusErr
			}
			if err = waitWithContext(ctx, delay); err != nil {
				return nil, err
			}
			delay *= 2
			if delay > pdfExportRetryMaxDur {
				delay = pdfExportRetryMaxDur
			}
			continue
		}

		pngRaw, err := convertPDFToPNG(ctx, pdfRaw, cfg.PDFDPI, cfg.PDFConverter, cfg.TempDir)
		if err != nil {
			return nil, err
		}
		return pngRaw, nil
	}
	return nil, errors.New("sheets export pdf retry exhausted")
}

func isRetryablePDFExportStatus(statusCode int) bool {
	return statusCode == http.StatusTooManyRequests || statusCode >= 500
}

func buildSheetsPDFExportURL(sheetID string, gid int64, captureRange string) string {
	values := url.Values{}
	values.Set("format", "pdf")
	values.Set("gid", strconv.FormatInt(gid, 10))
	values.Set("range", strings.TrimSpace(captureRange))
	values.Set("attachment", "false")
	values.Set("sheetnames", "false")
	values.Set("printtitle", "false")
	values.Set("pagenum", "UNDEFINED")
	values.Set("gridlines", "false")
	values.Set("fzr", "false")
	values.Set("fitw", "true")
	values.Set("top_margin", "0")
	values.Set("bottom_margin", "0")
	values.Set("left_margin", "0")
	values.Set("right_margin", "0")
	return fmt.Sprintf("https://docs.google.com/spreadsheets/d/%s/export?%s", url.PathEscape(strings.TrimSpace(sheetID)), values.Encode())
}

func lookupSheetGID(ctx context.Context, sheetsSvc *sheets.Service, sheetID, tab string) (int64, error) {
	resp, err := sheetsSvc.Spreadsheets.Get(sheetID).
		Fields("sheets(properties(sheetId,title))").
		Context(ctx).
		Do()
	if err != nil {
		return 0, fmt.Errorf("load sheet metadata for pdf export: %w", err)
	}
	normalizedTab := normalizeSheetTabName(tab)
	for _, sh := range resp.Sheets {
		if sh == nil || sh.Properties == nil {
			continue
		}
		if normalizeSheetTabName(sh.Properties.Title) == normalizedTab {
			return sh.Properties.SheetId, nil
		}
	}
	return 0, fmt.Errorf("sheet tab %q not found for pdf export", tab)
}

func newGoogleAuthenticatedHTTPClient(ctx context.Context, credsFile, credsJSON string) (*http.Client, error) {
	var (
		raw []byte
		err error
	)
	if strings.TrimSpace(credsJSON) != "" {
		raw = []byte(credsJSON)
	} else {
		raw, err = os.ReadFile(credsFile)
		if err != nil {
			return nil, fmt.Errorf("read google credentials file for pdf export: %w", err)
		}
	}
	creds, err := google.CredentialsFromJSON(ctx, raw, drive.DriveReadonlyScope, sheets.SpreadsheetsReadonlyScope)
	if err != nil {
		return nil, fmt.Errorf("google credentials for pdf export: %w", err)
	}
	return oauth2.NewClient(ctx, creds.TokenSource), nil
}

func convertPDFToPNG(ctx context.Context, pdfRaw []byte, dpi int, converter, tempDir string) ([]byte, error) {
	pdfFile, err := os.CreateTemp(tempDir, "wf3-mdt-*.pdf")
	if err != nil {
		return nil, fmt.Errorf("create temp pdf file: %w", err)
	}
	pdfPath := pdfFile.Name()
	defer os.Remove(pdfPath)

	if _, err = pdfFile.Write(pdfRaw); err != nil {
		pdfFile.Close()
		return nil, fmt.Errorf("write temp pdf file: %w", err)
	}
	if err = pdfFile.Close(); err != nil {
		return nil, fmt.Errorf("close temp pdf file: %w", err)
	}

	resolved := strings.TrimSpace(converter)
	if !isPdftoppmBinary(resolved) && !isMagickBinary(resolved) {
		resolved, err = resolvePDFConverter(converter)
		if err != nil {
			return nil, err
		}
	}

	var pngPath string
	switch {
	case isPdftoppmBinary(resolved):
		outputPrefix := strings.TrimSuffix(pdfPath, ".pdf") + "-page"
		pngPath = outputPrefix + ".png"
		cmd := exec.CommandContext(
			ctx,
			resolved,
			"-png",
			"-f",
			"1",
			"-singlefile",
			"-rx",
			strconv.Itoa(dpi),
			"-ry",
			strconv.Itoa(dpi),
			pdfPath,
			outputPrefix,
		)
		out, runErr := cmd.CombinedOutput()
		if runErr != nil {
			return nil, fmt.Errorf("pdftoppm convert failed: %w output=%s", runErr, strings.TrimSpace(string(out)))
		}
	case isMagickBinary(resolved):
		pngPath = strings.TrimSuffix(pdfPath, ".pdf") + ".png"
		cmd := exec.CommandContext(
			ctx,
			resolved,
			"-density",
			strconv.Itoa(dpi),
			pdfPath+"[0]",
			"-quality",
			"100",
			pngPath,
		)
		out, runErr := cmd.CombinedOutput()
		if runErr != nil {
			return nil, fmt.Errorf("magick convert failed: %w output=%s", runErr, strings.TrimSpace(string(out)))
		}
	default:
		return nil, fmt.Errorf("unsupported pdf converter %q", resolved)
	}
	defer os.Remove(pngPath)

	pngRaw, err := os.ReadFile(pngPath)
	if err != nil {
		return nil, fmt.Errorf("read converted png file: %w", err)
	}
	normalizedPNG, normalizeErr := normalizePDFPNGBottomMargin(pngRaw)
	if normalizeErr == nil && len(normalizedPNG) > 0 {
		pngRaw = normalizedPNG
	}
	return pngRaw, nil
}

func normalizePDFPNGBottomMargin(pngRaw []byte) ([]byte, error) {
	src, _, err := image.Decode(bytes.NewReader(pngRaw))
	if err != nil {
		return nil, fmt.Errorf("decode png for margin normalization: %w", err)
	}
	b := src.Bounds()
	if b.Dx() < 8 || b.Dy() < 8 {
		return pngRaw, nil
	}

	bg := pdfTopBorderColor(src, b)
	topPad := countTopBackgroundRows(src, b, bg)
	bottomPad := countBottomBackgroundRows(src, b, bg)

	const minExtraBottomRows = 24
	if bottomPad <= topPad+minExtraBottomRows {
		return pngRaw, nil
	}

	targetBottomPad := topPad
	removeRows := bottomPad - targetBottomPad
	newMaxY := b.Max.Y - removeRows
	if newMaxY <= b.Min.Y+1 {
		return pngRaw, nil
	}

	cropRect := image.Rect(b.Min.X, b.Min.Y, b.Max.X, newMaxY)
	cropped := image.NewRGBA(image.Rect(0, 0, cropRect.Dx(), cropRect.Dy()))
	draw.Draw(cropped, cropped.Bounds(), src, cropRect.Min, draw.Src)

	var out bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestCompression}
	if err = enc.Encode(&out, cropped); err != nil {
		return nil, fmt.Errorf("encode normalized png: %w", err)
	}
	return out.Bytes(), nil
}

func pdfTopBorderColor(img image.Image, b image.Rectangle) color.RGBA {
	topY := b.Min.Y
	leftX := b.Min.X
	rightX := b.Max.X - 1
	centerX := b.Min.X + b.Dx()/2
	return mostCommonColor(
		pdfColorRGBA(img.At(leftX, topY)),
		pdfColorRGBA(img.At(centerX, topY)),
		pdfColorRGBA(img.At(rightX, topY)),
	)
}

func countTopBackgroundRows(img image.Image, b image.Rectangle, bg color.RGBA) int {
	rows := 0
	for y := b.Min.Y; y < b.Max.Y; y++ {
		if !rowMostlyBackground(img, y, b.Min.X, b.Max.X-1, bg) {
			break
		}
		rows++
	}
	return rows
}

func countBottomBackgroundRows(img image.Image, b image.Rectangle, bg color.RGBA) int {
	rows := 0
	for y := b.Max.Y - 1; y >= b.Min.Y; y-- {
		if !rowMostlyBackground(img, y, b.Min.X, b.Max.X-1, bg) {
			break
		}
		rows++
	}
	return rows
}

func rowMostlyBackground(img image.Image, y, minX, maxX int, bg color.RGBA) bool {
	const (
		maxDeltaPerChannel = 12
		minMatchRatio      = 0.995
	)
	width := maxX - minX + 1
	if width <= 0 {
		return false
	}
	matches := 0
	for x := minX; x <= maxX; x++ {
		c := pdfColorRGBA(img.At(x, y))
		if pdfColorNear(c, bg, maxDeltaPerChannel) {
			matches++
		}
	}
	return float64(matches)/float64(width) >= minMatchRatio
}

func mostCommonColor(colors ...color.RGBA) color.RGBA {
	type key struct {
		r uint8
		g uint8
		b uint8
	}
	counts := map[key]int{}
	values := map[key]color.RGBA{}
	for _, c := range colors {
		k := key{r: c.R >> 2, g: c.G >> 2, b: c.B >> 2}
		counts[k]++
		values[k] = c
	}

	var (
		bestKey   key
		bestCount int
	)
	for k, v := range counts {
		if v > bestCount {
			bestKey = k
			bestCount = v
		}
	}
	return values[bestKey]
}

func pdfColorNear(a, b color.RGBA, maxDelta uint8) bool {
	return pdfAbsInt(int(a.R)-int(b.R)) <= int(maxDelta) &&
		pdfAbsInt(int(a.G)-int(b.G)) <= int(maxDelta) &&
		pdfAbsInt(int(a.B)-int(b.B)) <= int(maxDelta)
}

func pdfColorRGBA(c color.Color) color.RGBA {
	r, g, b, a := c.RGBA()
	return color.RGBA{
		R: uint8(r >> 8),
		G: uint8(g >> 8),
		B: uint8(b >> 8),
		A: uint8(a >> 8),
	}
}

func pdfAbsInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func resolvePDFConverter(preferred string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(preferred)) {
	case "", "auto":
		if path, err := findPdftoppmBinary(); err == nil {
			return path, nil
		}
		if path, err := findMagickBinary(); err == nil {
			return path, nil
		}
		return "", errors.New("pdf_png render mode requires either pdftoppm (Poppler) or magick (ImageMagick) installed")
	case "pdftoppm":
		path, err := findPdftoppmBinary()
		if err != nil {
			return "", errors.New("WF3_PDF_CONVERTER=pdftoppm but pdftoppm is not installed")
		}
		return path, nil
	case "magick":
		path, err := findMagickBinary()
		if err != nil {
			return "", errors.New("WF3_PDF_CONVERTER=magick but ImageMagick (magick) is not installed")
		}
		return path, nil
	default:
		return "", fmt.Errorf("unsupported pdf converter %q", preferred)
	}
}

func findPdftoppmBinary() (string, error) {
	if path, err := exec.LookPath("pdftoppm"); err == nil {
		return path, nil
	}
	base := strings.TrimSpace(os.Getenv("LOCALAPPDATA"))
	if base == "" {
		return "", errors.New("pdftoppm not found in PATH")
	}
	pattern := filepath.Join(
		base,
		"Microsoft",
		"WinGet",
		"Packages",
		"oschwartz10612.Poppler_*",
		"poppler-*",
		"Library",
		"bin",
		"pdftoppm.exe",
	)
	matches, err := filepath.Glob(pattern)
	if err != nil || len(matches) == 0 {
		return "", errors.New("pdftoppm not found in PATH or WinGet Poppler folder")
	}
	return matches[0], nil
}

func findMagickBinary() (string, error) {
	if path, err := exec.LookPath("magick"); err == nil {
		return path, nil
	}
	return "", errors.New("magick not found in PATH")
}

func isPdftoppmBinary(path string) bool {
	base := strings.ToLower(filepath.Base(path))
	return strings.HasPrefix(base, "pdftoppm")
}

func isMagickBinary(path string) bool {
	base := strings.ToLower(filepath.Base(path))
	return strings.HasPrefix(base, "magick")
}

func waitForStableRangeDigest(
	ctx context.Context,
	svc *sheets.Service,
	sheetID, tab, captureRange string,
	runs int,
	interval time.Duration,
) (bool, error) {
	if runs < 1 {
		runs = 1
	}
	if interval <= 0 {
		interval = 1 * time.Second
	}

	previous := ""
	for i := 1; i <= runs; i++ {
		digest, err := captureValuesDigest(ctx, svc, sheetID, tab, captureRange)
		if err != nil {
			return false, err
		}
		if i > 1 && digest == previous {
			return true, nil
		}
		previous = digest
		if i < runs {
			if err := waitWithContext(ctx, interval); err != nil {
				return false, err
			}
		}
	}
	return false, nil
}

func waitWithContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
}

func captureValuesDigest(ctx context.Context, svc *sheets.Service, sheetID, tab, captureRange string) (string, error) {
	rangeRef := buildSheetRangeRef(tab, captureRange)
	resp, err := svc.Spreadsheets.Values.BatchGet(sheetID).
		Ranges(rangeRef).
		ValueRenderOption("FORMATTED_VALUE").
		Context(ctx).
		Do()
	if err != nil {
		return "", fmt.Errorf("read snapshot values: %w", err)
	}

	var builder strings.Builder
	for idx, vr := range resp.ValueRanges {
		builder.WriteString(fmt.Sprintf("[%d]%s\n", idx, vr.Range))
		for _, row := range vr.Values {
			for col := range row {
				if col > 0 {
					builder.WriteRune('\t')
				}
				builder.WriteString(strings.TrimSpace(fmt.Sprint(row[col])))
			}
			builder.WriteRune('\n')
		}
		builder.WriteString("--\n")
	}

	sum := sha256.Sum256([]byte(builder.String()))
	return hex.EncodeToString(sum[:]), nil
}

func buildSheetRangeRef(tab, captureRange string) string {
	t := normalizeSheetTabName(tab)
	t = strings.ReplaceAll(t, "'", "''")
	return fmt.Sprintf("'%s'!%s", t, strings.TrimSpace(captureRange))
}

func normalizeSheetTabName(tab string) string {
	t := strings.TrimSpace(tab)
	if len(t) >= 2 && strings.HasPrefix(t, "'") && strings.HasSuffix(t, "'") {
		t = strings.TrimPrefix(strings.TrimSuffix(t, "'"), "'")
	}
	return strings.ReplaceAll(t, "''", "'")
}

func loadConfig() (workflowConfig, error) {
	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		return workflowConfig{}, fmt.Errorf("load .env: %w", err)
	}

	bootstrapSendExisting, err := getBoolEnv("WF3_BOOTSTRAP_SEND_EXISTING", false)
	if err != nil {
		return workflowConfig{}, err
	}
	dryRun, err := getBoolEnv("WF3_DRY_RUN", false)
	if err != nil {
		return workflowConfig{}, err
	}
	continuous, err := getBoolEnv("WF3_CONTINUOUS", true)
	if err != nil {
		return workflowConfig{}, err
	}
	testSendOnce, err := getBoolEnv("WF3_TEST_SEND_ONCE", false)
	if err != nil {
		return workflowConfig{}, err
	}
	enableHealth, err := getBoolEnv("WF3_ENABLE_HEALTH_SERVER", true)
	if err != nil {
		return workflowConfig{}, err
	}

	credsFile := firstNonEmpty(
		strings.TrimSpace(os.Getenv("WF3_GOOGLE_CREDENTIALS_FILE")),
		strings.TrimSpace(os.Getenv("WF21_GOOGLE_CREDENTIALS_FILE")),
		strings.TrimSpace(os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")),
	)
	credsJSON := firstNonEmpty(
		strings.TrimSpace(os.Getenv("WF3_GOOGLE_CREDENTIALS_JSON")),
		strings.TrimSpace(os.Getenv("WF21_GOOGLE_CREDENTIALS_JSON")),
	)
	if credsFile == "" && credsJSON == "" {
		return workflowConfig{}, errors.New("set WF3_GOOGLE_CREDENTIALS_FILE/GOOGLE_APPLICATION_CREDENTIALS or WF3_GOOGLE_CREDENTIALS_JSON")
	}

	appID := strings.TrimSpace(firstNonEmpty(
		os.Getenv("WF3_SEATALK_APP_ID"),
		os.Getenv("WF21_SEATALK_APP_ID"),
		os.Getenv("SEATALK_APP_ID"),
	))
	appSecret := strings.TrimSpace(firstNonEmpty(
		os.Getenv("WF3_SEATALK_APP_SECRET"),
		os.Getenv("WF21_SEATALK_APP_SECRET"),
		os.Getenv("SEATALK_APP_SECRET"),
	))
	groupID := strings.TrimSpace(firstNonEmpty(
		os.Getenv("WF3_SEATALK_GROUP_ID"),
		os.Getenv("WF21_SEATALK_GROUP_ID"),
		os.Getenv("WF2_SEATALK_GROUP_ID"),
	))
	if appID == "" || appSecret == "" {
		return workflowConfig{}, errors.New("set WF3_SEATALK_APP_ID and WF3_SEATALK_APP_SECRET")
	}
	if groupID == "" {
		return workflowConfig{}, errors.New("set WF3_SEATALK_GROUP_ID")
	}

	pdfConverter := strings.ToLower(strings.TrimSpace(firstNonEmpty(os.Getenv("WF3_PDF_CONVERTER"), defaultPDFConverter)))
	resolvedConverter, err := resolvePDFConverter(pdfConverter)
	if err != nil {
		return workflowConfig{}, err
	}

	imageRangesRaw := firstNonEmpty(strings.TrimSpace(os.Getenv("WF3_IMAGE_RANGES")), defaultImageRanges)
	imageRanges, err := parseImageRanges(imageRangesRaw)
	if err != nil {
		return workflowConfig{}, fmt.Errorf("invalid WF3_IMAGE_RANGES: %w", err)
	}

	image1FixedHideRowsRaw := firstNonEmpty(strings.TrimSpace(os.Getenv("WF3_IMAGE1_FIXED_HIDE_ROWS")), defaultImage1FixedHideRows)
	image1FixedHideRows, err := parseRowSet(image1FixedHideRowsRaw)
	if err != nil {
		return workflowConfig{}, fmt.Errorf("invalid WF3_IMAGE1_FIXED_HIDE_ROWS: %w", err)
	}

	statusFile := strings.TrimSpace(os.Getenv("WF3_STATUS_FILE"))
	switch strings.ToLower(statusFile) {
	case "none", "off":
		statusFile = ""
	case "":
		statusFile = defaultStatusFile
	}

	timeZone := firstNonEmpty(
		strings.TrimSpace(os.Getenv("WF3_TIMEZONE")),
		strings.TrimSpace(os.Getenv("WF21_TIMEZONE")),
		defaultTimezone,
	)
	loc, err := time.LoadLocation(timeZone)
	if err != nil {
		return workflowConfig{}, fmt.Errorf("invalid WF3_TIMEZONE %q: %w", timeZone, err)
	}

	healthPort := firstNonEmpty(
		strings.TrimSpace(os.Getenv("WF3_HEALTH_PORT")),
		strings.TrimSpace(os.Getenv("PORT")),
		"8080",
	)

	cfg := workflowConfig{
		SheetID:      firstNonEmpty(strings.TrimSpace(os.Getenv("WF3_SHEET_ID")), defaultSheetID),
		SheetTab:     firstNonEmpty(strings.TrimSpace(os.Getenv("WF3_SHEET_TAB")), defaultSheetTab),
		MonitorRange: firstNonEmpty(strings.TrimSpace(os.Getenv("WF3_MONITOR_RANGE")), defaultMonitorRange),
		ImageRanges:  imageRanges,

		Image1FixedHideRows: image1FixedHideRows,

		GoogleCredentialsFile: credsFile,
		GoogleCredentialsJSON: credsJSON,

		SeaTalkAppID:     appID,
		SeaTalkAppSecret: appSecret,
		SeaTalkBaseURL: strings.TrimSpace(firstNonEmpty(
			os.Getenv("WF3_SEATALK_BASE_URL"),
			os.Getenv("WF21_SEATALK_BASE_URL"),
			os.Getenv("SEATALK_BASE_URL"),
			"https://openapi.seatalk.io",
		)),
		SeaTalkGroupID: groupID,

		Continuous:            continuous,
		BootstrapSendExisting: bootstrapSendExisting,
		DryRun:                dryRun,
		TestSendOnce:          testSendOnce,
		PollInterval:          getDurationSeconds("WF3_POLL_INTERVAL_SECONDS", int(defaultPollInterval/time.Second)),
		SendDebounce:          getDurationSeconds("WF3_SEND_DEBOUNCE_SECONDS", int(defaultDebounce/time.Second)),
		SendMinGap:            getDurationSeconds("WF3_SEND_MIN_INTERVAL_SECONDS", int(defaultSendMinGap/time.Second)),
		HTTPTimeout:           getDurationSeconds("WF3_HTTP_TIMEOUT_SECONDS", int(defaultHTTPTimeout/time.Second)),
		TimeZone:              timeZone,
		Location:              loc,
		StabilityRuns:         getIntEnv("WF3_STABILITY_RUNS", defaultStabilityRuns),
		StabilityWait:         getDurationSeconds("WF3_STABILITY_WAIT_SECONDS", int(defaultStabilityWait/time.Second)),

		PDFDPI:       getIntEnv("WF3_PDF_DPI", defaultPDFDPI),
		PDFConverter: resolvedConverter,
		TempDir:      strings.TrimSpace(os.Getenv("WF3_TEMP_DIR")),

		MaxImageB64Size: getIntEnv("WF3_IMAGE_MAX_BASE64_BYTES", defaultMaxImageB64),

		StateFile:  firstNonEmpty(strings.TrimSpace(os.Getenv("WF3_STATE_FILE")), defaultStateFile),
		StatusFile: statusFile,

		EnableHealthServer: enableHealth,
		HealthListenAddr:   normalizeListenAddr(healthPort),
	}

	if cfg.StabilityRuns < 2 {
		cfg.StabilityRuns = 2
	}
	if cfg.StabilityWait < time.Second {
		cfg.StabilityWait = time.Second
	}
	if cfg.SendDebounce < time.Second {
		cfg.SendDebounce = time.Second
	}
	if cfg.SendMinGap < 500*time.Millisecond {
		cfg.SendMinGap = 500 * time.Millisecond
	}
	if cfg.MaxImageB64Size < 500000 {
		cfg.MaxImageB64Size = defaultMaxImageB64
	}
	return cfg, nil
}

func parseImageRanges(raw string) ([]imageRangeSpec, error) {
	parts := strings.Split(raw, ",")
	out := make([]imageRangeSpec, 0, len(parts))
	for idx, rawPart := range parts {
		part := strings.TrimSpace(rawPart)
		if part == "" {
			continue
		}
		rangePart := part
		if strings.Contains(part, "!") {
			items := strings.SplitN(part, "!", 2)
			rangePart = strings.TrimSpace(items[1])
		}
		parsed, err := parseA1Range(rangePart)
		if err != nil {
			return nil, err
		}
		out = append(out, imageRangeSpec{
			Label:    fmt.Sprintf("image%d", idx+1),
			StartCol: parsed.startCol,
			EndCol:   parsed.endCol,
			StartRow: parsed.startRow,
			EndRow:   parsed.endRow,
		})
	}
	if len(out) == 0 {
		return nil, errors.New("at least one image range is required")
	}
	return out, nil
}

func parseRowSet(raw string) (map[int]struct{}, error) {
	result := map[int]struct{}{}
	if strings.TrimSpace(raw) == "" {
		return result, nil
	}
	parts := strings.Split(raw, ",")
	for _, part := range parts {
		token := strings.TrimSpace(part)
		if token == "" {
			continue
		}
		if strings.Contains(token, "-") {
			bounds := strings.SplitN(token, "-", 2)
			if len(bounds) != 2 {
				return nil, fmt.Errorf("invalid row range %q", token)
			}
			start, err := strconv.Atoi(strings.TrimSpace(bounds[0]))
			if err != nil || start <= 0 {
				return nil, fmt.Errorf("invalid row number in %q", token)
			}
			end, err := strconv.Atoi(strings.TrimSpace(bounds[1]))
			if err != nil || end <= 0 {
				return nil, fmt.Errorf("invalid row number in %q", token)
			}
			if end < start {
				return nil, fmt.Errorf("invalid descending row range %q", token)
			}
			for row := start; row <= end; row++ {
				result[row] = struct{}{}
			}
			continue
		}
		row, err := strconv.Atoi(token)
		if err != nil || row <= 0 {
			return nil, fmt.Errorf("invalid row number %q", token)
		}
		result[row] = struct{}{}
	}
	return result, nil
}

func parseA1Range(raw string) (sheetRange, error) {
	ref := strings.TrimSpace(raw)
	parts := strings.Split(ref, ":")
	if len(parts) != 2 {
		return sheetRange{}, fmt.Errorf("invalid A1 range %q", raw)
	}
	startCol, startRow, err := parseCellRef(parts[0])
	if err != nil {
		return sheetRange{}, err
	}
	endCol, endRow, err := parseCellRef(parts[1])
	if err != nil {
		return sheetRange{}, err
	}
	if endCol < startCol || endRow < startRow {
		return sheetRange{}, fmt.Errorf("invalid A1 range %q", raw)
	}
	return sheetRange{
		startCol: startCol,
		endCol:   endCol,
		startRow: startRow,
		endRow:   endRow,
	}, nil
}

func parseCellRef(raw string) (col int, row int, err error) {
	re := regexp.MustCompile(`(?i)^([A-Z]+)(\d+)$`)
	matches := re.FindStringSubmatch(strings.TrimSpace(raw))
	if len(matches) != 3 {
		return 0, 0, fmt.Errorf("invalid cell reference %q", raw)
	}
	colLabel := strings.ToUpper(matches[1])
	rowVal, convErr := strconv.Atoi(matches[2])
	if convErr != nil || rowVal <= 0 {
		return 0, 0, fmt.Errorf("invalid row in cell reference %q", raw)
	}

	colVal := 0
	for _, ch := range colLabel {
		colVal = colVal*26 + int(ch-'A'+1)
	}
	return colVal, rowVal, nil
}

func columnIndexToName(idx int) string {
	if idx <= 0 {
		return ""
	}
	var out []byte
	for idx > 0 {
		idx--
		out = append([]byte{byte('A' + (idx % 26))}, out...)
		idx /= 26
	}
	return string(out)
}

func encodeImageWithinLimit(pngRaw []byte, maxBase64Bytes int) (base64Content string, format string, rawSize int, err error) {
	encoded := base64.StdEncoding.EncodeToString(pngRaw)
	if len(encoded) <= maxBase64Bytes {
		return encoded, "png", len(pngRaw), nil
	}

	src, _, decodeErr := image.Decode(bytes.NewReader(pngRaw))
	if decodeErr != nil {
		return "", "", 0, fmt.Errorf("decode png for jpeg fallback: %w", decodeErr)
	}

	qualities := []int{92, 86, 80, 74, 68}
	for _, quality := range qualities {
		var buf bytes.Buffer
		if encodeErr := jpeg.Encode(&buf, src, &jpeg.Options{Quality: quality}); encodeErr != nil {
			return "", "", 0, fmt.Errorf("encode jpeg quality=%d: %w", quality, encodeErr)
		}
		jpegRaw := buf.Bytes()
		encoded = base64.StdEncoding.EncodeToString(jpegRaw)
		if len(encoded) <= maxBase64Bytes {
			return encoded, "jpeg", len(jpegRaw), nil
		}
	}

	return "", "", 0, fmt.Errorf("image exceeds seatalk size limit: base64 bytes=%d max=%d", len(encoded), maxBase64Bytes)
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

func newReadonlySheetsService(ctx context.Context, credsFile, credsJSON string) (*sheets.Service, error) {
	options := []option.ClientOption{
		option.WithScopes(sheets.SpreadsheetsReadonlyScope),
	}
	if strings.TrimSpace(credsJSON) != "" {
		options = append(options, option.WithCredentialsJSON([]byte(credsJSON)))
	} else {
		options = append(options, option.WithCredentialsFile(credsFile))
	}
	return sheets.NewService(ctx, options...)
}

func loadState(path string) (workflowState, bool, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return workflowState{}, false, nil
	}
	if err != nil {
		return workflowState{}, false, err
	}
	var parsed workflowState
	if err = json.Unmarshal(raw, &parsed); err != nil {
		return workflowState{}, false, fmt.Errorf("decode state file %s: %w", path, err)
	}
	return parsed, true, nil
}

func saveState(path string, state workflowState) error {
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func parseRFC3339OrNow(raw string, fallback time.Time) time.Time {
	ts := strings.TrimSpace(raw)
	if ts == "" {
		return fallback
	}
	parsed, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return fallback
	}
	return parsed
}

func shortDigest(v string) string {
	if len(v) <= 12 {
		return v
	}
	return v[:12]
}

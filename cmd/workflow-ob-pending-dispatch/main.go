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
	"log"
	"math"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/go-fonts/liberation/liberationmonobold"
	"github.com/go-fonts/liberation/liberationmonobolditalic"
	"github.com/go-fonts/liberation/liberationmonoitalic"
	"github.com/go-fonts/liberation/liberationmonoregular"
	"github.com/go-fonts/liberation/liberationsansbold"
	"github.com/go-fonts/liberation/liberationsansbolditalic"
	"github.com/go-fonts/liberation/liberationsansitalic"
	"github.com/go-fonts/liberation/liberationsansregular"
	"github.com/go-fonts/liberation/liberationserifbold"
	"github.com/go-fonts/liberation/liberationserifbolditalic"
	"github.com/go-fonts/liberation/liberationserifitalic"
	"github.com/go-fonts/liberation/liberationserifregular"
	"github.com/joho/godotenv"
	"github.com/spxph4227/go-bot-server/internal/seatalk"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"
)

const (
	workflowName = "workflow_2_ob_pending_dispatch"

	defaultSheetID      = "17cvCc6ffMXNs6JYnpMYvDO_V8nBCRKRm3G78oINj_yo"
	defaultSheetTab     = "Backlogs Summary"
	defaultCaptureRange = "C2:S64"
	defaultSecondTab    = "SOLIIS & MINDANAO"
	defaultSecondRange  = "B1:K41"
	defaultTriggerCell  = "G4"

	defaultStateFile  = "data/workflow2-ob-pending-dispatch-state.json"
	defaultStatusFile = "data/workflow2-ob-pending-dispatch-status.json"

	defaultPollInterval  = 10 * time.Second
	defaultHTTPTimeout   = 10 * time.Second
	defaultMaxImageWidth = 3000
	defaultMaxImageB64   = 5 * 1024 * 1024
	defaultTimezone      = "Asia/Manila"
	defaultRenderScale   = 2
	defaultSecondScale   = 1
	defaultStabilityWait = 2 * time.Second
	defaultStabilityRuns = 3
)

type workflowConfig struct {
	SheetID               string
	SheetTab              string
	CaptureRange          string
	SecondCaptureTab      string
	SecondCaptureRange    string
	TriggerCell           string
	GoogleCredentialsFile string
	GoogleCredentialsJSON string

	SeaTalkAppID     string
	SeaTalkAppSecret string
	SeaTalkBaseURL   string
	SeaTalkGroupID   string

	Continuous            bool
	BootstrapSendExisting bool
	DryRun                bool
	PollInterval          time.Duration
	HTTPTimeout           time.Duration

	TimeZone          string
	MaxImageWidthPx   int
	MaxImageB64Size   int
	RenderScale       int
	SecondRenderScale int
	StabilityWait     time.Duration
	StabilityRuns     int

	StateFile  string
	StatusFile string

	EnableHealthServer bool
	HealthListenAddr   string
}

type workflowState struct {
	LastTriggerValue string `json:"last_trigger_value"`
	LastSeenAt       string `json:"last_seen_at,omitempty"`
	LastSentAt       string `json:"last_sent_at,omitempty"`
}

type workflowStatus struct {
	Workflow        string `json:"workflow"`
	Continuous      bool   `json:"continuous"`
	DryRun          bool   `json:"dry_run"`
	Cycle           int    `json:"cycle"`
	LastCycleAt     string `json:"last_cycle_at"`
	Changed         bool   `json:"changed"`
	TriggerCell     string `json:"trigger_cell"`
	TriggerValue    string `json:"trigger_value"`
	LastTrigger     string `json:"last_trigger_value"`
	LastSentAt      string `json:"last_sent_at,omitempty"`
	LastImageFormat string `json:"last_image_format,omitempty"`
	LastImageBytes  int    `json:"last_image_bytes,omitempty"`
	LastImageCount  int    `json:"last_image_count,omitempty"`
	StateFile       string `json:"state_file"`
	StatusFile      string `json:"status_file,omitempty"`
}

type sheetRange struct {
	startCol int
	endCol   int
	startRow int
	endRow   int
}

type styledRangeData struct {
	Rows       int
	Cols       int
	RowHeights []int
	ColWidths  []int
	Cells      [][]styledCell
	Merges     []mergeRegion
}

type styledCell struct {
	Text         string
	Background   color.RGBA
	Foreground   color.RGBA
	FontFamily   string
	Bold         bool
	Italic       bool
	Underline    bool
	Strikethru   bool
	FontSize     float64
	HAlign       string
	VAlign       string
	WrapStrategy string
	Borders      styledBorders
}

type styledBorders struct {
	Top    styledBorder
	Bottom styledBorder
	Left   styledBorder
	Right  styledBorder
}

type styledBorder struct {
	Style string
	Color color.RGBA
}

type mergeRegion struct {
	StartRow int
	EndRow   int
	StartCol int
	EndCol   int
}

type captureTarget struct {
	Tab         string
	Range       string
	RenderScale int
}

var (
	fontInitOnce sync.Once
	fontInitErr  error
	regularSans  *opentype.Font
	boldSans     *opentype.Font
	italicSans   *opentype.Font
	boldItSans   *opentype.Font
	regularSerif *opentype.Font
	boldSerif    *opentype.Font
	italicSerif  *opentype.Font
	boldItSerif  *opentype.Font
	regularMono  *opentype.Font
	boldMono     *opentype.Font
	italicMono   *opentype.Font
	boldItMono   *opentype.Font
	fontFaceMu   sync.Mutex
	fontFaceMap  = map[string]font.Face{}
)

func main() {
	logger := log.New(os.Stdout, "[workflow-ob-pending-dispatch] ", log.LstdFlags|log.Lmsgprefix)

	cfg, err := loadConfig()
	if err != nil {
		logger.Fatalf("config error: %v", err)
	}

	loc, err := time.LoadLocation(cfg.TimeZone)
	if err != nil {
		logger.Fatalf("timezone error: %v", err)
	}

	state, stateExists, err := loadState(cfg.StateFile)
	if err != nil {
		logger.Fatalf("state load error: %v", err)
	}

	sheetsSvc, err := newSheetsService(context.Background(), cfg)
	if err != nil {
		logger.Fatalf("google sheets init error: %v", err)
	}

	seaTalkClient := seatalk.NewClient(seatalk.ClientConfig{
		AppID:     cfg.SeaTalkAppID,
		AppSecret: cfg.SeaTalkAppSecret,
		BaseURL:   cfg.SeaTalkBaseURL,
		Timeout:   cfg.HTTPTimeout,
	})

	if !cfg.Continuous {
		if err = runCycle(context.Background(), cfg, sheetsSvc, seaTalkClient, loc, &state, &stateExists, logger, 1); err != nil {
			logger.Fatalf("workflow failed: %v", err)
		}
		return
	}

	sigCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if cfg.EnableHealthServer {
		startHealthServer(sigCtx, cfg, logger)
	}

	logger.Printf("watch mode enabled poll_interval=%s trigger=%s!%s", cfg.PollInterval, cfg.SheetTab, cfg.TriggerCell)
	cycle := 1
	for {
		if err = runCycle(sigCtx, cfg, sheetsSvc, seaTalkClient, loc, &state, &stateExists, logger, cycle); err != nil {
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
	svc *sheets.Service,
	seaTalkClient *seatalk.Client,
	loc *time.Location,
	state *workflowState,
	stateExists *bool,
	logger *log.Logger,
	cycle int,
) error {
	now := time.Now().UTC()
	triggerValue, err := readSingleCell(ctx, svc, cfg.SheetID, cfg.SheetTab, cfg.TriggerCell)
	if err != nil {
		return err
	}
	triggerValue = strings.TrimSpace(triggerValue)

	changed := triggerValue != strings.TrimSpace(state.LastTriggerValue)
	if !*stateExists && !cfg.BootstrapSendExisting {
		changed = false
		state.LastTriggerValue = triggerValue
		state.LastSeenAt = now.Format(time.RFC3339)
		if err = saveState(cfg.StateFile, *state); err != nil {
			return err
		}
		*stateExists = true
		logger.Printf("baseline set trigger_cell=%s value=%q", cfg.TriggerCell, triggerValue)
	}

	lastImageFmt := ""
	lastImageSize := 0
	lastImageCount := 0

	if changed {
		targets := buildCaptureTargets(cfg)
		stable, stableTrigger, stableErr := waitForStableSnapshots(ctx, cfg, svc, triggerValue, targets)
		if stableErr != nil {
			return stableErr
		}
		if !stable {
			logger.Printf("change pending stabilization trigger_cell=%s old=%q new=%q stable_now=%q", cfg.TriggerCell, state.LastTriggerValue, triggerValue, stableTrigger)
			status := workflowStatus{
				Workflow:       workflowName,
				Continuous:     cfg.Continuous,
				DryRun:         cfg.DryRun,
				Cycle:          cycle,
				LastCycleAt:    now.Format(time.RFC3339),
				Changed:        true,
				TriggerCell:    cfg.TriggerCell,
				TriggerValue:   triggerValue,
				LastTrigger:    state.LastTriggerValue,
				LastSentAt:     state.LastSentAt,
				StateFile:      cfg.StateFile,
				StatusFile:     cfg.StatusFile,
				LastImageCount: 0,
			}
			if cfg.StatusFile != "" {
				if statusErr := saveStatus(cfg.StatusFile, status); statusErr != nil {
					logger.Printf("status write failed path=%s err=%v", cfg.StatusFile, statusErr)
				}
			}
			return nil
		}

		announce := fmt.Sprintf(
			"<mention-tag target=\"seatalk://user?id=0\"/> OB Pending for Dispatch as of %s",
			time.Now().In(loc).Format("3:04 PM Jan-02"),
		)

		if cfg.DryRun {
			for idx, target := range targets {
				styledRange, readErr := readStyledRange(ctx, svc, cfg.SheetID, target.Tab, target.Range)
				if readErr != nil {
					return readErr
				}
				pngRaw, renderErr := renderStyledRangeImage(styledRange, cfg.MaxImageWidthPx, target.RenderScale)
				if renderErr != nil {
					return renderErr
				}
				_, imageFmt, imageBytes, encodeErr := encodeImageWithinLimit(pngRaw, cfg.MaxImageB64Size)
				if encodeErr != nil {
					return encodeErr
				}
				lastImageFmt = imageFmt
				lastImageSize += imageBytes
				lastImageCount++
				logger.Printf("dry_run=true changed=true trigger_cell=%s old=%q new=%q capture=%d/%d range=%s!%s render_scale=%d image_format=%s image_bytes=%d", cfg.TriggerCell, state.LastTriggerValue, triggerValue, idx+1, len(targets), target.Tab, target.Range, target.RenderScale, imageFmt, imageBytes)
			}
		} else {
			if sendErr := seaTalkClient.SendTextToGroup(ctx, cfg.SeaTalkGroupID, announce, 1); sendErr != nil {
				return fmt.Errorf("send text: %w", sendErr)
			}
			for idx, target := range targets {
				styledRange, readErr := readStyledRange(ctx, svc, cfg.SheetID, target.Tab, target.Range)
				if readErr != nil {
					return readErr
				}
				pngRaw, renderErr := renderStyledRangeImage(styledRange, cfg.MaxImageWidthPx, target.RenderScale)
				if renderErr != nil {
					return renderErr
				}
				base64Image, imageFmt, imageBytes, encodeErr := encodeImageWithinLimit(pngRaw, cfg.MaxImageB64Size)
				if encodeErr != nil {
					return encodeErr
				}
				if sendErr := seaTalkClient.SendImageToGroupBase64(ctx, cfg.SeaTalkGroupID, base64Image); sendErr != nil {
					return fmt.Errorf("send image (%s!%s): %w", target.Tab, target.Range, sendErr)
				}
				lastImageFmt = imageFmt
				lastImageSize += imageBytes
				lastImageCount++
				logger.Printf("sent image trigger_cell=%s old=%q new=%q capture=%d/%d range=%s!%s render_scale=%d image_format=%s image_bytes=%d", cfg.TriggerCell, state.LastTriggerValue, triggerValue, idx+1, len(targets), target.Tab, target.Range, target.RenderScale, imageFmt, imageBytes)
			}
			state.LastSentAt = now.Format(time.RFC3339)
			logger.Printf("sent changed=true trigger_cell=%s old=%q new=%q image_count=%d image_bytes_total=%d", cfg.TriggerCell, state.LastTriggerValue, triggerValue, lastImageCount, lastImageSize)
		}
	}

	state.LastTriggerValue = triggerValue
	state.LastSeenAt = now.Format(time.RFC3339)
	if err = saveState(cfg.StateFile, *state); err != nil {
		return err
	}
	*stateExists = true

	status := workflowStatus{
		Workflow:        workflowName,
		Continuous:      cfg.Continuous,
		DryRun:          cfg.DryRun,
		Cycle:           cycle,
		LastCycleAt:     now.Format(time.RFC3339),
		Changed:         changed,
		TriggerCell:     cfg.TriggerCell,
		TriggerValue:    triggerValue,
		LastTrigger:     state.LastTriggerValue,
		LastSentAt:      state.LastSentAt,
		LastImageFormat: lastImageFmt,
		LastImageBytes:  lastImageSize,
		LastImageCount:  lastImageCount,
		StateFile:       cfg.StateFile,
		StatusFile:      cfg.StatusFile,
	}
	if cfg.StatusFile != "" {
		if statusErr := saveStatus(cfg.StatusFile, status); statusErr != nil {
			logger.Printf("status write failed path=%s err=%v", cfg.StatusFile, statusErr)
		}
	}

	if !changed {
		logger.Printf("no change trigger_cell=%s value=%q", cfg.TriggerCell, triggerValue)
	}
	return nil
}

func loadConfig() (workflowConfig, error) {
	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		return workflowConfig{}, fmt.Errorf("load .env: %w", err)
	}

	bootstrapSendExisting, err := getBoolEnv("WF2_BOOTSTRAP_SEND_EXISTING", false)
	if err != nil {
		return workflowConfig{}, err
	}
	dryRun, err := getBoolEnv("WF2_DRY_RUN", false)
	if err != nil {
		return workflowConfig{}, err
	}
	continuous, err := getBoolEnv("WF2_CONTINUOUS", true)
	if err != nil {
		return workflowConfig{}, err
	}
	enableHealth, err := getBoolEnv("WF2_ENABLE_HEALTH_SERVER", true)
	if err != nil {
		return workflowConfig{}, err
	}

	credsFile := firstNonEmpty(
		strings.TrimSpace(os.Getenv("WF2_GOOGLE_CREDENTIALS_FILE")),
		strings.TrimSpace(os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")),
	)
	credsJSON := strings.TrimSpace(os.Getenv("WF2_GOOGLE_CREDENTIALS_JSON"))
	if credsFile == "" && credsJSON == "" {
		return workflowConfig{}, errors.New("set WF2_GOOGLE_CREDENTIALS_FILE/GOOGLE_APPLICATION_CREDENTIALS or WF2_GOOGLE_CREDENTIALS_JSON")
	}

	appID := strings.TrimSpace(firstNonEmpty(os.Getenv("WF2_SEATALK_APP_ID"), os.Getenv("SEATALK_APP_ID")))
	appSecret := strings.TrimSpace(firstNonEmpty(os.Getenv("WF2_SEATALK_APP_SECRET"), os.Getenv("SEATALK_APP_SECRET")))
	if appID == "" || appSecret == "" {
		return workflowConfig{}, errors.New("set WF2_SEATALK_APP_ID/WF2_SEATALK_APP_SECRET or SEATALK_APP_ID/SEATALK_APP_SECRET")
	}

	groupID := strings.TrimSpace(os.Getenv("WF2_SEATALK_GROUP_ID"))
	if groupID == "" {
		return workflowConfig{}, errors.New("WF2_SEATALK_GROUP_ID is required")
	}

	statusFile := strings.TrimSpace(os.Getenv("WF2_STATUS_FILE"))
	switch strings.ToLower(statusFile) {
	case "none", "off":
		statusFile = ""
	case "":
		statusFile = defaultStatusFile
	}

	healthPort := firstNonEmpty(
		strings.TrimSpace(os.Getenv("WF2_HEALTH_PORT")),
		strings.TrimSpace(os.Getenv("PORT")),
		"8080",
	)

	cfg := workflowConfig{
		SheetID:               firstNonEmpty(strings.TrimSpace(os.Getenv("WF2_SHEET_ID")), defaultSheetID),
		SheetTab:              firstNonEmpty(strings.TrimSpace(os.Getenv("WF2_SHEET_TAB")), defaultSheetTab),
		CaptureRange:          firstNonEmpty(strings.TrimSpace(os.Getenv("WF2_CAPTURE_RANGE")), defaultCaptureRange),
		SecondCaptureTab:      firstNonEmpty(strings.TrimSpace(os.Getenv("WF2_SECOND_CAPTURE_TAB")), defaultSecondTab),
		SecondCaptureRange:    firstNonEmpty(strings.TrimSpace(os.Getenv("WF2_SECOND_CAPTURE_RANGE")), defaultSecondRange),
		TriggerCell:           firstNonEmpty(strings.TrimSpace(os.Getenv("WF2_TRIGGER_CELL")), defaultTriggerCell),
		GoogleCredentialsFile: credsFile,
		GoogleCredentialsJSON: credsJSON,
		SeaTalkAppID:          appID,
		SeaTalkAppSecret:      appSecret,
		SeaTalkBaseURL:        firstNonEmpty(strings.TrimSpace(os.Getenv("SEATALK_BASE_URL")), "https://openapi.seatalk.io"),
		SeaTalkGroupID:        groupID,
		Continuous:            continuous,
		BootstrapSendExisting: bootstrapSendExisting,
		DryRun:                dryRun,
		PollInterval:          getDurationSeconds("WF2_POLL_INTERVAL_SECONDS", int(defaultPollInterval/time.Second)),
		HTTPTimeout:           getDurationSeconds("WF2_HTTP_TIMEOUT_SECONDS", int(defaultHTTPTimeout/time.Second)),
		TimeZone:              firstNonEmpty(strings.TrimSpace(os.Getenv("WF2_TIMEZONE")), defaultTimezone),
		MaxImageWidthPx:       getIntEnv("WF2_IMAGE_MAX_WIDTH_PX", defaultMaxImageWidth),
		MaxImageB64Size:       getIntEnv("WF2_IMAGE_MAX_BASE64_BYTES", defaultMaxImageB64),
		RenderScale:           getIntEnv("WF2_RENDER_SCALE", defaultRenderScale),
		SecondRenderScale:     getIntEnv("WF2_SECOND_RENDER_SCALE", defaultSecondScale),
		StabilityWait:         getDurationSeconds("WF2_STABILITY_WAIT_SECONDS", int(defaultStabilityWait/time.Second)),
		StabilityRuns:         getIntEnv("WF2_STABILITY_RUNS", defaultStabilityRuns),
		StateFile:             firstNonEmpty(strings.TrimSpace(os.Getenv("WF2_STATE_FILE")), defaultStateFile),
		StatusFile:            statusFile,
		EnableHealthServer:    enableHealth,
		HealthListenAddr:      normalizeListenAddr(healthPort),
	}

	if cfg.MaxImageWidthPx < 1200 {
		cfg.MaxImageWidthPx = 1200
	}
	if cfg.MaxImageB64Size < 500000 {
		cfg.MaxImageB64Size = defaultMaxImageB64
	}
	if cfg.RenderScale < 1 {
		cfg.RenderScale = 1
	}
	if cfg.RenderScale > 4 {
		cfg.RenderScale = 4
	}
	if cfg.SecondRenderScale < 1 {
		cfg.SecondRenderScale = 1
	}
	if cfg.SecondRenderScale > 4 {
		cfg.SecondRenderScale = 4
	}
	if cfg.StabilityWait < time.Second {
		cfg.StabilityWait = time.Second
	}
	if cfg.StabilityRuns < 2 {
		cfg.StabilityRuns = 2
	}
	return cfg, nil
}

func newSheetsService(ctx context.Context, cfg workflowConfig) (*sheets.Service, error) {
	options := []option.ClientOption{
		option.WithScopes(sheets.SpreadsheetsReadonlyScope),
	}
	if cfg.GoogleCredentialsJSON != "" {
		options = append(options, option.WithCredentialsJSON([]byte(cfg.GoogleCredentialsJSON)))
	} else {
		options = append(options, option.WithCredentialsFile(cfg.GoogleCredentialsFile))
	}
	svc, err := sheets.NewService(ctx, options...)
	if err != nil {
		return nil, err
	}
	return svc, nil
}

func readSingleCell(ctx context.Context, svc *sheets.Service, sheetID, tab, cell string) (string, error) {
	rangeRef := fmt.Sprintf("%s!%s", tab, cell)
	resp, err := svc.Spreadsheets.Values.Get(sheetID, rangeRef).Context(ctx).Do()
	if err != nil {
		return "", fmt.Errorf("read trigger cell %s: %w", rangeRef, err)
	}
	if len(resp.Values) == 0 || len(resp.Values[0]) == 0 {
		return "", nil
	}
	return strings.TrimSpace(fmt.Sprint(resp.Values[0][0])), nil
}

func buildCaptureTargets(cfg workflowConfig) []captureTarget {
	targets := []captureTarget{
		{Tab: cfg.SheetTab, Range: cfg.CaptureRange, RenderScale: cfg.RenderScale},
	}
	secondTab := strings.TrimSpace(cfg.SecondCaptureTab)
	secondRange := strings.TrimSpace(cfg.SecondCaptureRange)
	if secondTab != "" && secondRange != "" {
		secondScale := cfg.SecondRenderScale
		if secondScale < 1 {
			secondScale = cfg.RenderScale
		}
		targets = append(targets, captureTarget{Tab: secondTab, Range: secondRange, RenderScale: secondScale})
	}
	return targets
}

func waitForStableSnapshots(
	ctx context.Context,
	cfg workflowConfig,
	svc *sheets.Service,
	expectedTrigger string,
	targets []captureTarget,
) (stable bool, latestTrigger string, err error) {
	var previousDigest string
	latestTrigger = expectedTrigger

	for run := 1; run <= cfg.StabilityRuns; run++ {
		currentTrigger, readErr := readSingleCell(ctx, svc, cfg.SheetID, cfg.SheetTab, cfg.TriggerCell)
		if readErr != nil {
			return false, latestTrigger, readErr
		}
		currentTrigger = strings.TrimSpace(currentTrigger)
		latestTrigger = currentTrigger
		if currentTrigger != expectedTrigger {
			return false, latestTrigger, nil
		}

		digest, digestErr := captureValuesDigest(ctx, svc, cfg.SheetID, targets)
		if digestErr != nil {
			return false, latestTrigger, digestErr
		}
		if run > 1 && digest == previousDigest {
			return true, latestTrigger, nil
		}
		previousDigest = digest

		if run < cfg.StabilityRuns {
			select {
			case <-ctx.Done():
				return false, latestTrigger, ctx.Err()
			case <-time.After(cfg.StabilityWait):
			}
		}
	}

	return false, latestTrigger, nil
}

func captureValuesDigest(ctx context.Context, svc *sheets.Service, sheetID string, targets []captureTarget) (string, error) {
	ranges := make([]string, 0, len(targets))
	for _, target := range targets {
		ranges = append(ranges, fmt.Sprintf("%s!%s", target.Tab, target.Range))
	}

	resp, err := svc.Spreadsheets.Values.BatchGet(sheetID).
		Ranges(ranges...).
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

func readStyledRange(ctx context.Context, svc *sheets.Service, sheetID, tab, captureRange string) (styledRangeData, error) {
	parsed, err := parseA1Range(captureRange)
	if err != nil {
		return styledRangeData{}, err
	}

	rangeRef := fmt.Sprintf("%s!%s", tab, captureRange)
	resp, err := svc.Spreadsheets.Get(sheetID).
		Ranges(rangeRef).
		IncludeGridData(true).
		Fields(
			"sheets(properties(title),merges,data(startRow,startColumn,rowMetadata(pixelSize),columnMetadata(pixelSize),rowData(values(formattedValue,effectiveValue(stringValue,numberValue,boolValue),effectiveFormat(backgroundColor,textFormat(foregroundColor,bold,italic,underline,strikethrough,fontSize,fontFamily),horizontalAlignment,verticalAlignment,wrapStrategy,borders(top(style,color),bottom(style,color),left(style,color),right(style,color)))))))",
		).
		Context(ctx).
		Do()
	if err != nil {
		return styledRangeData{}, fmt.Errorf("read range %s: %w", rangeRef, err)
	}

	rowCount := parsed.endRow - parsed.startRow + 1
	colCount := parsed.endCol - parsed.startCol + 1

	result := styledRangeData{
		Rows:       rowCount,
		Cols:       colCount,
		RowHeights: make([]int, rowCount),
		ColWidths:  make([]int, colCount),
		Cells:      make([][]styledCell, rowCount),
	}

	for r := 0; r < rowCount; r++ {
		result.RowHeights[r] = 24
		result.Cells[r] = make([]styledCell, colCount)
		for c := 0; c < colCount; c++ {
			result.Cells[r][c] = defaultStyledCell()
		}
	}
	for c := 0; c < colCount; c++ {
		result.ColWidths[c] = 100
	}

	var targetSheet *sheets.Sheet
	for _, sh := range resp.Sheets {
		if sh == nil || sh.Properties == nil {
			continue
		}
		if sh.Properties.Title == tab {
			targetSheet = sh
			break
		}
	}
	if targetSheet == nil {
		return styledRangeData{}, fmt.Errorf("sheet tab %q not found in response", tab)
	}
	if len(targetSheet.Data) == 0 || targetSheet.Data[0] == nil {
		return result, nil
	}
	grid := targetSheet.Data[0]

	for r := 0; r < rowCount && r < len(grid.RowMetadata); r++ {
		if grid.RowMetadata[r] != nil && grid.RowMetadata[r].PixelSize > 0 {
			result.RowHeights[r] = int(grid.RowMetadata[r].PixelSize)
		}
	}
	for c := 0; c < colCount && c < len(grid.ColumnMetadata); c++ {
		if grid.ColumnMetadata[c] != nil && grid.ColumnMetadata[c].PixelSize > 0 {
			result.ColWidths[c] = int(grid.ColumnMetadata[c].PixelSize)
		}
	}

	for r := 0; r < rowCount && r < len(grid.RowData); r++ {
		if grid.RowData[r] == nil {
			continue
		}
		rowVals := grid.RowData[r].Values
		for c := 0; c < colCount && c < len(rowVals); c++ {
			cellData := rowVals[c]
			if cellData == nil {
				continue
			}

			cell := defaultStyledCell()
			cell.Text = strings.TrimSpace(cellData.FormattedValue)
			if cell.Text == "" && cellData.EffectiveValue != nil {
				cell.Text = effectiveValueToString(cellData.EffectiveValue)
			}

			if cellData.EffectiveFormat != nil {
				eff := cellData.EffectiveFormat
				cell.Background = toRGBA(eff.BackgroundColor, cell.Background)
				cell.HAlign = firstNonEmpty(strings.TrimSpace(eff.HorizontalAlignment), cell.HAlign)
				cell.VAlign = firstNonEmpty(strings.TrimSpace(eff.VerticalAlignment), cell.VAlign)
				cell.WrapStrategy = firstNonEmpty(strings.TrimSpace(eff.WrapStrategy), cell.WrapStrategy)
				cell.Borders = parseStyledBorders(eff.Borders)

				if eff.TextFormat != nil {
					cell.Foreground = toRGBA(eff.TextFormat.ForegroundColor, cell.Foreground)
					cell.FontFamily = strings.TrimSpace(eff.TextFormat.FontFamily)
					cell.Bold = eff.TextFormat.Bold
					cell.Italic = eff.TextFormat.Italic
					cell.Underline = eff.TextFormat.Underline
					cell.Strikethru = eff.TextFormat.Strikethrough
					if eff.TextFormat.FontSize > 0 {
						cell.FontSize = float64(eff.TextFormat.FontSize)
					}
				}
			}
			result.Cells[r][c] = cell
		}
	}

	captureStartRow := int64(parsed.startRow - 1)
	captureEndRow := int64(parsed.endRow)
	captureStartCol := int64(parsed.startCol - 1)
	captureEndCol := int64(parsed.endCol)

	for _, merged := range targetSheet.Merges {
		if merged == nil {
			continue
		}
		sr := maxInt64(merged.StartRowIndex, captureStartRow)
		er := minInt64(merged.EndRowIndex, captureEndRow)
		sc := maxInt64(merged.StartColumnIndex, captureStartCol)
		ec := minInt64(merged.EndColumnIndex, captureEndCol)
		if sr >= er || sc >= ec {
			continue
		}
		result.Merges = append(result.Merges, mergeRegion{
			StartRow: int(sr - captureStartRow),
			EndRow:   int(er - captureStartRow),
			StartCol: int(sc - captureStartCol),
			EndCol:   int(ec - captureStartCol),
		})
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

func renderStyledRangeImage(data styledRangeData, maxWidth int, renderScale int) ([]byte, error) {
	if data.Rows <= 0 || data.Cols <= 0 {
		return nil, errors.New("empty styled range")
	}
	if renderScale < 1 {
		renderScale = 1
	}

	colWidths := make([]int, data.Cols)
	rowHeights := make([]int, data.Rows)
	for c := 0; c < data.Cols; c++ {
		width := 100
		if c < len(data.ColWidths) && data.ColWidths[c] > 0 {
			width = data.ColWidths[c]
		}
		colWidths[c] = maxInt(width*renderScale, 24)
	}
	for r := 0; r < data.Rows; r++ {
		height := 24
		if r < len(data.RowHeights) && data.RowHeights[r] > 0 {
			height = data.RowHeights[r]
		}
		rowHeights[r] = maxInt(height*renderScale, 16)
	}

	totalWidth := 1
	for _, w := range colWidths {
		totalWidth += w
	}
	totalHeight := 1
	for _, h := range rowHeights {
		totalHeight += h
	}

	layoutScale := 1.0
	if maxWidth > 0 && totalWidth > maxWidth {
		fitRatio := float64(maxWidth-1) / float64(totalWidth-1)
		if fitRatio < 0.35 {
			fitRatio = 0.35
		}
		layoutScale = fitRatio
		totalWidth = 1
		totalHeight = 1
		for idx, w := range colWidths {
			scaled := int(math.Round(float64(w) * fitRatio))
			colWidths[idx] = maxInt(scaled, 18)
			totalWidth += colWidths[idx]
		}
		for idx, h := range rowHeights {
			scaled := int(math.Round(float64(h) * fitRatio))
			rowHeights[idx] = maxInt(scaled, 14)
			totalHeight += rowHeights[idx]
		}
	}

	xOffsets := make([]int, data.Cols+1)
	yOffsets := make([]int, data.Rows+1)
	for c := 0; c < data.Cols; c++ {
		xOffsets[c+1] = xOffsets[c] + colWidths[c]
	}
	for r := 0; r < data.Rows; r++ {
		yOffsets[r+1] = yOffsets[r] + rowHeights[r]
	}

	canvas := image.NewRGBA(image.Rect(0, 0, totalWidth, totalHeight))
	draw.Draw(canvas, canvas.Bounds(), &image.Uniform{C: color.RGBA{255, 255, 255, 255}}, image.Point{}, draw.Src)
	borderScale := maxInt(int(math.Round(float64(renderScale)*layoutScale)), 1)

	mergeMap := make([][]int, data.Rows)
	for r := 0; r < data.Rows; r++ {
		mergeMap[r] = make([]int, data.Cols)
		for c := 0; c < data.Cols; c++ {
			mergeMap[r][c] = -1
		}
	}
	for idx, merge := range data.Merges {
		for r := maxInt(merge.StartRow, 0); r < minInt(merge.EndRow, data.Rows); r++ {
			for c := maxInt(merge.StartCol, 0); c < minInt(merge.EndCol, data.Cols); c++ {
				mergeMap[r][c] = idx
			}
		}
	}

	drawnMergeBG := map[int]bool{}
	for r := 0; r < data.Rows; r++ {
		for c := 0; c < data.Cols; c++ {
			mergeIdx := mergeMap[r][c]
			if mergeIdx >= 0 {
				if drawnMergeBG[mergeIdx] {
					continue
				}
				merge := data.Merges[mergeIdx]
				topRow := maxInt(merge.StartRow, 0)
				leftCol := maxInt(merge.StartCol, 0)
				bottomRow := minInt(merge.EndRow, data.Rows)
				rightCol := minInt(merge.EndCol, data.Cols)
				bg := data.Cells[topRow][leftCol].Background
				rect := image.Rect(
					xOffsets[leftCol],
					yOffsets[topRow],
					xOffsets[rightCol],
					yOffsets[bottomRow],
				)
				fillRect(canvas, rect, bg)
				drawnMergeBG[mergeIdx] = true
				continue
			}
			rect := image.Rect(
				xOffsets[c],
				yOffsets[r],
				xOffsets[c+1],
				yOffsets[r+1],
			)
			fillRect(canvas, rect, data.Cells[r][c].Background)
		}
	}

	for r := 0; r < data.Rows; r++ {
		for c := 0; c < data.Cols; c++ {
			mergeIdx := mergeMap[r][c]
			if mergeIdx >= 0 {
				merge := data.Merges[mergeIdx]
				if r != merge.StartRow || c != merge.StartCol {
					continue
				}
				b := data.Cells[r][c].Borders
				rect := image.Rect(
					xOffsets[merge.StartCol],
					yOffsets[merge.StartRow],
					xOffsets[minInt(merge.EndCol, data.Cols)],
					yOffsets[minInt(merge.EndRow, data.Rows)],
				)
				drawCellBorders(canvas, rect, b, borderScale)
				continue
			}

			b := data.Cells[r][c].Borders
			rect := image.Rect(
				xOffsets[c],
				yOffsets[r],
				xOffsets[c+1],
				yOffsets[r+1],
			)
			drawCellBorders(canvas, rect, b, borderScale)
		}
	}

	for r := 0; r < data.Rows; r++ {
		for c := 0; c < data.Cols; c++ {
			mergeIdx := mergeMap[r][c]
			if mergeIdx >= 0 {
				merge := data.Merges[mergeIdx]
				if r != merge.StartRow || c != merge.StartCol {
					continue
				}
				rect := image.Rect(
					xOffsets[merge.StartCol],
					yOffsets[merge.StartRow],
					xOffsets[minInt(merge.EndCol, data.Cols)],
					yOffsets[minInt(merge.EndRow, data.Rows)],
				)
				drawCellText(canvas, rect, data.Cells[r][c], renderScale, layoutScale)
				continue
			}
			rect := image.Rect(
				xOffsets[c],
				yOffsets[r],
				xOffsets[c+1],
				yOffsets[r+1],
			)
			drawCellText(canvas, rect, data.Cells[r][c], renderScale, layoutScale)
		}
	}

	var buf bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestCompression}
	if err := enc.Encode(&buf, canvas); err != nil {
		return nil, fmt.Errorf("encode png: %w", err)
	}
	return buf.Bytes(), nil
}

func drawCellText(dst draw.Image, rect image.Rectangle, cell styledCell, renderScale int, layoutScale float64) {
	text := strings.TrimSpace(cell.Text)
	if text == "" {
		return
	}

	fontSize := cell.FontSize
	if fontSize <= 0 {
		fontSize = 10
	}
	textScale := float64(renderScale) * layoutScale
	if textScale <= 0 {
		textScale = float64(renderScale)
	}
	face := loadFace(cell.FontFamily, cell.Bold, cell.Italic, fontSize*textScale)
	if face == nil {
		face = basicfont.Face7x13
	}

	paddingX := maxInt(int(math.Round(4*textScale)), 4)
	paddingY := maxInt(int(math.Round(3*textScale)), 3)
	decorationThickness := maxInt(int(math.Round(textScale)), 1)
	maxTextWidth := rect.Dx() - (paddingX * 2)
	if maxTextWidth <= 4 {
		return
	}

	lines := []string{text}
	if strings.EqualFold(cell.WrapStrategy, "WRAP") {
		lines = wrapTextToWidth(text, face, maxTextWidth)
	} else {
		lines = []string{ellipsizeToWidth(text, face, maxTextWidth)}
	}
	if len(lines) == 0 {
		return
	}

	ascent := face.Metrics().Ascent.Ceil()
	lineHeight := face.Metrics().Height.Ceil()
	if lineHeight <= 0 {
		lineHeight = ascent + face.Metrics().Descent.Ceil()
	}
	if lineHeight <= 0 {
		lineHeight = 12
	}
	textBlockHeight := lineHeight * len(lines)

	startY := rect.Min.Y + paddingY + ascent
	switch strings.ToUpper(strings.TrimSpace(cell.VAlign)) {
	case "TOP":
		startY = rect.Min.Y + paddingY + ascent
	case "BOTTOM":
		startY = rect.Max.Y - paddingY - textBlockHeight + ascent
	default:
		startY = rect.Min.Y + (rect.Dy()-textBlockHeight)/2 + ascent
	}

	for idx, line := range lines {
		textWidth := font.MeasureString(face, line).Ceil()
		x := rect.Min.X + paddingX
		switch strings.ToUpper(strings.TrimSpace(cell.HAlign)) {
		case "CENTER":
			x = rect.Min.X + (rect.Dx()-textWidth)/2
		case "RIGHT":
			x = rect.Max.X - paddingX - textWidth
		default:
			x = rect.Min.X + paddingX
		}
		if x < rect.Min.X+2 {
			x = rect.Min.X + 2
		}
		y := startY + idx*lineHeight
		if y > rect.Max.Y-paddingY {
			break
		}

		d := &font.Drawer{
			Dst:  dst,
			Src:  image.NewUniform(cell.Foreground),
			Face: face,
			Dot:  fixed.P(x, y),
		}
		d.DrawString(line)

		if cell.Underline {
			underlineY := minInt(y+decorationThickness, rect.Max.Y-1)
			fillRect(dst, image.Rect(x, underlineY, minInt(x+textWidth, rect.Max.X-1), minInt(underlineY+decorationThickness, rect.Max.Y)), cell.Foreground)
		}
		if cell.Strikethru {
			strikeY := y - ascent/2
			if strikeY < rect.Min.Y {
				strikeY = rect.Min.Y
			}
			fillRect(dst, image.Rect(x, strikeY, minInt(x+textWidth, rect.Max.X-1), minInt(strikeY+decorationThickness, rect.Max.Y)), cell.Foreground)
		}
	}
}

func drawCellBorders(dst draw.Image, rect image.Rectangle, borders styledBorders, renderScale int) {
	drawBorderSide(dst, rect, "TOP", borders.Top, renderScale)
	drawBorderSide(dst, rect, "BOTTOM", borders.Bottom, renderScale)
	drawBorderSide(dst, rect, "LEFT", borders.Left, renderScale)
	drawBorderSide(dst, rect, "RIGHT", borders.Right, renderScale)
}

func drawBorderSide(dst draw.Image, rect image.Rectangle, side string, border styledBorder, renderScale int) {
	thickness := borderThickness(border.Style, renderScale)
	if thickness <= 0 {
		return
	}
	if border.Color.A == 0 {
		border.Color = color.RGBA{194, 203, 214, 255}
	}

	switch side {
	case "TOP":
		fillRect(dst, image.Rect(rect.Min.X, rect.Min.Y, rect.Max.X, minInt(rect.Min.Y+thickness, rect.Max.Y)), border.Color)
	case "BOTTOM":
		fillRect(dst, image.Rect(rect.Min.X, maxInt(rect.Max.Y-thickness, rect.Min.Y), rect.Max.X, rect.Max.Y), border.Color)
	case "LEFT":
		fillRect(dst, image.Rect(rect.Min.X, rect.Min.Y, minInt(rect.Min.X+thickness, rect.Max.X), rect.Max.Y), border.Color)
	case "RIGHT":
		fillRect(dst, image.Rect(maxInt(rect.Max.X-thickness, rect.Min.X), rect.Min.Y, rect.Max.X, rect.Max.Y), border.Color)
	}
}

func borderThickness(style string, renderScale int) int {
	base := 0
	switch strings.ToUpper(strings.TrimSpace(style)) {
	case "", "NONE":
		base = 0
	case "SOLID":
		base = 1
	case "DOTTED", "DASHED":
		base = 1
	case "SOLID_MEDIUM":
		base = 2
	case "SOLID_THICK", "DOUBLE":
		base = 3
	default:
		base = 1
	}
	return base * maxInt(renderScale, 1)
}

func fillRect(dst draw.Image, rect image.Rectangle, c color.RGBA) {
	if rect.Empty() {
		return
	}
	draw.Draw(dst, rect, &image.Uniform{C: c}, image.Point{}, draw.Src)
}

func wrapTextToWidth(text string, face font.Face, maxWidth int) []string {
	words := strings.Fields(text)
	if len(words) == 0 {
		return nil
	}

	lines := make([]string, 0, 2)
	current := words[0]
	for i := 1; i < len(words); i++ {
		next := current + " " + words[i]
		if font.MeasureString(face, next).Ceil() <= maxWidth {
			current = next
			continue
		}
		lines = append(lines, current)
		current = words[i]
	}
	lines = append(lines, current)
	return lines
}

func ellipsizeToWidth(text string, face font.Face, maxWidthPx int) string {
	if maxWidthPx <= 0 {
		return ""
	}
	if font.MeasureString(face, text).Ceil() <= maxWidthPx {
		return text
	}
	runes := []rune(text)
	if len(runes) == 0 {
		return ""
	}
	ellipsis := "..."
	maxWidthPx -= font.MeasureString(face, ellipsis).Ceil()
	if maxWidthPx <= 0 {
		return ellipsis
	}
	cut := len(runes)
	for cut > 0 {
		candidate := string(runes[:cut])
		if font.MeasureString(face, candidate).Ceil() <= maxWidthPx {
			return candidate + ellipsis
		}
		cut--
	}
	return ellipsis
}

func defaultStyledCell() styledCell {
	return styledCell{
		Text:         "",
		Background:   color.RGBA{255, 255, 255, 255},
		Foreground:   color.RGBA{20, 28, 39, 255},
		FontFamily:   "",
		Bold:         false,
		Italic:       false,
		Underline:    false,
		Strikethru:   false,
		FontSize:     10,
		HAlign:       "LEFT",
		VAlign:       "MIDDLE",
		WrapStrategy: "CLIP",
		Borders:      styledBorders{},
	}
}

func effectiveValueToString(v *sheets.ExtendedValue) string {
	if v == nil {
		return ""
	}
	if v.StringValue != nil {
		return strings.TrimSpace(*v.StringValue)
	}
	if v.NumberValue != nil {
		return strings.TrimSpace(strconv.FormatFloat(*v.NumberValue, 'f', -1, 64))
	}
	if v.BoolValue != nil {
		if *v.BoolValue {
			return "TRUE"
		}
		return "FALSE"
	}
	return ""
}

func parseStyledBorders(b *sheets.Borders) styledBorders {
	if b == nil {
		return styledBorders{}
	}
	return styledBorders{
		Top:    parseStyledBorder(b.Top),
		Bottom: parseStyledBorder(b.Bottom),
		Left:   parseStyledBorder(b.Left),
		Right:  parseStyledBorder(b.Right),
	}
}

func parseStyledBorder(b *sheets.Border) styledBorder {
	if b == nil {
		return styledBorder{}
	}
	return styledBorder{
		Style: b.Style,
		Color: toRGBA(b.Color, color.RGBA{194, 203, 214, 255}),
	}
}

func toRGBA(c *sheets.Color, fallback color.RGBA) color.RGBA {
	if c == nil {
		return fallback
	}
	a := c.Alpha
	if a <= 0 {
		a = 1
	}
	return color.RGBA{
		R: toByte(c.Red),
		G: toByte(c.Green),
		B: toByte(c.Blue),
		A: toByte(a),
	}
}

func toByte(v float64) uint8 {
	if v < 0 {
		v = 0
	}
	if v > 1 {
		v = 1
	}
	return uint8(math.Round(v * 255))
}

func loadFace(fontFamily string, isBold bool, isItalic bool, size float64) font.Face {
	fontInitOnce.Do(func() {
		regularSans, fontInitErr = opentype.Parse(liberationsansregular.TTF)
		if fontInitErr != nil {
			return
		}
		boldSans, fontInitErr = opentype.Parse(liberationsansbold.TTF)
		if fontInitErr != nil {
			return
		}
		italicSans, fontInitErr = opentype.Parse(liberationsansitalic.TTF)
		if fontInitErr != nil {
			return
		}
		boldItSans, fontInitErr = opentype.Parse(liberationsansbolditalic.TTF)
		if fontInitErr != nil {
			return
		}

		regularSerif, fontInitErr = opentype.Parse(liberationserifregular.TTF)
		if fontInitErr != nil {
			return
		}
		boldSerif, fontInitErr = opentype.Parse(liberationserifbold.TTF)
		if fontInitErr != nil {
			return
		}
		italicSerif, fontInitErr = opentype.Parse(liberationserifitalic.TTF)
		if fontInitErr != nil {
			return
		}
		boldItSerif, fontInitErr = opentype.Parse(liberationserifbolditalic.TTF)
		if fontInitErr != nil {
			return
		}

		regularMono, fontInitErr = opentype.Parse(liberationmonoregular.TTF)
		if fontInitErr != nil {
			return
		}
		boldMono, fontInitErr = opentype.Parse(liberationmonobold.TTF)
		if fontInitErr != nil {
			return
		}
		italicMono, fontInitErr = opentype.Parse(liberationmonoitalic.TTF)
		if fontInitErr != nil {
			return
		}
		boldItMono, fontInitErr = opentype.Parse(liberationmonobolditalic.TTF)
	})
	if fontInitErr != nil {
		return basicfont.Face7x13
	}
	if size < 8 {
		size = 8
	}
	if size > 42 {
		size = 42
	}

	family := normalizeFontFamily(fontFamily)
	key := fmt.Sprintf("%s:%t:%t:%.1f", family, isBold, isItalic, size)
	fontFaceMu.Lock()
	defer fontFaceMu.Unlock()
	if cached, ok := fontFaceMap[key]; ok {
		return cached
	}

	selected := regularSans
	if family == "mono" {
		switch {
		case isBold && isItalic:
			selected = boldItMono
		case isBold:
			selected = boldMono
		case isItalic:
			selected = italicMono
		default:
			selected = regularMono
		}
	} else if family == "serif" {
		switch {
		case isBold && isItalic:
			selected = boldItSerif
		case isBold:
			selected = boldSerif
		case isItalic:
			selected = italicSerif
		default:
			selected = regularSerif
		}
	} else {
		switch {
		case isBold && isItalic:
			selected = boldItSans
		case isBold:
			selected = boldSans
		case isItalic:
			selected = italicSans
		default:
			selected = regularSans
		}
	}
	face, err := opentype.NewFace(selected, &opentype.FaceOptions{
		Size:    size,
		DPI:     96,
		Hinting: font.HintingFull,
	})
	if err != nil {
		return basicfont.Face7x13
	}
	fontFaceMap[key] = face
	return face
}

func normalizeFontFamily(raw string) string {
	normalized := strings.ToLower(strings.TrimSpace(raw))
	switch {
	case strings.Contains(normalized, "mono"),
		strings.Contains(normalized, "courier"),
		strings.Contains(normalized, "consolas"):
		return "mono"
	case strings.Contains(normalized, "serif"),
		strings.Contains(normalized, "times"),
		strings.Contains(normalized, "cambria"),
		strings.Contains(normalized, "georgia"),
		strings.Contains(normalized, "garamond"):
		return "serif"
	default:
		return "sans"
	}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func minInt64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
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

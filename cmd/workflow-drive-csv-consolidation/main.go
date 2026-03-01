package main

import (
	"archive/zip"
	"bufio"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awscfg "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/joho/godotenv"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"
)

const (
	workflowName = "workflow_2_1_drive_csv_consolidation"

	defaultDriveParentFolderID = "1oU9kj5VIJIoNrR388wYCHSdtHGanRrgZ"
	defaultDestinationSheetID  = "1mdi-8ACluDHGZ7yAyNLwXLwpmQ4f6VAx3kpbaJORViA"
	defaultDestinationTab      = "generated_file"

	defaultStateFile  = "data/workflow2-1-drive-csv-consolidation-state.json"
	defaultStatusFile = "data/workflow2-1-drive-csv-consolidation-status.json"

	defaultPollInterval  = 30 * time.Second
	defaultSheetsBatch   = 5000
	defaultR2ObjectPrefx = "wf2-1"

	defaultSummarySheetTab        = "[SOC] Backlogs Summary"
	defaultSummaryRange           = "B2:Q59"
	defaultSummaryWaitAfterImport = 8 * time.Second
	defaultSummaryStabilityWait   = 2 * time.Second
	defaultSummaryStabilityRuns   = 3
	defaultSummaryRenderScale     = 2
	defaultSummaryImageMaxWidthPx = 3000
	defaultSummaryImageMaxBase64  = 5 * 1024 * 1024
	defaultSummaryHTTPTimeout     = 10 * time.Second
	defaultSummaryTimezone        = "Asia/Manila"
)

var selectedOutputHeaders = []string{
	"TO Number",
	"SPX Tracking Number",
	"Receiver Name",
	"TO Order Quantity",
	"TO Number",
	"Operator",
	"Create Time",
	"Complete Time",
	"Remark",
	"Receive Status",
	"Staging Area ID",
}

type workflowConfig struct {
	GoogleCredentialsFile string
	GoogleCredentialsJSON string

	DriveParentFolderID string

	DestinationSheetID string
	DestinationTab     string

	R2AccountID       string
	R2Bucket          string
	R2AccessKeyID     string
	R2SecretAccessKey string
	R2ObjectPrefix    string

	Continuous               bool
	PollInterval             time.Duration
	DryRun                   bool
	BootstrapProcessExisting bool
	DropLeadingUnnamedColumn bool
	SheetsBatchSize          int
	TempDir                  string

	StateFile  string
	StatusFile string

	EnableHealthServer bool
	HealthListenAddr   string

	SummarySendEnabled     bool
	SummarySeaTalkMode     string
	SummaryWebhookURL      string
	SummarySeaTalkAppID    string
	SummarySeaTalkSecret   string
	SummarySeaTalkBaseURL  string
	SummarySeaTalkGroupID  string
	SummarySheetID         string
	SummaryTab             string
	SummaryRange           string
	SummaryWaitAfterImport time.Duration
	SummaryStabilityWait   time.Duration
	SummaryStabilityRuns   int
	SummaryRenderScale     int
	SummaryImageMaxWidthPx int
	SummaryImageMaxBase64  int
	SummarySendHTTPTimeout time.Duration
	SummaryTimezone        string
	SummaryLocation        *time.Location
}

type workflowState struct {
	LastProcessedFileID       string `json:"last_processed_file_id,omitempty"`
	LastProcessedFileMD5      string `json:"last_processed_file_md5,omitempty"`
	LastProcessedModifiedTime string `json:"last_processed_modified_time,omitempty"`
	LastProcessedAt           string `json:"last_processed_at,omitempty"`
	LastUploadedObjectKey     string `json:"last_uploaded_object_key,omitempty"`
}

type workflowStatus struct {
	Workflow     string `json:"workflow"`
	Continuous   bool   `json:"continuous"`
	DryRun       bool   `json:"dry_run"`
	Cycle        int    `json:"cycle"`
	LastCycleAt  string `json:"last_cycle_at"`
	FoundZip     bool   `json:"found_zip"`
	Changed      bool   `json:"changed"`
	FileID       string `json:"file_id,omitempty"`
	FileName     string `json:"file_name,omitempty"`
	FileModified string `json:"file_modified,omitempty"`
	FileMD5      string `json:"file_md5,omitempty"`

	CSVFilesProcessed int    `json:"csv_files_processed,omitempty"`
	RowsConsolidated  int64  `json:"rows_consolidated,omitempty"`
	RowsImported      int64  `json:"rows_imported,omitempty"`
	ObjectKey         string `json:"object_key,omitempty"`
	ObjectBytes       int64  `json:"object_bytes,omitempty"`
	SummaryImageSent  bool   `json:"summary_image_sent,omitempty"`
	SummaryStable     bool   `json:"summary_stable,omitempty"`
	SummaryImageFmt   string `json:"summary_image_format,omitempty"`
	SummaryImageBytes int    `json:"summary_image_bytes,omitempty"`

	StateFile  string `json:"state_file"`
	StatusFile string `json:"status_file,omitempty"`
	Message    string `json:"message,omitempty"`
}

type driveZipFile struct {
	ID           string
	Name         string
	MD5Checksum  string
	ModifiedTime time.Time
	Size         int64
}

type processResult struct {
	CSVFilesProcessed int
	RowsConsolidated  int64
	RowsImported      int64
	ObjectKey         string
	ObjectBytes       int64
	SummaryImageSent  bool
	SummaryStable     bool
	SummaryImageFmt   string
	SummaryImageBytes int
}

type sheetGridState struct {
	sheetID int64
	rows    int
	cols    int
	loaded  bool
}

func main() {
	logger := log.New(os.Stdout, "[workflow-drive-csv-consolidation] ", log.LstdFlags|log.Lmsgprefix)

	cfg, err := loadConfig()
	if err != nil {
		logger.Fatalf("config error: %v", err)
	}

	state, stateExists, err := loadState(cfg.StateFile)
	if err != nil {
		logger.Fatalf("state load error: %v", err)
	}

	driveSvc, sheetsSvc, err := newGoogleServices(context.Background(), cfg)
	if err != nil {
		logger.Fatalf("google init error: %v", err)
	}

	r2Client, err := newR2Client(context.Background(), cfg)
	if err != nil {
		logger.Fatalf("r2 init error: %v", err)
	}

	if !cfg.Continuous {
		if err = runCycle(context.Background(), cfg, driveSvc, sheetsSvc, r2Client, &state, &stateExists, logger, 1); err != nil {
			logger.Fatalf("workflow failed: %v", err)
		}
		return
	}

	sigCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if cfg.EnableHealthServer {
		startHealthServer(sigCtx, cfg, logger)
	}

	outputEndCol := columnNameFromIndex(len(selectedOutputHeaders))
	logger.Printf(
		"destination write target sheet=%s tab=%s range=A:%s columns=%d headers=%q",
		cfg.DestinationSheetID,
		cfg.DestinationTab,
		outputEndCol,
		len(selectedOutputHeaders),
		strings.Join(selectedOutputHeaders, ", "),
	)
	if cfg.SummarySendEnabled {
		logger.Printf(
			"summary snapshot enabled mode=%s sheet=%s tab=%q range=%s wait_after_import=%s stability_runs=%d stability_wait=%s timezone=%s",
			cfg.SummarySeaTalkMode,
			cfg.SummarySheetID,
			cfg.SummaryTab,
			cfg.SummaryRange,
			cfg.SummaryWaitAfterImport,
			cfg.SummaryStabilityRuns,
			cfg.SummaryStabilityWait,
			cfg.SummaryTimezone,
		)
	}

	cycle := 1
	logger.Printf("watch mode enabled poll_interval=%s", cfg.PollInterval)
	for {
		if err = runCycle(sigCtx, cfg, driveSvc, sheetsSvc, r2Client, &state, &stateExists, logger, cycle); err != nil {
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
	driveSvc *drive.Service,
	sheetsSvc *sheets.Service,
	r2Client *s3.Client,
	state *workflowState,
	stateExists *bool,
	logger *log.Logger,
	cycle int,
) error {
	now := time.Now().UTC()
	status := workflowStatus{
		Workflow:    workflowName,
		Continuous:  cfg.Continuous,
		DryRun:      cfg.DryRun,
		Cycle:       cycle,
		LastCycleAt: now.Format(time.RFC3339),
		StateFile:   cfg.StateFile,
		StatusFile:  cfg.StatusFile,
	}

	files, err := listZipFiles(ctx, driveSvc, cfg.DriveParentFolderID)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		status.FoundZip = false
		status.Message = "no zip files found in parent folder"
		writeStatusIfConfigured(cfg.StatusFile, status, logger)
		logger.Printf("no zip files found parent_folder=%s", cfg.DriveParentFolderID)
		return nil
	}
	latest := files[len(files)-1]

	status.FoundZip = true
	status.FileID = latest.ID
	status.FileName = latest.Name
	status.FileModified = latest.ModifiedTime.Format(time.RFC3339)
	status.FileMD5 = latest.MD5Checksum

	if !*stateExists && !cfg.BootstrapProcessExisting {
		state.LastProcessedFileID = latest.ID
		state.LastProcessedFileMD5 = latest.MD5Checksum
		state.LastProcessedModifiedTime = latest.ModifiedTime.Format(time.RFC3339)
		state.LastProcessedAt = now.Format(time.RFC3339)
		if err = saveState(cfg.StateFile, *state); err != nil {
			return err
		}
		*stateExists = true
		status.Changed = false
		status.Message = "baseline set from latest zip (bootstrap disabled)"
		writeStatusIfConfigured(cfg.StatusFile, status, logger)
		logger.Printf("baseline set file_id=%s file_name=%q", latest.ID, latest.Name)
		return nil
	}

	var pending []driveZipFile
	if !*stateExists {
		pending = []driveZipFile{latest}
	} else {
		pending = selectPendingZipFiles(files, *state)
	}
	status.Changed = len(pending) > 0

	if len(pending) == 0 {
		status.Message = "no new zip files to process"
		writeStatusIfConfigured(cfg.StatusFile, status, logger)
		logger.Printf("already processed latest file_id=%s file_name=%q", latest.ID, latest.Name)
		return nil
	}

	var (
		totalResult        processResult
		lastProcessedFile  driveZipFile
		lastProcessedCycle processResult
	)
	for i, file := range pending {
		zipPath, downloadErr := downloadDriveFileToTemp(ctx, driveSvc, file.ID, cfg.TempDir)
		if downloadErr != nil {
			return fmt.Errorf("download zip %s: %w", file.ID, downloadErr)
		}

		sendSummaryAfterImport := i == len(pending)-1
		result, processErr := processZipAndImport(ctx, cfg, sheetsSvc, r2Client, file, zipPath, sendSummaryAfterImport)
		removeErr := os.Remove(zipPath)
		if processErr != nil {
			return processErr
		}
		if removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			return fmt.Errorf("cleanup temp zip %s: %w", zipPath, removeErr)
		}

		totalResult.CSVFilesProcessed += result.CSVFilesProcessed
		totalResult.RowsConsolidated += result.RowsConsolidated
		totalResult.RowsImported += result.RowsImported
		totalResult.ObjectBytes += result.ObjectBytes

		lastProcessedFile = file
		lastProcessedCycle = result
		totalResult.ObjectKey = result.ObjectKey

		state.LastProcessedFileID = file.ID
		state.LastProcessedFileMD5 = file.MD5Checksum
		state.LastProcessedModifiedTime = file.ModifiedTime.Format(time.RFC3339)
		state.LastProcessedAt = now.Format(time.RFC3339)
		state.LastUploadedObjectKey = result.ObjectKey
		if err = saveState(cfg.StateFile, *state); err != nil {
			return err
		}
		*stateExists = true

		logger.Printf(
			"processed file_id=%s file_name=%q csv_files=%d rows_consolidated=%d rows_imported=%d object_key=%q bytes=%d summary_image_sent=%t summary_stable=%t summary_fmt=%q summary_bytes=%d dry_run=%t",
			file.ID,
			file.Name,
			result.CSVFilesProcessed,
			result.RowsConsolidated,
			result.RowsImported,
			result.ObjectKey,
			result.ObjectBytes,
			result.SummaryImageSent,
			result.SummaryStable,
			result.SummaryImageFmt,
			result.SummaryImageBytes,
			cfg.DryRun,
		)
	}

	status.FileID = lastProcessedFile.ID
	status.FileName = lastProcessedFile.Name
	status.FileModified = lastProcessedFile.ModifiedTime.Format(time.RFC3339)
	status.FileMD5 = lastProcessedFile.MD5Checksum
	status.CSVFilesProcessed = totalResult.CSVFilesProcessed
	status.RowsConsolidated = totalResult.RowsConsolidated
	status.RowsImported = totalResult.RowsImported
	status.ObjectKey = lastProcessedCycle.ObjectKey
	status.ObjectBytes = lastProcessedCycle.ObjectBytes
	status.SummaryImageSent = lastProcessedCycle.SummaryImageSent
	status.SummaryStable = lastProcessedCycle.SummaryStable
	status.SummaryImageFmt = lastProcessedCycle.SummaryImageFmt
	status.SummaryImageBytes = lastProcessedCycle.SummaryImageBytes
	status.Message = fmt.Sprintf("processed %d new zip file(s)", len(pending))
	writeStatusIfConfigured(cfg.StatusFile, status, logger)
	return nil
}

func processZipAndImport(
	ctx context.Context,
	cfg workflowConfig,
	sheetsSvc *sheets.Service,
	r2Client *s3.Client,
	file driveZipFile,
	zipPath string,
	sendSummaryAfterImport bool,
) (processResult, error) {
	var result processResult

	zipFile, err := os.Open(zipPath)
	if err != nil {
		return result, err
	}
	defer zipFile.Close()

	zipStat, err := zipFile.Stat()
	if err != nil {
		return result, err
	}

	reader, err := zip.NewReader(zipFile, zipStat.Size())
	if err != nil {
		return result, fmt.Errorf("open zip: %w", err)
	}

	csvFiles := make([]*zip.File, 0)
	for _, entry := range reader.File {
		if entry.FileInfo().IsDir() {
			continue
		}
		if strings.EqualFold(filepath.Ext(entry.Name), ".csv") {
			csvFiles = append(csvFiles, entry)
		}
	}
	sort.Slice(csvFiles, func(i, j int) bool {
		return strings.ToLower(csvFiles[i].Name) < strings.ToLower(csvFiles[j].Name)
	})
	if len(csvFiles) == 0 {
		return result, errors.New("zip contains no csv files")
	}

	consolidatedFile, err := os.CreateTemp(cfg.TempDir, "wf21-consolidated-*.csv")
	if err != nil {
		return result, err
	}
	defer func() {
		consolidatedFile.Close()
		os.Remove(consolidatedFile.Name())
	}()

	bufferedWriter := bufio.NewWriterSize(consolidatedFile, 1<<20)
	csvWriter := csv.NewWriter(bufferedWriter)

	var canonicalHeader []string
	var canonicalHeaderMap map[string]int
	selectorIndexes := make([]int, len(selectedOutputHeaders))
	for i := range selectorIndexes {
		selectorIndexes[i] = -1
	}
	filterCurrentStationIdx := -1
	filterReceiverTypeIdx := -1
	nextSheetRow := 2
	pendingSheetRows := make([][]string, 0, cfg.SheetsBatchSize)
	var gridState sheetGridState

	if !cfg.DryRun {
		if err = loadSheetGridState(ctx, sheetsSvc, cfg.DestinationSheetID, cfg.DestinationTab, &gridState); err != nil {
			return result, err
		}
		if err = clearDestinationSheet(ctx, sheetsSvc, cfg.DestinationSheetID, cfg.DestinationTab); err != nil {
			return result, err
		}
		if err = writeHeaderRow(ctx, sheetsSvc, cfg.DestinationSheetID, cfg.DestinationTab, selectedOutputHeaders); err != nil {
			return result, err
		}
	}

	for _, entry := range csvFiles {
		entryReader, openErr := entry.Open()
		if openErr != nil {
			return result, fmt.Errorf("open csv %q: %w", entry.Name, openErr)
		}

		csvReader := csv.NewReader(bufio.NewReaderSize(entryReader, 1<<20))
		csvReader.FieldsPerRecord = -1

		header, readHeaderErr := csvReader.Read()
		if readHeaderErr != nil {
			entryReader.Close()
			if errors.Is(readHeaderErr, io.EOF) {
				continue
			}
			return result, fmt.Errorf("read header %q: %w", entry.Name, readHeaderErr)
		}

		header, dropLeading := normalizeHeaderRecord(header, cfg.DropLeadingUnnamedColumn)
		if len(header) == 0 {
			entryReader.Close()
			continue
		}

		if canonicalHeader == nil {
			canonicalHeader = append([]string(nil), header...)
			canonicalHeaderMap = buildHeaderMap(canonicalHeader)

			filterCurrentStationIdx = findIndexByHeader(canonicalHeaderMap, "Current Station", 12, len(canonicalHeader))
			filterReceiverTypeIdx = findIndexByHeader(canonicalHeaderMap, "Receiver Type", 10, len(canonicalHeader))

			for i, name := range selectedOutputHeaders {
				if idx, ok := canonicalHeaderMap[normalizeHeaderKey(name)]; ok {
					selectorIndexes[i] = idx
				}
			}

			if err = csvWriter.Write(canonicalHeader); err != nil {
				entryReader.Close()
				return result, fmt.Errorf("write consolidated header: %w", err)
			}
		}

		canonicalMap := buildCanonicalColumnMap(canonicalHeader, header)
		for {
			record, readErr := csvReader.Read()
			if readErr != nil {
				if errors.Is(readErr, io.EOF) {
					break
				}
				entryReader.Close()
				return result, fmt.Errorf("read csv row %q: %w", entry.Name, readErr)
			}
			record = normalizeDataRecord(record, dropLeading)
			canonicalRow := mapRecordToCanonical(record, canonicalMap, len(canonicalHeader))

			if err = csvWriter.Write(canonicalRow); err != nil {
				entryReader.Close()
				return result, fmt.Errorf("write consolidated row: %w", err)
			}
			result.RowsConsolidated++

			if rowMatchesFilters(canonicalRow, filterReceiverTypeIdx, filterCurrentStationIdx) {
				result.RowsImported++
				picked := pickColumns(canonicalRow, selectorIndexes)
				if !cfg.DryRun {
					pendingSheetRows = append(pendingSheetRows, picked)
					if len(pendingSheetRows) >= cfg.SheetsBatchSize {
						if err = writeRowsBatch(ctx, sheetsSvc, cfg.DestinationSheetID, cfg.DestinationTab, nextSheetRow, pendingSheetRows, &gridState); err != nil {
							entryReader.Close()
							return result, err
						}
						nextSheetRow += len(pendingSheetRows)
						pendingSheetRows = pendingSheetRows[:0]
					}
				}
			}
		}
		entryReader.Close()
		result.CSVFilesProcessed++
	}

	if canonicalHeader == nil {
		return result, errors.New("no valid csv headers found")
	}

	csvWriter.Flush()
	if err = csvWriter.Error(); err != nil {
		return result, err
	}
	if err = bufferedWriter.Flush(); err != nil {
		return result, err
	}
	if err = consolidatedFile.Sync(); err != nil {
		return result, err
	}

	if !cfg.DryRun && len(pendingSheetRows) > 0 {
		if err = writeRowsBatch(ctx, sheetsSvc, cfg.DestinationSheetID, cfg.DestinationTab, nextSheetRow, pendingSheetRows, &gridState); err != nil {
			return result, err
		}
	}

	consolidatedInfo, err := consolidatedFile.Stat()
	if err != nil {
		return result, err
	}
	result.ObjectBytes = consolidatedInfo.Size()

	objectKey := buildObjectKey(cfg.R2ObjectPrefix, file, time.Now().UTC())
	result.ObjectKey = objectKey

	if !cfg.DryRun {
		if _, err = consolidatedFile.Seek(0, io.SeekStart); err != nil {
			return result, err
		}
		if err = uploadToR2(ctx, r2Client, cfg.R2Bucket, objectKey, consolidatedFile); err != nil {
			return result, err
		}
	}

	if sendSummaryAfterImport && !cfg.DryRun && cfg.SummarySendEnabled {
		summaryResult, summaryErr := sendSummarySnapshotToSeaTalk(ctx, cfg, sheetsSvc)
		if summaryErr != nil {
			return result, summaryErr
		}
		result.SummaryImageSent = true
		result.SummaryStable = summaryResult.Stable
		result.SummaryImageFmt = summaryResult.Format
		result.SummaryImageBytes = summaryResult.RawBytes
	}

	return result, nil
}

func loadConfig() (workflowConfig, error) {
	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		return workflowConfig{}, fmt.Errorf("load .env: %w", err)
	}

	continuous, err := getBoolEnv("WF21_CONTINUOUS", true)
	if err != nil {
		return workflowConfig{}, err
	}
	dryRun, err := getBoolEnv("WF21_DRY_RUN", false)
	if err != nil {
		return workflowConfig{}, err
	}
	bootstrapProcessExisting, err := getBoolEnv("WF21_BOOTSTRAP_PROCESS_EXISTING", true)
	if err != nil {
		return workflowConfig{}, err
	}
	dropLeadingUnnamed, err := getBoolEnv("WF21_DROP_LEADING_UNNAMED_COLUMN", true)
	if err != nil {
		return workflowConfig{}, err
	}
	enableHealthServer, err := getBoolEnv("WF21_ENABLE_HEALTH_SERVER", true)
	if err != nil {
		return workflowConfig{}, err
	}
	summarySendEnabled, err := getBoolEnv("WF21_SUMMARY_SEND_ENABLED", true)
	if err != nil {
		return workflowConfig{}, err
	}
	summarySeaTalkMode := strings.ToLower(strings.TrimSpace(firstNonEmpty(
		os.Getenv("WF21_SUMMARY_SEATALK_MODE"),
		"bot",
	)))

	credsFile := firstNonEmpty(
		strings.TrimSpace(os.Getenv("WF21_GOOGLE_CREDENTIALS_FILE")),
		strings.TrimSpace(os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")),
	)
	credsJSON := strings.TrimSpace(os.Getenv("WF21_GOOGLE_CREDENTIALS_JSON"))
	if credsFile == "" && credsJSON == "" {
		return workflowConfig{}, errors.New("set WF21_GOOGLE_CREDENTIALS_FILE/GOOGLE_APPLICATION_CREDENTIALS or WF21_GOOGLE_CREDENTIALS_JSON")
	}

	r2AccountID := strings.TrimSpace(os.Getenv("WF21_R2_ACCOUNT_ID"))
	r2Bucket := strings.TrimSpace(os.Getenv("WF21_R2_BUCKET"))
	r2AccessKeyID := strings.TrimSpace(os.Getenv("WF21_R2_ACCESS_KEY_ID"))
	r2SecretAccessKey := strings.TrimSpace(os.Getenv("WF21_R2_SECRET_ACCESS_KEY"))
	if r2AccountID == "" || r2Bucket == "" || r2AccessKeyID == "" || r2SecretAccessKey == "" {
		return workflowConfig{}, errors.New("WF21_R2_ACCOUNT_ID, WF21_R2_BUCKET, WF21_R2_ACCESS_KEY_ID, WF21_R2_SECRET_ACCESS_KEY are required")
	}

	statusFile := strings.TrimSpace(os.Getenv("WF21_STATUS_FILE"))
	switch strings.ToLower(statusFile) {
	case "none", "off":
		statusFile = ""
	case "":
		statusFile = defaultStatusFile
	}
	healthPort := firstNonEmpty(
		strings.TrimSpace(os.Getenv("WF21_HEALTH_PORT")),
		strings.TrimSpace(os.Getenv("PORT")),
		"8080",
	)
	summarySeaTalkAppID := strings.TrimSpace(firstNonEmpty(
		os.Getenv("WF21_SEATALK_APP_ID"),
		os.Getenv("WF2_SEATALK_APP_ID"),
		os.Getenv("SEATALK_APP_ID"),
	))
	summarySeaTalkSecret := strings.TrimSpace(firstNonEmpty(
		os.Getenv("WF21_SEATALK_APP_SECRET"),
		os.Getenv("WF2_SEATALK_APP_SECRET"),
		os.Getenv("SEATALK_APP_SECRET"),
	))
	summarySeaTalkBaseURL := strings.TrimSpace(firstNonEmpty(
		os.Getenv("WF21_SEATALK_BASE_URL"),
		os.Getenv("WF2_SEATALK_BASE_URL"),
		os.Getenv("SEATALK_BASE_URL"),
		"https://openapi.seatalk.io",
	))
	summarySeaTalkGroupID := strings.TrimSpace(firstNonEmpty(
		os.Getenv("WF21_SEATALK_GROUP_ID"),
		os.Getenv("WF2_SEATALK_GROUP_ID"),
	))
	summaryWebhookURL := strings.TrimSpace(firstNonEmpty(
		os.Getenv("WF21_SEATALK_WEBHOOK_URL"),
		os.Getenv("SEATALK_SYSTEM_WEBHOOK_URL"),
	))
	summaryTimezone := firstNonEmpty(
		strings.TrimSpace(os.Getenv("WF21_TIMEZONE")),
		strings.TrimSpace(os.Getenv("WF2_TIMEZONE")),
		defaultSummaryTimezone,
	)
	summaryLocation, err := time.LoadLocation(summaryTimezone)
	if err != nil {
		return workflowConfig{}, fmt.Errorf("invalid WF21_TIMEZONE %q: %w", summaryTimezone, err)
	}

	cfg := workflowConfig{
		GoogleCredentialsFile:    credsFile,
		GoogleCredentialsJSON:    credsJSON,
		DriveParentFolderID:      firstNonEmpty(strings.TrimSpace(os.Getenv("WF21_DRIVE_PARENT_FOLDER_ID")), defaultDriveParentFolderID),
		DestinationSheetID:       firstNonEmpty(strings.TrimSpace(os.Getenv("WF21_DESTINATION_SHEET_ID")), defaultDestinationSheetID),
		DestinationTab:           firstNonEmpty(strings.TrimSpace(os.Getenv("WF21_DESTINATION_TAB")), defaultDestinationTab),
		R2AccountID:              r2AccountID,
		R2Bucket:                 r2Bucket,
		R2AccessKeyID:            r2AccessKeyID,
		R2SecretAccessKey:        r2SecretAccessKey,
		R2ObjectPrefix:           firstNonEmpty(strings.TrimSpace(os.Getenv("WF21_R2_OBJECT_PREFIX")), defaultR2ObjectPrefx),
		Continuous:               continuous,
		PollInterval:             getDurationSeconds("WF21_POLL_INTERVAL_SECONDS", int(defaultPollInterval/time.Second)),
		DryRun:                   dryRun,
		BootstrapProcessExisting: bootstrapProcessExisting,
		DropLeadingUnnamedColumn: dropLeadingUnnamed,
		SheetsBatchSize:          getIntEnv("WF21_SHEETS_BATCH_SIZE", defaultSheetsBatch),
		TempDir:                  strings.TrimSpace(os.Getenv("WF21_TEMP_DIR")),
		StateFile:                firstNonEmpty(strings.TrimSpace(os.Getenv("WF21_STATE_FILE")), defaultStateFile),
		StatusFile:               statusFile,
		EnableHealthServer:       enableHealthServer,
		HealthListenAddr:         normalizeListenAddr(healthPort),
		SummarySendEnabled:       summarySendEnabled,
		SummarySeaTalkMode:       summarySeaTalkMode,
		SummaryWebhookURL:        summaryWebhookURL,
		SummarySeaTalkAppID:      summarySeaTalkAppID,
		SummarySeaTalkSecret:     summarySeaTalkSecret,
		SummarySeaTalkBaseURL:    summarySeaTalkBaseURL,
		SummarySeaTalkGroupID:    summarySeaTalkGroupID,
		SummarySheetID: firstNonEmpty(
			strings.TrimSpace(os.Getenv("WF21_SUMMARY_SHEET_ID")),
			firstNonEmpty(strings.TrimSpace(os.Getenv("WF21_DESTINATION_SHEET_ID")), defaultDestinationSheetID),
		),
		SummaryTab: firstNonEmpty(
			strings.TrimSpace(os.Getenv("WF21_SUMMARY_TAB")),
			defaultSummarySheetTab,
		),
		SummaryRange: firstNonEmpty(
			strings.TrimSpace(os.Getenv("WF21_SUMMARY_RANGE")),
			defaultSummaryRange,
		),
		SummaryWaitAfterImport: getDurationSeconds("WF21_SUMMARY_WAIT_SECONDS", int(defaultSummaryWaitAfterImport/time.Second)),
		SummaryStabilityWait:   getDurationSeconds("WF21_SUMMARY_STABILITY_WAIT_SECONDS", int(defaultSummaryStabilityWait/time.Second)),
		SummaryStabilityRuns:   getIntEnv("WF21_SUMMARY_STABILITY_RUNS", defaultSummaryStabilityRuns),
		SummaryRenderScale:     getIntEnv("WF21_SUMMARY_RENDER_SCALE", defaultSummaryRenderScale),
		SummaryImageMaxWidthPx: getIntEnv("WF21_SUMMARY_IMAGE_MAX_WIDTH_PX", defaultSummaryImageMaxWidthPx),
		SummaryImageMaxBase64:  getIntEnv("WF21_SUMMARY_IMAGE_MAX_BASE64_BYTES", defaultSummaryImageMaxBase64),
		SummarySendHTTPTimeout: getDurationSeconds("WF21_SUMMARY_HTTP_TIMEOUT_SECONDS", int(defaultSummaryHTTPTimeout/time.Second)),
		SummaryTimezone:        summaryTimezone,
		SummaryLocation:        summaryLocation,
	}

	if cfg.PollInterval < 5*time.Second {
		cfg.PollInterval = 5 * time.Second
	}
	if cfg.SheetsBatchSize < 100 {
		cfg.SheetsBatchSize = 100
	}
	switch cfg.SummarySeaTalkMode {
	case "bot", "webhook":
	default:
		return workflowConfig{}, fmt.Errorf("WF21_SUMMARY_SEATALK_MODE must be one of: bot, webhook (got %q)", cfg.SummarySeaTalkMode)
	}
	if cfg.SummarySendEnabled && !cfg.DryRun {
		if cfg.SummarySeaTalkMode == "bot" {
			if cfg.SummarySeaTalkAppID == "" || cfg.SummarySeaTalkSecret == "" {
				return workflowConfig{}, errors.New("WF21_SEATALK_APP_ID/WF21_SEATALK_APP_SECRET (or WF2_/SEATALK_ fallbacks) are required when WF21_SUMMARY_SEATALK_MODE=bot")
			}
			if cfg.SummarySeaTalkGroupID == "" {
				return workflowConfig{}, errors.New("WF21_SEATALK_GROUP_ID (or WF2_SEATALK_GROUP_ID fallback) is required when WF21_SUMMARY_SEATALK_MODE=bot")
			}
		}
		if cfg.SummarySeaTalkMode == "webhook" && cfg.SummaryWebhookURL == "" {
			return workflowConfig{}, errors.New("WF21_SEATALK_WEBHOOK_URL or SEATALK_SYSTEM_WEBHOOK_URL is required when WF21_SUMMARY_SEATALK_MODE=webhook")
		}
	}
	if cfg.SummaryStabilityRuns < 1 {
		cfg.SummaryStabilityRuns = 1
	}
	if cfg.SummaryRenderScale < 1 {
		cfg.SummaryRenderScale = 1
	}
	if cfg.SummaryRenderScale > 4 {
		cfg.SummaryRenderScale = 4
	}
	if cfg.SummaryImageMaxWidthPx < 1200 {
		cfg.SummaryImageMaxWidthPx = 1200
	}
	if cfg.SummaryImageMaxBase64 < 512*1024 {
		cfg.SummaryImageMaxBase64 = defaultSummaryImageMaxBase64
	}
	return cfg, nil
}

func newGoogleServices(ctx context.Context, cfg workflowConfig) (*drive.Service, *sheets.Service, error) {
	clientOptions := []option.ClientOption{
		option.WithScopes(drive.DriveReadonlyScope, sheets.SpreadsheetsScope),
	}
	if cfg.GoogleCredentialsJSON != "" {
		clientOptions = append(clientOptions, option.WithCredentialsJSON([]byte(cfg.GoogleCredentialsJSON)))
	} else {
		clientOptions = append(clientOptions, option.WithCredentialsFile(cfg.GoogleCredentialsFile))
	}

	driveSvc, err := drive.NewService(ctx, clientOptions...)
	if err != nil {
		return nil, nil, err
	}
	sheetsSvc, err := sheets.NewService(ctx, clientOptions...)
	if err != nil {
		return nil, nil, err
	}
	return driveSvc, sheetsSvc, nil
}

func newR2Client(ctx context.Context, cfg workflowConfig) (*s3.Client, error) {
	awsConfig, err := awscfg.LoadDefaultConfig(
		ctx,
		awscfg.WithRegion("auto"),
		awscfg.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(cfg.R2AccessKeyID, cfg.R2SecretAccessKey, "")),
	)
	if err != nil {
		return nil, err
	}
	endpoint := fmt.Sprintf("https://%s.r2.cloudflarestorage.com", cfg.R2AccountID)
	client := s3.NewFromConfig(awsConfig, func(o *s3.Options) {
		o.UsePathStyle = true
		o.BaseEndpoint = aws.String(endpoint)
	})
	return client, nil
}

func listZipFiles(ctx context.Context, driveSvc *drive.Service, parentFolderID string) ([]driveZipFile, error) {
	query := fmt.Sprintf("'%s' in parents and trashed=false and (mimeType='application/zip' or name contains '.zip' or name contains '.ZIP')", parentFolderID)
	out := make([]driveZipFile, 0, 32)

	pageToken := ""
	for {
		call := driveSvc.Files.List().
			Q(query).
			OrderBy("modifiedTime asc,name asc").
			PageSize(200).
			SupportsAllDrives(true).
			IncludeItemsFromAllDrives(true).
			Fields("nextPageToken,files(id,name,md5Checksum,modifiedTime,size,mimeType)").
			Context(ctx)
		if pageToken != "" {
			call = call.PageToken(pageToken)
		}

		resp, err := call.Do()
		if err != nil {
			return nil, fmt.Errorf("list drive files: %w", err)
		}
		for _, f := range resp.Files {
			if f == nil {
				continue
			}
			if !strings.EqualFold(filepath.Ext(f.Name), ".zip") && !strings.EqualFold(f.MimeType, "application/zip") {
				continue
			}
			modified, parseErr := time.Parse(time.RFC3339, f.ModifiedTime)
			if parseErr != nil {
				modified = time.Time{}
			}
			out = append(out, driveZipFile{
				ID:           f.Id,
				Name:         f.Name,
				MD5Checksum:  f.Md5Checksum,
				ModifiedTime: modified,
				Size:         f.Size,
			})
		}
		if strings.TrimSpace(resp.NextPageToken) == "" {
			break
		}
		pageToken = resp.NextPageToken
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].ModifiedTime.Equal(out[j].ModifiedTime) {
			leftName := strings.ToLower(strings.TrimSpace(out[i].Name))
			rightName := strings.ToLower(strings.TrimSpace(out[j].Name))
			if leftName == rightName {
				return strings.TrimSpace(out[i].ID) < strings.TrimSpace(out[j].ID)
			}
			return leftName < rightName
		}
		return out[i].ModifiedTime.Before(out[j].ModifiedTime)
	})
	return out, nil
}

func downloadDriveFileToTemp(ctx context.Context, driveSvc *drive.Service, fileID, tempDir string) (string, error) {
	resp, err := driveSvc.Files.Get(fileID).SupportsAllDrives(true).Context(ctx).Download()
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	tempFile, err := os.CreateTemp(tempDir, "wf21-zip-*.zip")
	if err != nil {
		return "", err
	}
	defer tempFile.Close()

	if _, err = io.Copy(tempFile, resp.Body); err != nil {
		os.Remove(tempFile.Name())
		return "", err
	}
	if err = tempFile.Sync(); err != nil {
		os.Remove(tempFile.Name())
		return "", err
	}
	return tempFile.Name(), nil
}

func clearDestinationSheet(ctx context.Context, sheetsSvc *sheets.Service, sheetID, tab string) error {
	endCol := columnNameFromIndex(len(selectedOutputHeaders))
	_, err := sheetsSvc.Spreadsheets.Values.Clear(sheetID, fmt.Sprintf("%s!A:%s", tab, endCol), &sheets.ClearValuesRequest{}).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("clear destination sheet: %w", err)
	}
	return nil
}

func writeHeaderRow(ctx context.Context, sheetsSvc *sheets.Service, sheetID, tab string, headers []string) error {
	values := []interface{}{}
	for _, h := range headers {
		values = append(values, h)
	}
	vr := &sheets.ValueRange{
		Values: [][]interface{}{values},
	}
	endCol := columnNameFromIndex(len(headers))
	targetRange := fmt.Sprintf("%s!A1:%s1", tab, endCol)
	_, err := sheetsSvc.Spreadsheets.Values.Update(sheetID, targetRange, vr).
		ValueInputOption("RAW").
		Context(ctx).
		Do()
	if err != nil {
		return fmt.Errorf("write header row range=%s headers=%d: %w", targetRange, len(headers), err)
	}
	return nil
}

func writeRowsBatch(
	ctx context.Context,
	sheetsSvc *sheets.Service,
	sheetID, tab string,
	startRow int,
	rows [][]string,
	gridState *sheetGridState,
) error {
	if len(rows) == 0 {
		return nil
	}
	payload := make([][]interface{}, 0, len(rows))
	for _, row := range rows {
		items := make([]interface{}, 0, len(row))
		for _, val := range row {
			items = append(items, val)
		}
		payload = append(payload, items)
	}
	endRow := startRow + len(rows) - 1
	if err := ensureSheetGridCapacity(ctx, sheetsSvc, sheetID, tab, endRow, len(selectedOutputHeaders), gridState); err != nil {
		return err
	}
	endCol := columnNameFromIndex(len(selectedOutputHeaders))
	targetRange := fmt.Sprintf("%s!A%d:%s%d", tab, startRow, endCol, endRow)
	vr := &sheets.ValueRange{Values: payload}
	_, err := sheetsSvc.Spreadsheets.Values.Update(sheetID, targetRange, vr).
		ValueInputOption("RAW").
		Context(ctx).
		Do()
	if err != nil {
		return fmt.Errorf("write rows batch [%d..%d]: %w", startRow, endRow, err)
	}
	return nil
}

func loadSheetGridState(ctx context.Context, sheetsSvc *sheets.Service, spreadsheetID, tab string, state *sheetGridState) error {
	if state != nil && state.loaded {
		return nil
	}
	resp, err := sheetsSvc.Spreadsheets.Get(spreadsheetID).
		Fields("sheets(properties(sheetId,title,gridProperties(rowCount,columnCount)))").
		Context(ctx).
		Do()
	if err != nil {
		return fmt.Errorf("load destination sheet metadata: %w", err)
	}
	for _, sh := range resp.Sheets {
		if sh == nil || sh.Properties == nil {
			continue
		}
		if sh.Properties.Title != tab {
			continue
		}
		rows := 1000
		cols := 26
		if sh.Properties.GridProperties != nil {
			if sh.Properties.GridProperties.RowCount > 0 {
				rows = int(sh.Properties.GridProperties.RowCount)
			}
			if sh.Properties.GridProperties.ColumnCount > 0 {
				cols = int(sh.Properties.GridProperties.ColumnCount)
			}
		}
		if state != nil {
			state.sheetID = sh.Properties.SheetId
			state.rows = rows
			state.cols = cols
			state.loaded = true
		}
		return nil
	}
	return fmt.Errorf("destination tab %q not found in sheet %s", tab, spreadsheetID)
}

func ensureSheetGridCapacity(
	ctx context.Context,
	sheetsSvc *sheets.Service,
	spreadsheetID, tab string,
	requiredRows, requiredCols int,
	state *sheetGridState,
) error {
	if state == nil {
		return errors.New("sheet grid state is required")
	}
	if err := loadSheetGridState(ctx, sheetsSvc, spreadsheetID, tab, state); err != nil {
		return err
	}

	needsRows := requiredRows > state.rows
	needsCols := requiredCols > state.cols
	if !needsRows && !needsCols {
		return nil
	}

	newRows := state.rows
	newCols := state.cols
	if needsRows {
		newRows = requiredRows
	}
	if needsCols {
		newCols = requiredCols
	}

	req := &sheets.BatchUpdateSpreadsheetRequest{
		Requests: []*sheets.Request{
			{
				UpdateSheetProperties: &sheets.UpdateSheetPropertiesRequest{
					Properties: &sheets.SheetProperties{
						SheetId: state.sheetID,
						GridProperties: &sheets.GridProperties{
							RowCount:    int64(newRows),
							ColumnCount: int64(newCols),
						},
					},
					Fields: "gridProperties(rowCount,columnCount)",
				},
			},
		},
	}
	if _, err := sheetsSvc.Spreadsheets.BatchUpdate(spreadsheetID, req).Context(ctx).Do(); err != nil {
		return fmt.Errorf("resize destination sheet to rows=%d cols=%d: %w", newRows, newCols, err)
	}
	state.rows = newRows
	state.cols = newCols
	return nil
}

func uploadToR2(ctx context.Context, client *s3.Client, bucket, objectKey string, source *os.File) error {
	_, err := client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(bucket),
		Key:         aws.String(objectKey),
		Body:        source,
		ContentType: aws.String("text/csv"),
	})
	if err != nil {
		return fmt.Errorf("upload to r2 bucket=%s key=%s: %w", bucket, objectKey, err)
	}
	return nil
}

func buildObjectKey(prefix string, file driveZipFile, now time.Time) string {
	base := strings.TrimSuffix(filepath.Base(file.Name), filepath.Ext(file.Name))
	base = sanitizeObjectToken(base)
	if base == "" {
		base = "input"
	}
	fileID := sanitizeObjectToken(file.ID)
	if fileID == "" {
		fileID = "unknown"
	}
	name := fmt.Sprintf("%s-%s-%s.csv", now.Format("20060102T150405.000000000Z"), base, fileID)
	cleanPrefix := strings.Trim(strings.TrimSpace(prefix), "/")
	if cleanPrefix == "" {
		return name
	}
	return cleanPrefix + "/" + name
}

func sanitizeObjectToken(v string) string {
	v = strings.TrimSpace(strings.ToLower(v))
	var b strings.Builder
	for _, ch := range v {
		if (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') {
			b.WriteRune(ch)
			continue
		}
		if ch == '-' || ch == '_' || ch == '.' {
			b.WriteRune(ch)
			continue
		}
		b.WriteRune('-')
	}
	out := strings.Trim(b.String(), "-")
	out = strings.ReplaceAll(out, "--", "-")
	return out
}

func shouldProcessFile(file driveZipFile, state workflowState, stateExists bool) bool {
	if !stateExists {
		return true
	}
	if strings.TrimSpace(state.LastProcessedFileID) != strings.TrimSpace(file.ID) {
		return true
	}
	if file.MD5Checksum != "" && strings.TrimSpace(state.LastProcessedFileMD5) != strings.TrimSpace(file.MD5Checksum) {
		return true
	}
	fileModified := file.ModifiedTime.Format(time.RFC3339)
	if fileModified != "" && strings.TrimSpace(state.LastProcessedModifiedTime) != strings.TrimSpace(fileModified) {
		return true
	}
	return false
}

func selectPendingZipFiles(files []driveZipFile, state workflowState) []driveZipFile {
	if len(files) == 0 {
		return nil
	}

	lastModified, err := time.Parse(time.RFC3339, strings.TrimSpace(state.LastProcessedModifiedTime))
	if err != nil {
		latest := files[len(files)-1]
		if shouldProcessFile(latest, state, true) {
			return []driveZipFile{latest}
		}
		return nil
	}

	lastID := strings.TrimSpace(state.LastProcessedFileID)
	pending := make([]driveZipFile, 0)
	for _, file := range files {
		if file.ModifiedTime.After(lastModified) {
			pending = append(pending, file)
			continue
		}
		if !file.ModifiedTime.Equal(lastModified) {
			continue
		}

		fileID := strings.TrimSpace(file.ID)
		if fileID == lastID {
			if shouldProcessFile(file, state, true) {
				pending = append(pending, file)
			}
			continue
		}
		if lastID == "" || fileID > lastID {
			pending = append(pending, file)
		}
	}
	return pending
}

func normalizeHeaderRecord(header []string, dropLeadingUnnamed bool) ([]string, bool) {
	cleaned := make([]string, len(header))
	for i, item := range header {
		cleaned[i] = strings.TrimSpace(strings.TrimPrefix(item, "\ufeff"))
	}
	dropped := false
	if dropLeadingUnnamed && len(cleaned) > 1 && isUnnamedHeader(cleaned[0]) {
		cleaned = cleaned[1:]
		dropped = true
	}
	return cleaned, dropped
}

func normalizeDataRecord(record []string, dropLeading bool) []string {
	values := record
	if dropLeading && len(values) > 0 {
		values = values[1:]
	}
	out := make([]string, len(values))
	for i, item := range values {
		out[i] = strings.TrimSpace(item)
	}
	return out
}

func isUnnamedHeader(v string) bool {
	key := strings.ToLower(strings.TrimSpace(v))
	return key == "" || strings.HasPrefix(key, "unnamed")
}

func buildHeaderMap(header []string) map[string]int {
	result := make(map[string]int, len(header))
	for idx, name := range header {
		key := normalizeHeaderKey(name)
		if key == "" {
			continue
		}
		if _, exists := result[key]; !exists {
			result[key] = idx
		}
	}
	return result
}

func normalizeHeaderKey(name string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(name)), " "))
}

func findIndexByHeader(headerMap map[string]int, header string, fallback int, headerLen int) int {
	if idx, ok := headerMap[normalizeHeaderKey(header)]; ok {
		return idx
	}
	if fallback >= 0 && fallback < headerLen {
		return fallback
	}
	return -1
}

func buildCanonicalColumnMap(canonicalHeader, fileHeader []string) []int {
	result := make([]int, len(canonicalHeader))
	for i := range result {
		result[i] = -1
	}
	fileHeaderMap := buildHeaderMap(fileHeader)
	for canonicalIdx, name := range canonicalHeader {
		if srcIdx, ok := fileHeaderMap[normalizeHeaderKey(name)]; ok {
			result[canonicalIdx] = srcIdx
		}
	}
	return result
}

func mapRecordToCanonical(record []string, canonicalMap []int, canonicalLen int) []string {
	result := make([]string, canonicalLen)
	for targetIdx := 0; targetIdx < canonicalLen; targetIdx++ {
		srcIdx := canonicalMap[targetIdx]
		if srcIdx >= 0 && srcIdx < len(record) {
			result[targetIdx] = record[srcIdx]
		}
	}
	return result
}

func rowMatchesFilters(row []string, receiverTypeIdx, currentStationIdx int) bool {
	if receiverTypeIdx < 0 || receiverTypeIdx >= len(row) {
		return false
	}
	if currentStationIdx < 0 || currentStationIdx >= len(row) {
		return false
	}
	receiverType := strings.TrimSpace(row[receiverTypeIdx])
	currentStation := strings.TrimSpace(row[currentStationIdx])
	return strings.EqualFold(receiverType, "Station") && strings.EqualFold(currentStation, "SOC 5")
}

func pickColumns(row []string, indexes []int) []string {
	out := make([]string, len(indexes))
	for i, idx := range indexes {
		if idx >= 0 && idx < len(row) {
			out[i] = row[idx]
		}
	}
	return out
}

func loadState(path string) (workflowState, bool, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return workflowState{}, false, nil
		}
		return workflowState{}, false, err
	}
	var parsed workflowState
	if err = json.Unmarshal(content, &parsed); err != nil {
		return workflowState{}, false, fmt.Errorf("decode state file %s: %w", path, err)
	}
	return parsed, true, nil
}

func saveState(path string, state workflowState) error {
	content, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return writeJSONFile(path, content)
}

func saveStatus(path string, status workflowStatus) error {
	content, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		return err
	}
	return writeJSONFile(path, content)
}

func writeStatusIfConfigured(path string, status workflowStatus, logger *log.Logger) {
	if strings.TrimSpace(path) == "" {
		return
	}
	if err := saveStatus(path, status); err != nil {
		logger.Printf("status write failed path=%s err=%v", path, err)
	}
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

func writeJSONFile(path string, payload []byte) error {
	dir := filepath.Dir(path)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, append(payload, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

func getBoolEnv(name string, defaultValue bool) (bool, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return defaultValue, nil
	}
	parsed, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("%s: %w", name, err)
	}
	return parsed, nil
}

func getIntEnv(name string, defaultValue int) int {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return defaultValue
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil {
		return defaultValue
	}
	return parsed
}

func getDurationSeconds(name string, defaultSeconds int) time.Duration {
	seconds := getIntEnv(name, defaultSeconds)
	if seconds <= 0 {
		seconds = defaultSeconds
	}
	return time.Duration(seconds) * time.Second
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func columnNameFromIndex(index int) string {
	if index <= 0 {
		return "A"
	}
	value := index
	var out []byte
	for value > 0 {
		value--
		out = append([]byte{byte('A' + (value % 26))}, out...)
		value /= 26
	}
	return string(out)
}

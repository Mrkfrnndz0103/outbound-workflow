package main

import (
	"archive/zip"
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
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
	"github.com/spxph4227/go-bot-server/internal/botconfig"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"
)

const (
	workflowName = "workflow_2_1_drive_csv_consolidation"

	defaultDriveParentFolderID           = "1oU9kj5VIJIoNrR388wYCHSdtHGanRrgZ"
	defaultDestinationSheetID            = "1mdi-8ACluDHGZ7yAyNLwXLwpmQ4f6VAx3kpbaJORViA"
	defaultDestinationTabPendingRCV      = "pending_rcv"
	defaultDestinationTabPackedAnotherTO = "packed_in_another_to"
	defaultDestinationTabNoLHPacking     = "no_lhpacking"

	defaultStateFile  = "data/workflow2-1-drive-csv-consolidation-state.json"
	defaultStatusFile = "data/workflow2-1-drive-csv-consolidation-status.json"

	defaultPollInterval                = 3 * time.Second
	defaultSheetsBatch                 = 7000
	defaultR2ObjectPrefx               = "wf2-1"
	defaultSheetsWriteRetryMaxAttempts = 6
	defaultSheetsWriteRetryBaseDelay   = 1 * time.Second
	defaultSheetsWriteRetryMaxDelay    = 15 * time.Second

	defaultSummarySheetTab        = "[SOC] Backlogs Summary"
	defaultSummaryRange           = "B2:Q59"
	defaultSummarySecondTab       = "config"
	defaultSummarySecondRanges    = "E154:Y184"
	defaultSummaryWaitAfterImport = 5 * time.Second
	defaultSummaryStabilityWait   = 2 * time.Second
	defaultSummaryStabilityRuns   = 3
	defaultSummaryRenderMode      = "styled"
	defaultSummaryRenderScale     = 2
	defaultSummaryAutoFitColumns  = false
	defaultSummaryPDFDPI          = 180
	defaultSummaryPDFConverter    = "auto"
	defaultSummaryImageMaxWidthPx = 3000
	defaultSummaryImageMaxBase64  = 5 * 1024 * 1024
	defaultSummaryHTTPTimeout     = 45 * time.Second
	defaultSummaryTimezone        = "Asia/Manila"
	defaultSummarySyncCell        = "config!B1"
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

	DestinationSheetID            string
	DestinationTabPendingRCV      string
	DestinationTabPackedAnotherTO string
	DestinationTabNoLHPacking     string

	R2AccountID       string
	R2Bucket          string
	R2AccessKeyID     string
	R2SecretAccessKey string
	R2ObjectPrefix    string

	Continuous                  bool
	PollInterval                time.Duration
	DryRun                      bool
	BootstrapProcessExisting    bool
	DropLeadingUnnamedColumn    bool
	SheetsBatchSize             int
	SheetsWriteRetryMaxAttempts int
	SheetsWriteRetryBaseDelay   time.Duration
	SheetsWriteRetryMaxDelay    time.Duration
	TempDir                     string

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
	SummarySecondEnabled   bool
	SummarySecondTab       string
	SummarySecondRanges    []string
	SummaryWaitAfterImport time.Duration
	SummaryStabilityWait   time.Duration
	SummaryStabilityRuns   int
	SummaryRenderMode      string
	SummaryRenderScale     int
	SummaryAutoFitColumns  bool
	SummaryPDFDPI          int
	SummaryPDFConverter    string
	SummaryImageMaxWidthPx int
	SummaryImageMaxBase64  int
	SummarySendHTTPTimeout time.Duration
	SummaryTimezone        string
	SummaryLocation        *time.Location
	SummarySyncCell        string
}

type workflowState struct {
	LastProcessedFileID       string `json:"last_processed_file_id,omitempty"`
	LastProcessedFileMD5      string `json:"last_processed_file_md5,omitempty"`
	LastProcessedModifiedTime string `json:"last_processed_modified_time,omitempty"`
	LastProcessedAt           string `json:"last_processed_at,omitempty"`
	LastUploadedObjectKey     string `json:"last_uploaded_object_key,omitempty"`
	LastDestinationSyncHash   string `json:"last_destination_sync_hash,omitempty"`
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
	DestinationSynced bool   `json:"destination_synced,omitempty"`
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
	CSVFilesProcessed   int
	RowsConsolidated    int64
	RowsImported        int64
	ObjectKey           string
	ObjectBytes         int64
	DestinationSynced   bool
	DestinationSyncHash string
	SummaryImageSent    bool
	SummaryStable       bool
	SummaryImageFmt     string
	SummaryImageBytes   int
}

type sheetGridState struct {
	sheetID int64
	rows    int
	cols    int
	loaded  bool
}

type destinationImportTabState struct {
	Name         string
	NextSheetRow int
	PendingRows  [][]string
	GridState    sheetGridState
}

func main() {
	logger := log.New(os.Stdout, "[workflow-drive-csv-consolidation] ", log.LstdFlags|log.Lmsgprefix)

	cfg, err := loadConfig()
	if err != nil {
		logger.Fatalf("config error: %v", err)
	}
	if identityHint, hintErr := googleCredentialsIdentityHint(cfg); hintErr != nil {
		logger.Printf("google credentials identity hint unavailable err=%v", hintErr)
	} else {
		logger.Printf("google credentials identity %s", identityHint)
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
		"destination write targets sheet=%s tabs=%q,%q,%q range=A:%s columns=%d headers=%q",
		cfg.DestinationSheetID,
		cfg.DestinationTabPendingRCV,
		cfg.DestinationTabPackedAnotherTO,
		cfg.DestinationTabNoLHPacking,
		outputEndCol,
		len(selectedOutputHeaders),
		strings.Join(selectedOutputHeaders, ", "),
	)
	if cfg.SummarySendEnabled {
		logger.Printf(
			"summary snapshot enabled mode=%s sheet=%s tab=%q range=%s second_image_enabled=%t second_tab=%q second_ranges=%q sync_cell=%q wait_after_import=%s stability_runs=%d stability_wait=%s render_mode=%s render_scale=%d auto_fit_columns=%t pdf_dpi=%d pdf_converter=%s timezone=%s",
			cfg.SummarySeaTalkMode,
			cfg.SummarySheetID,
			cfg.SummaryTab,
			cfg.SummaryRange,
			cfg.SummarySecondEnabled,
			cfg.SummarySecondTab,
			strings.Join(cfg.SummarySecondRanges, ","),
			cfg.SummarySyncCell,
			cfg.SummaryWaitAfterImport,
			cfg.SummaryStabilityRuns,
			cfg.SummaryStabilityWait,
			cfg.SummaryRenderMode,
			cfg.SummaryRenderScale,
			cfg.SummaryAutoFitColumns,
			cfg.SummaryPDFDPI,
			cfg.SummaryPDFConverter,
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
		state.LastProcessedModifiedTime = latest.ModifiedTime.Format(time.RFC3339Nano)
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
		result, processErr := processZipAndImport(
			ctx,
			cfg,
			sheetsSvc,
			r2Client,
			file,
			zipPath,
			sendSummaryAfterImport,
			state.LastDestinationSyncHash,
		)
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
		state.LastProcessedModifiedTime = file.ModifiedTime.Format(time.RFC3339Nano)
		state.LastProcessedAt = now.Format(time.RFC3339)
		state.LastUploadedObjectKey = result.ObjectKey
		if strings.TrimSpace(result.DestinationSyncHash) != "" {
			state.LastDestinationSyncHash = strings.TrimSpace(result.DestinationSyncHash)
		}
		if err = saveState(cfg.StateFile, *state); err != nil {
			return err
		}
		*stateExists = true

		logger.Printf(
			"processed file_id=%s file_name=%q csv_files=%d rows_consolidated=%d rows_imported=%d object_key=%q bytes=%d destination_synced=%t summary_image_sent=%t summary_stable=%t summary_fmt=%q summary_bytes=%d dry_run=%t",
			file.ID,
			file.Name,
			result.CSVFilesProcessed,
			result.RowsConsolidated,
			result.RowsImported,
			result.ObjectKey,
			result.ObjectBytes,
			result.DestinationSynced,
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
	status.DestinationSynced = lastProcessedCycle.DestinationSynced
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
	lastDestinationSyncHash string,
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
	remarkIdx := -1
	receiveStatusIdx := -1
	receiverTypeIdx := -1
	currentStationIdx := -1
	pendingRCVRows := make([][]string, 0, cfg.SheetsBatchSize)
	packedAnotherTORows := make([][]string, 0, cfg.SheetsBatchSize)
	noLHPackingRows := make([][]string, 0, cfg.SheetsBatchSize)

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

			remarkIdx = findIndexByHeader(canonicalHeaderMap, "Remark", -1, len(canonicalHeader))
			receiveStatusIdx = findIndexByHeader(canonicalHeaderMap, "Receive Status", -1, len(canonicalHeader))
			receiverTypeIdx = findIndexByHeader(canonicalHeaderMap, "Receiver Type", -1, len(canonicalHeader))
			currentStationIdx = findIndexByHeader(canonicalHeaderMap, "Current Station", -1, len(canonicalHeader))
			if remarkIdx < 0 {
				entryReader.Close()
				return result, errors.New(`required column "Remark" not found in canonical CSV header`)
			}
			if receiveStatusIdx < 0 {
				entryReader.Close()
				return result, errors.New(`required column "Receive Status" not found in canonical CSV header`)
			}
			if receiverTypeIdx < 0 {
				entryReader.Close()
				return result, errors.New(`required column "Receiver Type" not found in canonical CSV header`)
			}
			if currentStationIdx < 0 {
				entryReader.Close()
				return result, errors.New(`required column "Current Station" not found in canonical CSV header`)
			}

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
			if !rowMatchesFilters(canonicalRow, receiverTypeIdx, currentStationIdx) {
				continue
			}

			pendingReceive := shouldImportPendingReceive(canonicalRow, receiveStatusIdx)
			packedInAnotherTO := shouldImportPackedInAnotherTO(canonicalRow, remarkIdx)
			noLHPacking := shouldImportNoLHPacking(canonicalRow, remarkIdx)

			if pendingReceive || packedInAnotherTO || noLHPacking {
				picked := pickColumns(canonicalRow, selectorIndexes)
				if pendingReceive {
					result.RowsImported++
					if !cfg.DryRun {
						pendingRCVRows = append(pendingRCVRows, picked)
					}
				}
				if packedInAnotherTO {
					result.RowsImported++
					if !cfg.DryRun {
						packedAnotherTORows = append(packedAnotherTORows, picked)
					}
				}
				if noLHPacking {
					result.RowsImported++
					if !cfg.DryRun {
						noLHPackingRows = append(noLHPackingRows, picked)
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

	if !cfg.DryRun {
		snapshotHash, hashErr := buildDestinationSnapshotHash(
			pendingRCVRows,
			packedAnotherTORows,
			noLHPackingRows,
		)
		if hashErr != nil {
			return result, hashErr
		}
		result.DestinationSyncHash = snapshotHash
		if snapshotHash != "" && strings.TrimSpace(snapshotHash) == strings.TrimSpace(lastDestinationSyncHash) {
			result.DestinationSynced = false
		} else {
			importTabStates := []destinationImportTabState{
				{
					Name:         cfg.DestinationTabPendingRCV,
					NextSheetRow: 2,
					PendingRows:  make([][]string, 0, cfg.SheetsBatchSize),
				},
				{
					Name:         cfg.DestinationTabPackedAnotherTO,
					NextSheetRow: 2,
					PendingRows:  make([][]string, 0, cfg.SheetsBatchSize),
				},
				{
					Name:         cfg.DestinationTabNoLHPacking,
					NextSheetRow: 2,
					PendingRows:  make([][]string, 0, cfg.SheetsBatchSize),
				},
			}
			for i := range importTabStates {
				tabState := &importTabStates[i]
				if err = loadSheetGridState(ctx, sheetsSvc, cfg.DestinationSheetID, tabState.Name, &tabState.GridState); err != nil {
					return result, err
				}
				if err = clearDestinationSheet(ctx, sheetsSvc, cfg, cfg.DestinationSheetID, tabState.Name); err != nil {
					return result, err
				}
				if err = writeHeaderRow(ctx, sheetsSvc, cfg, cfg.DestinationSheetID, tabState.Name, selectedOutputHeaders); err != nil {
					return result, err
				}
			}

			// Import priority: pending_rcv -> packed_in_another_to.
			// Summary sync/snapshot happens before no_lhpacking import.
			if err = importRowsToDestinationTab(ctx, sheetsSvc, cfg, cfg.DestinationSheetID, &importTabStates[0], pendingRCVRows); err != nil {
				return result, err
			}
			if err = importRowsToDestinationTab(ctx, sheetsSvc, cfg, cfg.DestinationSheetID, &importTabStates[1], packedAnotherTORows); err != nil {
				return result, err
			}
			if err = updateSummarySyncCell(ctx, cfg, sheetsSvc); err != nil {
				return result, err
			}
			// sendSummarySnapshotToSeaTalk includes SummaryWaitAfterImport delay.
			if sendSummaryAfterImport && cfg.SummarySendEnabled {
				summaryResult, summaryErr := sendSummarySnapshotToSeaTalk(ctx, cfg, sheetsSvc)
				if summaryErr != nil {
					return result, summaryErr
				}
				result.SummaryImageSent = true
				result.SummaryStable = summaryResult.Stable
				result.SummaryImageFmt = summaryResult.Format
				result.SummaryImageBytes = summaryResult.RawBytes
			}
			if err = importRowsToDestinationTab(ctx, sheetsSvc, cfg, cfg.DestinationSheetID, &importTabStates[2], noLHPackingRows); err != nil {
				return result, err
			}
			result.DestinationSynced = true
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
	summarySecondEnabled, err := getBoolEnv("WF21_SUMMARY_SECOND_IMAGE_ENABLED", true)
	if err != nil {
		return workflowConfig{}, err
	}
	summaryAutoFitColumns, err := getBoolEnv("WF21_SUMMARY_AUTO_FIT_COLUMNS", defaultSummaryAutoFitColumns)
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
	botConfigSheetID := strings.TrimSpace(os.Getenv("BOT_CONFIG_SHEET_ID"))
	botConfigTab := strings.TrimSpace(os.Getenv("BOT_CONFIG_TAB"))
	if botConfigSheetID != "" {
		botCfgSvc, svcErr := newReadonlySheetsService(context.Background(), credsFile, credsJSON)
		if svcErr != nil {
			return workflowConfig{}, fmt.Errorf("create sheets service for bot_config: %w", svcErr)
		}
		rows, loadErr := botconfig.LoadRowsFromSheet(context.Background(), botCfgSvc, botConfigSheetID, botConfigTab)
		if loadErr != nil {
			return workflowConfig{}, loadErr
		}
		row, resolveErr := botconfig.ResolveForWorkflow(rows, "wf21")
		if resolveErr != nil {
			return workflowConfig{}, resolveErr
		}
		if validateErr := botconfig.ValidateResolvedRow(row); validateErr != nil {
			return workflowConfig{}, validateErr
		}
		summarySeaTalkMode = row.Mode
		summarySeaTalkGroupID = row.TargetGroup
		summarySeaTalkAppID = row.AppID
		summarySeaTalkSecret = row.AppSecret
		summaryWebhookURL = row.WebhookURL
	}
	summaryTimezone := firstNonEmpty(
		strings.TrimSpace(os.Getenv("WF21_TIMEZONE")),
		strings.TrimSpace(os.Getenv("WF2_TIMEZONE")),
		defaultSummaryTimezone,
	)
	summarySecondTab := strings.TrimSpace(firstNonEmpty(
		os.Getenv("WF21_SUMMARY_SECOND_TAB"),
		defaultSummarySecondTab,
	))
	summarySecondRanges, err := parseSummaryRangeList(strings.TrimSpace(firstNonEmpty(
		os.Getenv("WF21_SUMMARY_SECOND_RANGES"),
		defaultSummarySecondRanges,
	)))
	if err != nil {
		return workflowConfig{}, fmt.Errorf("invalid WF21_SUMMARY_SECOND_RANGES: %w", err)
	}
	summaryLocation, err := time.LoadLocation(summaryTimezone)
	if err != nil {
		return workflowConfig{}, fmt.Errorf("invalid WF21_TIMEZONE %q: %w", summaryTimezone, err)
	}

	cfg := workflowConfig{
		GoogleCredentialsFile: credsFile,
		GoogleCredentialsJSON: credsJSON,
		DriveParentFolderID:   firstNonEmpty(strings.TrimSpace(os.Getenv("WF21_DRIVE_PARENT_FOLDER_ID")), defaultDriveParentFolderID),
		DestinationSheetID:    firstNonEmpty(strings.TrimSpace(os.Getenv("WF21_DESTINATION_SHEET_ID")), defaultDestinationSheetID),
		DestinationTabPendingRCV: firstNonEmpty(
			strings.TrimSpace(os.Getenv("WF21_DESTINATION_TAB_PENDING_RCV")),
			defaultDestinationTabPendingRCV,
		),
		DestinationTabPackedAnotherTO: firstNonEmpty(
			strings.TrimSpace(os.Getenv("WF21_DESTINATION_TAB_PACKED_IN_ANOTHER_TO")),
			defaultDestinationTabPackedAnotherTO,
		),
		DestinationTabNoLHPacking: firstNonEmpty(
			strings.TrimSpace(os.Getenv("WF21_DESTINATION_TAB_NO_LHPACKING")),
			defaultDestinationTabNoLHPacking,
		),
		R2AccountID:                 r2AccountID,
		R2Bucket:                    r2Bucket,
		R2AccessKeyID:               r2AccessKeyID,
		R2SecretAccessKey:           r2SecretAccessKey,
		R2ObjectPrefix:              firstNonEmpty(strings.TrimSpace(os.Getenv("WF21_R2_OBJECT_PREFIX")), defaultR2ObjectPrefx),
		Continuous:                  continuous,
		PollInterval:                getDurationSeconds("WF21_POLL_INTERVAL_SECONDS", int(defaultPollInterval/time.Second)),
		DryRun:                      dryRun,
		BootstrapProcessExisting:    bootstrapProcessExisting,
		DropLeadingUnnamedColumn:    dropLeadingUnnamed,
		SheetsBatchSize:             getIntEnv("WF21_SHEETS_BATCH_SIZE", defaultSheetsBatch),
		SheetsWriteRetryMaxAttempts: getIntEnv("WF21_SHEETS_WRITE_RETRY_MAX_ATTEMPTS", defaultSheetsWriteRetryMaxAttempts),
		SheetsWriteRetryBaseDelay:   getDurationMillis("WF21_SHEETS_WRITE_RETRY_BASE_MS", int(defaultSheetsWriteRetryBaseDelay/time.Millisecond)),
		SheetsWriteRetryMaxDelay:    getDurationMillis("WF21_SHEETS_WRITE_RETRY_MAX_MS", int(defaultSheetsWriteRetryMaxDelay/time.Millisecond)),
		TempDir:                     strings.TrimSpace(os.Getenv("WF21_TEMP_DIR")),
		StateFile:                   firstNonEmpty(strings.TrimSpace(os.Getenv("WF21_STATE_FILE")), defaultStateFile),
		StatusFile:                  statusFile,
		EnableHealthServer:          enableHealthServer,
		HealthListenAddr:            normalizeListenAddr(healthPort),
		SummarySendEnabled:          summarySendEnabled,
		SummarySeaTalkMode:          summarySeaTalkMode,
		SummaryWebhookURL:           summaryWebhookURL,
		SummarySeaTalkAppID:         summarySeaTalkAppID,
		SummarySeaTalkSecret:        summarySeaTalkSecret,
		SummarySeaTalkBaseURL:       summarySeaTalkBaseURL,
		SummarySeaTalkGroupID:       summarySeaTalkGroupID,
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
		SummarySecondEnabled:   summarySecondEnabled,
		SummarySecondTab:       summarySecondTab,
		SummarySecondRanges:    summarySecondRanges,
		SummaryWaitAfterImport: getDurationSeconds("WF21_SUMMARY_WAIT_SECONDS", int(defaultSummaryWaitAfterImport/time.Second)),
		SummaryStabilityWait:   getDurationSeconds("WF21_SUMMARY_STABILITY_WAIT_SECONDS", int(defaultSummaryStabilityWait/time.Second)),
		SummaryStabilityRuns:   getIntEnv("WF21_SUMMARY_STABILITY_RUNS", defaultSummaryStabilityRuns),
		SummaryRenderMode:      strings.ToLower(strings.TrimSpace(firstNonEmpty(os.Getenv("WF21_SUMMARY_RENDER_MODE"), defaultSummaryRenderMode))),
		SummaryRenderScale:     getIntEnv("WF21_SUMMARY_RENDER_SCALE", defaultSummaryRenderScale),
		SummaryAutoFitColumns:  summaryAutoFitColumns,
		SummaryPDFDPI:          getIntEnv("WF21_SUMMARY_PDF_DPI", defaultSummaryPDFDPI),
		SummaryPDFConverter:    strings.ToLower(strings.TrimSpace(firstNonEmpty(os.Getenv("WF21_SUMMARY_PDF_CONVERTER"), defaultSummaryPDFConverter))),
		SummaryImageMaxWidthPx: getIntEnv("WF21_SUMMARY_IMAGE_MAX_WIDTH_PX", defaultSummaryImageMaxWidthPx),
		SummaryImageMaxBase64:  getIntEnv("WF21_SUMMARY_IMAGE_MAX_BASE64_BYTES", defaultSummaryImageMaxBase64),
		SummarySendHTTPTimeout: getDurationSeconds("WF21_SUMMARY_HTTP_TIMEOUT_SECONDS", int(defaultSummaryHTTPTimeout/time.Second)),
		SummaryTimezone:        summaryTimezone,
		SummaryLocation:        summaryLocation,
		SummarySyncCell: firstNonEmpty(
			strings.TrimSpace(os.Getenv("WF21_SUMMARY_SYNC_CELL")),
			defaultSummarySyncCell,
		),
	}

	if cfg.PollInterval < 3*time.Second {
		cfg.PollInterval = 3 * time.Second
	}
	if cfg.SheetsBatchSize < 100 {
		cfg.SheetsBatchSize = 100
	}
	if cfg.SheetsWriteRetryMaxAttempts < 1 {
		cfg.SheetsWriteRetryMaxAttempts = 1
	}
	if cfg.SheetsWriteRetryMaxAttempts > 10 {
		cfg.SheetsWriteRetryMaxAttempts = 10
	}
	if cfg.SheetsWriteRetryBaseDelay < 100*time.Millisecond {
		cfg.SheetsWriteRetryBaseDelay = 100 * time.Millisecond
	}
	if cfg.SheetsWriteRetryMaxDelay < cfg.SheetsWriteRetryBaseDelay {
		cfg.SheetsWriteRetryMaxDelay = cfg.SheetsWriteRetryBaseDelay
	}
	if cfg.SheetsWriteRetryMaxDelay > 60*time.Second {
		cfg.SheetsWriteRetryMaxDelay = 60 * time.Second
	}
	if strings.TrimSpace(cfg.DestinationTabPendingRCV) == "" {
		return workflowConfig{}, errors.New("WF21_DESTINATION_TAB_PENDING_RCV is required")
	}
	if strings.TrimSpace(cfg.DestinationTabPackedAnotherTO) == "" {
		return workflowConfig{}, errors.New("WF21_DESTINATION_TAB_PACKED_IN_ANOTHER_TO is required")
	}
	if strings.TrimSpace(cfg.DestinationTabNoLHPacking) == "" {
		return workflowConfig{}, errors.New("WF21_DESTINATION_TAB_NO_LHPACKING is required")
	}
	if strings.EqualFold(cfg.DestinationTabPendingRCV, cfg.DestinationTabPackedAnotherTO) ||
		strings.EqualFold(cfg.DestinationTabPendingRCV, cfg.DestinationTabNoLHPacking) ||
		strings.EqualFold(cfg.DestinationTabPackedAnotherTO, cfg.DestinationTabNoLHPacking) {
		return workflowConfig{}, errors.New("WF21 destination tab names must be unique across pending_rcv/packed_in_another_to/no_lhpacking")
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
	switch cfg.SummaryRenderMode {
	case "styled", "pdf_png":
	default:
		return workflowConfig{}, fmt.Errorf("WF21_SUMMARY_RENDER_MODE must be one of: styled, pdf_png (got %q)", cfg.SummaryRenderMode)
	}
	if cfg.SummaryRenderScale < 1 {
		cfg.SummaryRenderScale = 1
	}
	if cfg.SummaryRenderScale > 4 {
		cfg.SummaryRenderScale = 4
	}
	if cfg.SummaryPDFDPI < 72 {
		cfg.SummaryPDFDPI = 72
	}
	if cfg.SummaryPDFDPI > 600 {
		cfg.SummaryPDFDPI = 600
	}
	switch cfg.SummaryPDFConverter {
	case "", "auto":
		cfg.SummaryPDFConverter = "auto"
	case "pdftoppm", "magick":
	default:
		return workflowConfig{}, fmt.Errorf("WF21_SUMMARY_PDF_CONVERTER must be one of: auto, pdftoppm, magick (got %q)", cfg.SummaryPDFConverter)
	}
	if cfg.SummaryImageMaxWidthPx < 1200 {
		cfg.SummaryImageMaxWidthPx = 1200
	}
	if cfg.SummaryImageMaxBase64 < 512*1024 {
		cfg.SummaryImageMaxBase64 = defaultSummaryImageMaxBase64
	}
	if cfg.SummarySendEnabled && !cfg.DryRun && strings.TrimSpace(cfg.SummarySyncCell) == "" {
		return workflowConfig{}, errors.New("WF21_SUMMARY_SYNC_CELL is required when summary send is enabled")
	}
	if cfg.SummarySecondEnabled {
		if strings.TrimSpace(cfg.SummarySecondTab) == "" {
			return workflowConfig{}, errors.New("WF21_SUMMARY_SECOND_TAB is required when WF21_SUMMARY_SECOND_IMAGE_ENABLED=true")
		}
		if len(cfg.SummarySecondRanges) == 0 {
			return workflowConfig{}, errors.New("WF21_SUMMARY_SECOND_RANGES is required when WF21_SUMMARY_SECOND_IMAGE_ENABLED=true")
		}
	}
	return cfg, nil
}

func parseSummaryRangeList(raw string) ([]string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, nil
	}

	parts := strings.Split(trimmed, ",")
	ranges := make([]string, 0, len(parts))
	for _, part := range parts {
		token := strings.TrimSpace(part)
		if token == "" {
			continue
		}
		// Allow optional tab prefix on first token (for convenience):
		// "[SOC5] SOCPacked_Dashboard!A1:U9, B142:T167"
		if bang := strings.Index(token, "!"); bang >= 0 {
			token = strings.TrimSpace(token[bang+1:])
		}
		if _, err := parseA1Range(token); err != nil {
			return nil, err
		}
		ranges = append(ranges, token)
	}
	return ranges, nil
}

func googleCredentialsIdentityHint(cfg workflowConfig) (string, error) {
	raw := strings.TrimSpace(cfg.GoogleCredentialsJSON)
	source := ""
	if raw != "" {
		source = "WF21_GOOGLE_CREDENTIALS_JSON"
	} else if strings.TrimSpace(cfg.GoogleCredentialsFile) != "" {
		fileRaw, err := os.ReadFile(cfg.GoogleCredentialsFile)
		if err != nil {
			return "", fmt.Errorf("read credentials file: %w", err)
		}
		raw = strings.TrimSpace(string(fileRaw))
		source = "file:" + filepath.Base(cfg.GoogleCredentialsFile)
	} else {
		return "", errors.New("no google credentials source configured")
	}

	var parsed struct {
		Type        string `json:"type"`
		ProjectID   string `json:"project_id"`
		ClientEmail string `json:"client_email"`
	}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return "", fmt.Errorf("parse credentials json: %w", err)
	}

	credType := firstNonEmpty(strings.TrimSpace(parsed.Type), "unknown")
	project := firstNonEmpty(strings.TrimSpace(parsed.ProjectID), "unknown")
	clientEmail := strings.TrimSpace(parsed.ClientEmail)
	if clientEmail == "" {
		return fmt.Sprintf(
			"source=%s type=%s project=%s client_email=missing",
			source,
			credType,
			maskIdentityToken(project, 4),
		), nil
	}

	return fmt.Sprintf(
		"source=%s type=%s project=%s client_email=%s",
		source,
		credType,
		maskIdentityToken(project, 4),
		maskServiceAccountEmail(clientEmail),
	), nil
}

func maskServiceAccountEmail(email string) string {
	trimmed := strings.TrimSpace(strings.ToLower(email))
	parts := strings.Split(trimmed, "@")
	if len(parts) != 2 {
		return maskIdentityToken(trimmed, 6)
	}

	localPart := maskIdentityToken(parts[0], 6)
	domainPart := maskDomain(parts[1])
	return localPart + "@" + domainPart
}

func maskDomain(domain string) string {
	labels := strings.Split(strings.TrimSpace(strings.ToLower(domain)), ".")
	if len(labels) == 0 {
		return ""
	}
	labels[0] = maskIdentityToken(labels[0], 4)
	return strings.Join(labels, ".")
}

func maskIdentityToken(v string, keep int) string {
	trimmed := strings.TrimSpace(v)
	if trimmed == "" {
		return ""
	}
	runes := []rune(trimmed)
	if keep < 0 {
		keep = 0
	}
	if len(runes) <= keep {
		return trimmed
	}
	return string(runes[:keep]) + "***"
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

func buildDestinationSnapshotHash(
	pendingRCVRows [][]string,
	packedAnotherTORows [][]string,
	noLHPackingRows [][]string,
) (string, error) {
	snapshot := struct {
		Headers             []string   `json:"headers"`
		PendingRCVRows      [][]string `json:"pending_rcv_rows"`
		PackedAnotherTORows [][]string `json:"packed_in_another_to_rows"`
		NoLHPackingRows     [][]string `json:"no_lhpacking_rows"`
	}{
		Headers:             selectedOutputHeaders,
		PendingRCVRows:      pendingRCVRows,
		PackedAnotherTORows: packedAnotherTORows,
		NoLHPackingRows:     noLHPackingRows,
	}
	payload, err := json.Marshal(snapshot)
	if err != nil {
		return "", fmt.Errorf("encode destination snapshot hash payload: %w", err)
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

func clearDestinationSheet(ctx context.Context, sheetsSvc *sheets.Service, cfg workflowConfig, sheetID, tab string) error {
	endCol := columnNameFromIndex(len(selectedOutputHeaders))
	err := runSheetsWriteWithRetry(ctx, cfg, "clear_destination_sheet", func() error {
		_, clearErr := sheetsSvc.Spreadsheets.Values.Clear(sheetID, fmt.Sprintf("%s!A:%s", tab, endCol), &sheets.ClearValuesRequest{}).Context(ctx).Do()
		return clearErr
	})
	if err != nil {
		return fmt.Errorf("clear destination sheet: %w", err)
	}
	return nil
}

func writeHeaderRow(ctx context.Context, sheetsSvc *sheets.Service, cfg workflowConfig, sheetID, tab string, headers []string) error {
	values := []interface{}{}
	for _, h := range headers {
		values = append(values, h)
	}
	vr := &sheets.ValueRange{
		Values: [][]interface{}{values},
	}
	endCol := columnNameFromIndex(len(headers))
	targetRange := fmt.Sprintf("%s!A1:%s1", tab, endCol)
	err := runSheetsWriteWithRetry(ctx, cfg, "write_header_row", func() error {
		_, updateErr := sheetsSvc.Spreadsheets.Values.Update(sheetID, targetRange, vr).
			ValueInputOption("RAW").
			Context(ctx).
			Do()
		return updateErr
	})
	if err != nil {
		return fmt.Errorf("write header row range=%s headers=%d: %w", targetRange, len(headers), err)
	}
	return nil
}

func flushDestinationTabsRows(
	ctx context.Context,
	sheetsSvc *sheets.Service,
	cfg workflowConfig,
	sheetID string,
	tabStates []destinationImportTabState,
) error {
	if len(tabStates) == 0 {
		return nil
	}

	endCol := columnNameFromIndex(len(selectedOutputHeaders))
	pendingIndexes := make([]int, 0, len(tabStates))
	data := make([]*sheets.ValueRange, 0, len(tabStates))

	for idx := range tabStates {
		tabState := &tabStates[idx]
		if len(tabState.PendingRows) == 0 {
			continue
		}

		endRow := tabState.NextSheetRow + len(tabState.PendingRows) - 1
		if err := ensureSheetGridCapacity(
			ctx,
			sheetsSvc,
			cfg,
			sheetID,
			tabState.Name,
			endRow,
			len(selectedOutputHeaders),
			&tabState.GridState,
		); err != nil {
			return err
		}

		payload := make([][]interface{}, 0, len(tabState.PendingRows))
		for _, row := range tabState.PendingRows {
			items := make([]interface{}, 0, len(row))
			for _, val := range row {
				items = append(items, val)
			}
			payload = append(payload, items)
		}

		targetRange := fmt.Sprintf("%s!A%d:%s%d", tabState.Name, tabState.NextSheetRow, endCol, endRow)
		data = append(data, &sheets.ValueRange{
			Range:  targetRange,
			Values: payload,
		})
		pendingIndexes = append(pendingIndexes, idx)
	}

	if len(data) == 0 {
		return nil
	}

	req := &sheets.BatchUpdateValuesRequest{
		ValueInputOption: "RAW",
		Data:             data,
	}
	if err := runSheetsWriteWithRetry(ctx, cfg, "write_rows_batch_update", func() error {
		_, updateErr := sheetsSvc.Spreadsheets.Values.BatchUpdate(sheetID, req).Context(ctx).Do()
		return updateErr
	}); err != nil {
		return fmt.Errorf("write rows batch update ranges=%d: %w", len(data), err)
	}

	for _, idx := range pendingIndexes {
		tabState := &tabStates[idx]
		tabState.NextSheetRow += len(tabState.PendingRows)
		tabState.PendingRows = tabState.PendingRows[:0]
	}
	return nil
}

func importRowsToDestinationTab(
	ctx context.Context,
	sheetsSvc *sheets.Service,
	cfg workflowConfig,
	sheetID string,
	tabState *destinationImportTabState,
	rows [][]string,
) error {
	if tabState == nil || len(rows) == 0 {
		return nil
	}
	batchSize := maxInt(cfg.SheetsBatchSize, 1)
	batchStates := []destinationImportTabState{*tabState}
	for start := 0; start < len(rows); start += batchSize {
		end := minInt(start+batchSize, len(rows))
		batchStates[0].PendingRows = rows[start:end]
		if err := flushDestinationTabsRows(ctx, sheetsSvc, cfg, sheetID, batchStates); err != nil {
			return err
		}
	}
	*tabState = batchStates[0]
	return nil
}

func runSheetsWriteWithRetry(ctx context.Context, cfg workflowConfig, opName string, op func() error) error {
	attempts := maxInt(cfg.SheetsWriteRetryMaxAttempts, 1)
	delay := cfg.SheetsWriteRetryBaseDelay
	maxDelay := cfg.SheetsWriteRetryMaxDelay
	if delay <= 0 {
		delay = defaultSheetsWriteRetryBaseDelay
	}
	if maxDelay < delay {
		maxDelay = delay
	}

	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		if err := op(); err == nil {
			return nil
		} else {
			lastErr = err
		}
		if !isRetryableSheetsWriteError(lastErr) || attempt == attempts {
			break
		}

		wait := delay
		if wait > maxDelay {
			wait = maxDelay
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("%s canceled while waiting for retry: %w", opName, ctx.Err())
		case <-time.After(wait):
		}

		delay = delay * 2
		if delay > maxDelay {
			delay = maxDelay
		}
	}
	return fmt.Errorf("%s failed after %d attempt(s): %w", opName, attempts, lastErr)
}

func isRetryableSheetsWriteError(err error) bool {
	if err == nil {
		return false
	}

	var gErr *googleapi.Error
	if errors.As(err, &gErr) {
		switch gErr.Code {
		case 429, 500, 502, 503, 504:
			return true
		}
		for _, item := range gErr.Errors {
			reason := strings.ToLower(strings.TrimSpace(item.Reason))
			switch reason {
			case "ratelimitexceeded", "userratelimitexceeded", "backenderror", "internalerror":
				return true
			}
		}
	}

	var netErr net.Error
	if errors.As(err, &netErr) {
		return netErr.Timeout() || netErr.Temporary()
	}

	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	return strings.Contains(msg, "timeout") ||
		strings.Contains(msg, "temporarily unavailable") ||
		strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "broken pipe")
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
	cfg workflowConfig,
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
	err := runSheetsWriteWithRetry(ctx, cfg, "resize_destination_sheet", func() error {
		_, batchErr := sheetsSvc.Spreadsheets.BatchUpdate(spreadsheetID, req).Context(ctx).Do()
		return batchErr
	})
	if err != nil {
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
	lastModified, ok := parseStoredTimestamp(state.LastProcessedModifiedTime)
	if !ok {
		return true
	}
	if !file.ModifiedTime.Equal(lastModified) {
		return true
	}
	return false
}

func selectPendingZipFiles(files []driveZipFile, state workflowState) []driveZipFile {
	if len(files) == 0 {
		return nil
	}

	lastModified, ok := parseStoredTimestamp(state.LastProcessedModifiedTime)
	if !ok {
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

func parseStoredTimestamp(raw string) (time.Time, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return time.Time{}, false
	}
	if ts, err := time.Parse(time.RFC3339Nano, trimmed); err == nil {
		return ts, true
	}
	if ts, err := time.Parse(time.RFC3339, trimmed); err == nil {
		return ts, true
	}
	return time.Time{}, false
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

func shouldImportPendingReceive(row []string, receiveStatusIdx int) bool {
	if receiveStatusIdx < 0 || receiveStatusIdx >= len(row) {
		return false
	}
	return containsTextFold(row[receiveStatusIdx], "Pending Receive")
}

func shouldImportPackedInAnotherTO(row []string, remarkIdx int) bool {
	if remarkIdx < 0 || remarkIdx >= len(row) {
		return false
	}
	return containsTextFold(row[remarkIdx], "Pack in another TO")
}

func shouldImportNoLHPacking(row []string, remarkIdx int) bool {
	if remarkIdx < 0 || remarkIdx >= len(row) {
		return false
	}
	return containsTextFold(row[remarkIdx], "Receive in")
}

func containsTextFold(value, needle string) bool {
	return strings.Contains(strings.ToLower(strings.TrimSpace(value)), strings.ToLower(strings.TrimSpace(needle)))
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

func getDurationMillis(name string, defaultMillis int) time.Duration {
	millis := getIntEnv(name, defaultMillis)
	if millis <= 0 {
		millis = defaultMillis
	}
	return time.Duration(millis) * time.Millisecond
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

package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"math"
	"regexp"
	"strconv"
	"strings"
	"sync"
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
	"github.com/spxph4227/go-bot-server/internal/seatalk"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
	"google.golang.org/api/sheets/v4"
)

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

const (
	minRenderedFontSize = 4.0
	maxRenderedFontSize = 144.0
)

type summaryImageSendResult struct {
	Stable   bool
	Format   string
	RawBytes int
}

type encodedSummaryImage struct {
	Label      string
	Base64Data string
	Format     string
	RawBytes   int
}

type styledRangeSegment struct {
	Bounds sheetRange
	Data   styledRangeData
}

func sendSummarySnapshotToSeaTalk(ctx context.Context, cfg workflowConfig, sheetsSvc *sheets.Service) (summaryImageSendResult, error) {
	var result summaryImageSendResult

	if cfg.SummaryWaitAfterImport > 0 {
		if err := waitWithContext(ctx, cfg.SummaryWaitAfterImport); err != nil {
			return result, err
		}
	}

	stable, err := waitForStableRangeDigest(ctx, sheetsSvc, cfg.SummarySheetID, cfg.SummaryTab, cfg.SummaryRange, cfg.SummaryStabilityRuns, cfg.SummaryStabilityWait)
	if err != nil {
		return result, err
	}
	result.Stable = stable

	primaryImage, err := buildEncodedSummaryImage(
		ctx,
		sheetsSvc,
		cfg.SummarySheetID,
		cfg.SummaryTab,
		cfg.SummaryRange,
		cfg.SummaryImageMaxWidthPx,
		cfg.SummaryRenderScale,
		cfg.SummaryImageMaxBase64,
		"primary",
	)
	if err != nil {
		return result, err
	}
	images := []encodedSummaryImage{primaryImage}
	result.Format = primaryImage.Format
	result.RawBytes = primaryImage.RawBytes

	if cfg.SummarySecondEnabled {
		secondaryImage, secondaryErr := buildEncodedConnectedSummaryImage(
			ctx,
			sheetsSvc,
			cfg.SummarySheetID,
			cfg.SummarySecondTab,
			cfg.SummarySecondRanges,
			cfg.SummaryImageMaxWidthPx,
			cfg.SummaryRenderScale,
			cfg.SummaryImageMaxBase64,
			"secondary",
		)
		if secondaryErr != nil {
			return result, secondaryErr
		}
		images = append(images, secondaryImage)
	}

	captionTS := currentSummaryCaptionTime(cfg, time.Now())

	if cfg.SummarySeaTalkMode == "webhook" {
		sender := seatalk.NewSystemAccountClient(cfg.SummaryWebhookURL, cfg.SummarySendHTTPTimeout)
		if err = sender.SendTextWithAtAll(ctx, buildSummaryCaption(captionTS), 1, true); err != nil {
			return result, fmt.Errorf("send summary caption to seatalk webhook: %w", err)
		}
		for _, img := range images {
			if err = sender.SendImageBase64(ctx, img.Base64Data); err != nil {
				return result, fmt.Errorf("send %s summary image to seatalk webhook: %w", img.Label, err)
			}
		}
		return result, nil
	}

	sender := seatalk.NewClient(seatalk.ClientConfig{
		AppID:     cfg.SummarySeaTalkAppID,
		AppSecret: cfg.SummarySeaTalkSecret,
		BaseURL:   cfg.SummarySeaTalkBaseURL,
		Timeout:   cfg.SummarySendHTTPTimeout,
	})
	if err = sender.SendTextToGroup(ctx, cfg.SummarySeaTalkGroupID, buildSummaryCaptionForBot(captionTS), 1); err != nil {
		return result, fmt.Errorf("send summary caption to seatalk bot: %w", err)
	}
	for _, img := range images {
		if err = sender.SendImageToGroupBase64(ctx, cfg.SummarySeaTalkGroupID, img.Base64Data); err != nil {
			return result, fmt.Errorf("send %s summary image to seatalk bot: %w", img.Label, err)
		}
	}
	return result, nil
}

func buildEncodedSummaryImage(
	ctx context.Context,
	sheetsSvc *sheets.Service,
	sheetID string,
	tab string,
	captureRange string,
	maxWidthPx int,
	renderScale int,
	maxBase64Bytes int,
	label string,
) (encodedSummaryImage, error) {
	styledRange, err := readStyledRange(ctx, sheetsSvc, sheetID, tab, captureRange)
	if err != nil {
		return encodedSummaryImage{}, err
	}
	pngRaw, err := renderStyledRangeImage(styledRange, maxWidthPx, renderScale)
	if err != nil {
		return encodedSummaryImage{}, err
	}
	base64Image, imageFmt, imageBytes, err := encodeImageWithinLimit(pngRaw, maxBase64Bytes)
	if err != nil {
		return encodedSummaryImage{}, err
	}
	return encodedSummaryImage{
		Label:      label,
		Base64Data: base64Image,
		Format:     imageFmt,
		RawBytes:   imageBytes,
	}, nil
}

func buildEncodedConnectedSummaryImage(
	ctx context.Context,
	sheetsSvc *sheets.Service,
	sheetID string,
	tab string,
	ranges []string,
	maxWidthPx int,
	renderScale int,
	maxBase64Bytes int,
	label string,
) (encodedSummaryImage, error) {
	connectedData, err := readConnectedStyledRanges(ctx, sheetsSvc, sheetID, tab, ranges)
	if err != nil {
		return encodedSummaryImage{}, err
	}
	pngRaw, err := renderStyledRangeImage(connectedData, maxWidthPx, renderScale)
	if err != nil {
		return encodedSummaryImage{}, err
	}
	base64Image, imageFmt, imageBytes, err := encodeImageWithinLimit(pngRaw, maxBase64Bytes)
	if err != nil {
		return encodedSummaryImage{}, err
	}
	return encodedSummaryImage{
		Label:      label,
		Base64Data: base64Image,
		Format:     imageFmt,
		RawBytes:   imageBytes,
	}, nil
}

func readConnectedStyledRanges(
	ctx context.Context,
	svc *sheets.Service,
	sheetID string,
	tab string,
	ranges []string,
) (styledRangeData, error) {
	if len(ranges) == 0 {
		return styledRangeData{}, errors.New("at least one range is required")
	}

	segments := make([]styledRangeSegment, 0, len(ranges))
	for _, rawRange := range ranges {
		rng := strings.TrimSpace(rawRange)
		if rng == "" {
			continue
		}
		parsed, err := parseA1Range(rng)
		if err != nil {
			return styledRangeData{}, err
		}
		data, err := readStyledRange(ctx, svc, sheetID, tab, rng)
		if err != nil {
			return styledRangeData{}, err
		}
		segments = append(segments, styledRangeSegment{
			Bounds: parsed,
			Data:   data,
		})
	}

	return stitchStyledRangeSegments(segments)
}

func stitchStyledRangeSegments(segments []styledRangeSegment) (styledRangeData, error) {
	if len(segments) == 0 {
		return styledRangeData{}, errors.New("no styled range segments provided")
	}

	globalStartCol := segments[0].Bounds.startCol
	globalEndCol := segments[0].Bounds.endCol
	totalRows := 0
	for _, segment := range segments {
		if segment.Data.Rows <= 0 || segment.Data.Cols <= 0 {
			continue
		}
		globalStartCol = minInt(globalStartCol, segment.Bounds.startCol)
		globalEndCol = maxInt(globalEndCol, segment.Bounds.endCol)
		totalRows += segment.Data.Rows
	}
	if totalRows <= 0 {
		return styledRangeData{}, errors.New("no rows available from styled range segments")
	}
	totalCols := globalEndCol - globalStartCol + 1
	if totalCols <= 0 {
		return styledRangeData{}, errors.New("no columns available from styled range segments")
	}

	out := styledRangeData{
		Rows:       totalRows,
		Cols:       totalCols,
		RowHeights: make([]int, totalRows),
		ColWidths:  make([]int, totalCols),
		Cells:      make([][]styledCell, totalRows),
		Merges:     make([]mergeRegion, 0, 32),
	}
	for c := 0; c < totalCols; c++ {
		out.ColWidths[c] = 100
	}
	for r := 0; r < totalRows; r++ {
		out.RowHeights[r] = 24
		out.Cells[r] = make([]styledCell, totalCols)
		for c := 0; c < totalCols; c++ {
			out.Cells[r][c] = defaultStyledCell()
		}
	}

	rowOffset := 0
	for _, segment := range segments {
		if segment.Data.Rows <= 0 || segment.Data.Cols <= 0 {
			continue
		}
		colOffset := segment.Bounds.startCol - globalStartCol
		for c := 0; c < segment.Data.Cols && c < len(segment.Data.ColWidths); c++ {
			globalCol := colOffset + c
			if globalCol >= 0 && globalCol < len(out.ColWidths) {
				out.ColWidths[globalCol] = maxInt(out.ColWidths[globalCol], segment.Data.ColWidths[c])
			}
		}
		for r := 0; r < segment.Data.Rows && rowOffset+r < len(out.RowHeights); r++ {
			if r < len(segment.Data.RowHeights) && segment.Data.RowHeights[r] > 0 {
				out.RowHeights[rowOffset+r] = segment.Data.RowHeights[r]
			}
			for c := 0; c < segment.Data.Cols; c++ {
				globalCol := colOffset + c
				if globalCol >= 0 && globalCol < totalCols {
					out.Cells[rowOffset+r][globalCol] = segment.Data.Cells[r][c]
				}
			}
		}
		for _, merge := range segment.Data.Merges {
			out.Merges = append(out.Merges, mergeRegion{
				StartRow: rowOffset + merge.StartRow,
				EndRow:   rowOffset + merge.EndRow,
				StartCol: colOffset + merge.StartCol,
				EndCol:   colOffset + merge.EndCol,
			})
		}
		rowOffset += segment.Data.Rows
	}

	return out, nil
}

func currentSummaryCaptionTime(cfg workflowConfig, now time.Time) time.Time {
	if cfg.SummaryLocation != nil {
		return now.In(cfg.SummaryLocation)
	}
	tz := strings.TrimSpace(cfg.SummaryTimezone)
	if tz != "" {
		if loc, err := time.LoadLocation(tz); err == nil {
			return now.In(loc)
		}
	}
	return now.Local()
}

func buildSummaryCaption(ts time.Time) string {
	return fmt.Sprintf(
		"@All\nOutbound Pending for Dispatch as of %s. Thanks!",
		ts.Format("3:04 PM Jan-02"),
	)
}

func buildSummaryCaptionForBot(ts time.Time) string {
	return fmt.Sprintf(
		"<mention-tag target=\"seatalk://user?id=0\"/>\nOutbound Pending for Dispatch as of %s. Thanks!",
		ts.Format("3:04 PM Jan-02"),
	)
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

func readStyledRange(ctx context.Context, svc *sheets.Service, sheetID, tab, captureRange string) (styledRangeData, error) {
	parsed, err := parseA1Range(captureRange)
	if err != nil {
		return styledRangeData{}, err
	}

	rangeRef := buildSheetRangeRef(tab, captureRange)
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
	normalizedTab := normalizeSheetTabName(tab)
	for _, sh := range resp.Sheets {
		if sh == nil || sh.Properties == nil {
			continue
		}
		if sh.Properties.Title == normalizedTab {
			targetSheet = sh
			break
		}
	}
	if targetSheet == nil {
		return styledRangeData{}, fmt.Errorf("sheet tab %q not found in response", normalizedTab)
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

	mergeMap := buildMergeIndexMap(data.Rows, data.Cols, data.Merges)
	colWidths := make([]int, data.Cols)
	rowHeights := make([]int, data.Rows)
	for c := 0; c < data.Cols; c++ {
		width := 100
		if c < len(data.ColWidths) && data.ColWidths[c] > 0 {
			width = data.ColWidths[c]
		}
		colWidths[c] = maxInt(width*renderScale, 24)
	}
	colWidths = autoFitColumnWidths(data, colWidths, mergeMap, renderScale)
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

func buildMergeIndexMap(rows, cols int, merges []mergeRegion) [][]int {
	mergeMap := make([][]int, rows)
	for r := 0; r < rows; r++ {
		mergeMap[r] = make([]int, cols)
		for c := 0; c < cols; c++ {
			mergeMap[r][c] = -1
		}
	}
	for idx, merge := range merges {
		for r := maxInt(merge.StartRow, 0); r < minInt(merge.EndRow, rows); r++ {
			for c := maxInt(merge.StartCol, 0); c < minInt(merge.EndCol, cols); c++ {
				mergeMap[r][c] = idx
			}
		}
	}
	return mergeMap
}

func autoFitColumnWidths(data styledRangeData, colWidths []int, mergeMap [][]int, renderScale int) []int {
	if data.Rows <= 0 || data.Cols <= 0 || len(colWidths) == 0 {
		return colWidths
	}

	textScale := float64(maxInt(renderScale, 1))
	paddingX := maxInt(int(math.Round(4*textScale)), 4)
	minColWidth := maxInt(20*maxInt(renderScale, 1), 20)
	maxColWidth := maxInt(900*maxInt(renderScale, 1), 900)
	fitted := make([]int, data.Cols)
	for c := 0; c < data.Cols; c++ {
		fitted[c] = minColWidth
	}

	for r := 0; r < data.Rows; r++ {
		for c := 0; c < data.Cols; c++ {
			mergeIdx := -1
			if r < len(mergeMap) && c < len(mergeMap[r]) {
				mergeIdx = mergeMap[r][c]
			}

			spanStartCol := c
			spanEndCol := c + 1
			if mergeIdx >= 0 {
				merge := data.Merges[mergeIdx]
				if r != merge.StartRow || c != merge.StartCol {
					continue
				}
				spanStartCol = maxInt(merge.StartCol, 0)
				spanEndCol = minInt(merge.EndCol, data.Cols)
				if spanEndCol <= spanStartCol {
					continue
				}
			}

			cell := data.Cells[r][c]
			targetWidth := measuredCellTextWidth(cell, renderScale)
			if targetWidth <= 0 {
				continue
			}
			targetWidth += paddingX * 2
			targetWidth = minInt(targetWidth, maxColWidth)

			colSpan := spanEndCol - spanStartCol
			perColWidth := int(math.Ceil(float64(targetWidth) / float64(colSpan)))
			for col := spanStartCol; col < spanEndCol; col++ {
				if col >= 0 && col < len(fitted) {
					fitted[col] = maxInt(fitted[col], perColWidth)
				}
			}
		}
	}

	for c := 0; c < data.Cols && c < len(colWidths); c++ {
		colWidths[c] = minInt(maxInt(fitted[c], minColWidth), maxColWidth)
	}
	return colWidths
}

func measuredCellTextWidth(cell styledCell, renderScale int) int {
	text := strings.TrimSpace(cell.Text)
	if text == "" {
		return 0
	}

	fontSize := cell.FontSize
	if fontSize <= 0 {
		fontSize = 10
	}
	textScale := float64(maxInt(renderScale, 1))
	face := loadFace(cell.FontFamily, cell.Bold, cell.Italic, fontSize*textScale)
	if face == nil {
		face = basicfont.Face7x13
	}

	if strings.EqualFold(cell.WrapStrategy, "WRAP") {
		longestToken := longestTextToken(text)
		if longestToken == "" {
			return 0
		}
		return font.MeasureString(face, longestToken).Ceil()
	}

	return measureTextMaxLineWidth(face, text)
}

func longestTextToken(text string) string {
	tokens := strings.Fields(strings.TrimSpace(text))
	if len(tokens) == 0 {
		return ""
	}
	longest := tokens[0]
	for _, token := range tokens[1:] {
		if len([]rune(token)) > len([]rune(longest)) {
			longest = token
		}
	}
	return longest
}

func measureTextMaxLineWidth(face font.Face, text string) int {
	lines := strings.Split(text, "\n")
	maxWidth := 0
	for _, line := range lines {
		width := font.MeasureString(face, line).Ceil()
		if width > maxWidth {
			maxWidth = width
		}
	}
	return maxWidth
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
	size = clampRenderedFontSize(size)

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

func clampRenderedFontSize(size float64) float64 {
	if size < minRenderedFontSize {
		return minRenderedFontSize
	}
	if size > maxRenderedFontSize {
		return maxRenderedFontSize
	}
	return size
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

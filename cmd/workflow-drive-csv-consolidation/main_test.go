package main

import (
	"errors"
	"strings"
	"testing"
	"time"

	"golang.org/x/image/font"
	"google.golang.org/api/googleapi"
)

func TestNormalizeHeaderRecordDropsLeadingUnnamed(t *testing.T) {
	input := []string{"", "TO Number", "Receiver Type", "Current Station"}
	header, dropped := normalizeHeaderRecord(input, true)
	if !dropped {
		t.Fatalf("expected leading column to be dropped")
	}
	if len(header) != 3 {
		t.Fatalf("unexpected header length: %d", len(header))
	}
	if header[0] != "TO Number" {
		t.Fatalf("unexpected first header: %q", header[0])
	}
}

func TestBuildCanonicalColumnMapWithShiftedHeader(t *testing.T) {
	canonical := []string{"TO Number", "Receiver Type", "Current Station"}
	shifted := []string{"Hidden", "TO Number", "Receiver Type", "Current Station"}
	m := buildCanonicalColumnMap(canonical, shifted)
	if len(m) != len(canonical) {
		t.Fatalf("unexpected map length: %d", len(m))
	}
	if m[0] != 1 || m[1] != 2 || m[2] != 3 {
		t.Fatalf("unexpected map: %#v", m)
	}
}

func TestRowMatchesFilters(t *testing.T) {
	row := []string{"x", "Station", "y", "SOC 5"}
	if !rowMatchesFilters(row, 1, 3) {
		t.Fatalf("expected row to match filters")
	}
	if rowMatchesFilters(row, 0, 3) {
		t.Fatalf("expected row not to match filters")
	}
}

func TestShouldImportPendingReceive(t *testing.T) {
	row := []string{"abc", "pending receive now"}
	if !shouldImportPendingReceive(row, 1) {
		t.Fatalf("expected Pending Receive matcher to pass")
	}
	if shouldImportPendingReceive(row, 0) {
		t.Fatalf("expected Pending Receive matcher to fail for wrong index")
	}
}

func TestShouldImportPackedInAnotherTOMatchesSingleToken(t *testing.T) {
	rowMatch := []string{"Pack in another TO then anything else"}
	if !shouldImportPackedInAnotherTO(rowMatch, 0) {
		t.Fatalf("expected packed-in-another matcher to pass when token is present")
	}

	rowMissing := []string{"Pack in another HandoverTask only"}
	if shouldImportPackedInAnotherTO(rowMissing, 0) {
		t.Fatalf("expected packed-in-another matcher to fail when token is missing")
	}
}

func TestShouldImportNoLHPacking(t *testing.T) {
	row := []string{"x", "Receive in staging area"}
	if !shouldImportNoLHPacking(row, 1) {
		t.Fatalf("expected no-lhpacking matcher to pass")
	}
	if shouldImportNoLHPacking(row, 0) {
		t.Fatalf("expected no-lhpacking matcher to fail for wrong index")
	}
}

func TestIsRetryableSheetsWriteError(t *testing.T) {
	if !isRetryableSheetsWriteError(&googleapi.Error{Code: 429}) {
		t.Fatalf("expected 429 to be retryable")
	}
	if !isRetryableSheetsWriteError(&googleapi.Error{
		Code: 400,
		Errors: []googleapi.ErrorItem{
			{Reason: "rateLimitExceeded"},
		},
	}) {
		t.Fatalf("expected rateLimitExceeded reason to be retryable")
	}
	if isRetryableSheetsWriteError(&googleapi.Error{Code: 400}) {
		t.Fatalf("expected generic 400 not to be retryable")
	}
	if isRetryableSheetsWriteError(errors.New("validation failed")) {
		t.Fatalf("expected validation error not to be retryable")
	}
}

func TestPickColumnsSupportsDuplicateIndexes(t *testing.T) {
	row := []string{"TO-1", "SPX-1", "Receiver"}
	picked := pickColumns(row, []int{0, 1, 2, 0})
	if len(picked) != 4 {
		t.Fatalf("unexpected picked length: %d", len(picked))
	}
	if picked[0] != "TO-1" || picked[3] != "TO-1" {
		t.Fatalf("expected duplicated TO Number column, got: %#v", picked)
	}
}

func TestSelectPendingZipFilesProcessesAllNewerFiles(t *testing.T) {
	t1 := time.Date(2026, 2, 27, 17, 30, 0, 0, time.UTC)
	t2 := t1.Add(1 * time.Minute)
	t3 := t1.Add(2 * time.Minute)

	files := []driveZipFile{
		{ID: "zip-a", ModifiedTime: t1},
		{ID: "zip-b", ModifiedTime: t2},
		{ID: "zip-c", ModifiedTime: t3},
	}
	state := workflowState{
		LastProcessedFileID:       "zip-a",
		LastProcessedModifiedTime: t1.Format(time.RFC3339),
	}

	pending := selectPendingZipFiles(files, state)
	if len(pending) != 2 {
		t.Fatalf("expected 2 pending files, got %d", len(pending))
	}
	if pending[0].ID != "zip-b" || pending[1].ID != "zip-c" {
		t.Fatalf("unexpected pending order: %#v", pending)
	}
}

func TestSelectPendingZipFilesSameTimestampUsesFileIDCursor(t *testing.T) {
	t1 := time.Date(2026, 2, 27, 17, 30, 0, 0, time.UTC)
	files := []driveZipFile{
		{ID: "zip-a", ModifiedTime: t1},
		{ID: "zip-b", ModifiedTime: t1},
		{ID: "zip-c", ModifiedTime: t1},
	}
	state := workflowState{
		LastProcessedFileID:       "zip-b",
		LastProcessedModifiedTime: t1.Format(time.RFC3339),
	}

	pending := selectPendingZipFiles(files, state)
	if len(pending) != 1 {
		t.Fatalf("expected 1 pending file, got %d", len(pending))
	}
	if pending[0].ID != "zip-c" {
		t.Fatalf("expected zip-c pending, got %#v", pending)
	}
}

func TestSelectPendingZipFilesSubSecondTimestampDoesNotRequeueSameFile(t *testing.T) {
	modified := time.Date(2026, 3, 3, 5, 1, 40, 123000000, time.UTC)
	files := []driveZipFile{
		{ID: "zip-a", ModifiedTime: modified},
	}
	state := workflowState{
		LastProcessedFileID:       "zip-a",
		LastProcessedModifiedTime: modified.Format(time.RFC3339Nano),
	}

	pending := selectPendingZipFiles(files, state)
	if len(pending) != 0 {
		t.Fatalf("expected no pending files, got %#v", pending)
	}
}

func TestBuildSheetRangeRefQuotesSpecialTab(t *testing.T) {
	got := buildSheetRangeRef("[SOC] Backlogs Summary", "B2:Q59")
	want := "'[SOC] Backlogs Summary'!B2:Q59"
	if got != want {
		t.Fatalf("unexpected range ref: got=%q want=%q", got, want)
	}
}

func TestBuildSummaryCaption(t *testing.T) {
	ts := time.Date(2026, 2, 28, 21, 7, 0, 0, time.FixedZone("UTC+8", 8*3600))
	got := buildSummaryCaption(ts)
	want := "@All\nOutbound Pending for Dispatch as of 9:07 PM Feb-28. Thanks!"
	if got != want {
		t.Fatalf("unexpected caption: got=%q want=%q", got, want)
	}
}

func TestBuildSummaryCaptionForBot(t *testing.T) {
	ts := time.Date(2026, 2, 28, 21, 7, 0, 0, time.FixedZone("UTC+8", 8*3600))
	got := buildSummaryCaptionForBot(ts)
	want := "<mention-tag target=\"seatalk://user?id=0\"/>\nOutbound Pending for Dispatch as of 9:07 PM Feb-28. Thanks!"
	if got != want {
		t.Fatalf("unexpected caption: got=%q want=%q", got, want)
	}
}

func TestFormatSummarySyncTimestamp(t *testing.T) {
	ts := time.Date(2026, 3, 4, 17, 9, 45, 0, time.UTC)
	got := formatSummarySyncTimestamp(ts)
	want := "17:09:45 03-04"
	if got != want {
		t.Fatalf("unexpected sync timestamp format: got=%q want=%q", got, want)
	}
}

func TestCurrentSummaryCaptionTimeUsesConfiguredLocation(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Manila")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}
	cfg := workflowConfig{
		SummaryTimezone: "Asia/Manila",
		SummaryLocation: loc,
	}
	nowUTC := time.Date(2026, 3, 1, 10, 29, 0, 0, time.UTC)
	got := currentSummaryCaptionTime(cfg, nowUTC)
	if got.Location().String() != "Asia/Manila" {
		t.Fatalf("unexpected location: %s", got.Location())
	}
	if got.Format("3:04 PM Jan-02") != "6:29 PM Mar-01" {
		t.Fatalf("unexpected converted time: %s", got.Format("3:04 PM Jan-02"))
	}
}

func TestClampRenderedFontSize(t *testing.T) {
	if got := clampRenderedFontSize(1.5); got != minRenderedFontSize {
		t.Fatalf("expected min clamp %v, got %v", minRenderedFontSize, got)
	}
	if got := clampRenderedFontSize(200); got != maxRenderedFontSize {
		t.Fatalf("expected max clamp %v, got %v", maxRenderedFontSize, got)
	}
	if got := clampRenderedFontSize(18); got != 18 {
		t.Fatalf("expected unchanged font size 18, got %v", got)
	}
}

func TestMaskServiceAccountEmail(t *testing.T) {
	input := "outbound-automation@soc-5-operations.iam.gserviceaccount.com"
	got := maskServiceAccountEmail(input)
	if got == input {
		t.Fatalf("expected masked email, got unmasked value")
	}
	if !strings.HasPrefix(got, "outbou***@soc-***") {
		t.Fatalf("unexpected masked format: %q", got)
	}
}

func TestGoogleCredentialsIdentityHintFromJSON(t *testing.T) {
	cfg := workflowConfig{
		GoogleCredentialsJSON: `{
			"type": "service_account",
			"project_id": "soc-5-operations",
			"client_email": "outbound-automation@soc-5-operations.iam.gserviceaccount.com"
		}`,
	}
	hint, err := googleCredentialsIdentityHint(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(hint, "source=WF21_GOOGLE_CREDENTIALS_JSON") {
		t.Fatalf("unexpected source in hint: %q", hint)
	}
	if strings.Contains(hint, "outbound-automation@soc-5-operations.iam.gserviceaccount.com") {
		t.Fatalf("hint leaked full email: %q", hint)
	}
	if !strings.Contains(hint, "client_email=outbou***@soc-***.iam.gserviceaccount.com") {
		t.Fatalf("unexpected masked client_email in hint: %q", hint)
	}
}

func TestFitClipTextFaceShrinksToFit(t *testing.T) {
	cell := defaultStyledCell()
	cell.FontSize = 10
	text := "167,898"
	baseFace := loadFace(cell.FontFamily, cell.Bold, cell.Italic, cell.FontSize)
	if baseFace == nil {
		t.Fatalf("expected base font face")
	}
	baseWidth := font.MeasureString(baseFace, text).Ceil()
	if baseWidth < 12 {
		t.Fatalf("unexpectedly small base width: %d", baseWidth)
	}

	targetWidth := baseWidth - 8
	face, ok := fitClipTextFace(cell, text, 1.0, targetWidth)
	if !ok {
		t.Fatalf("expected text to fit by shrinking font")
	}
	if face == nil {
		t.Fatalf("expected fitted font face")
	}
	fittedWidth := font.MeasureString(face, text).Ceil()
	if fittedWidth > targetWidth {
		t.Fatalf("expected fitted width <= target (%d), got %d", targetWidth, fittedWidth)
	}
}

func TestFitClipTextFaceReturnsFalseWhenTooNarrow(t *testing.T) {
	cell := defaultStyledCell()
	cell.FontSize = 10
	if face, ok := fitClipTextFace(cell, "167,898", 1.0, 2); ok || face != nil {
		t.Fatalf("expected no fit for extremely narrow width")
	}
}

func TestAutoFitColumnWidthsExpandsForLongText(t *testing.T) {
	data := styledRangeData{
		Rows: 1,
		Cols: 1,
		Cells: [][]styledCell{
			{
				{
					Text:         "AUTO FIT THIS LONG TEXT VALUE",
					FontSize:     10,
					WrapStrategy: "CLIP",
				},
			},
		},
	}
	mergeMap := buildMergeIndexMap(data.Rows, data.Cols, data.Merges)
	widths := autoFitColumnWidths(data, []int{40}, mergeMap, 1)
	if widths[0] <= 40 {
		t.Fatalf("expected auto-fit to expand width, got %d", widths[0])
	}
}

func TestAutoFitColumnWidthsDistributesMergedCellWidth(t *testing.T) {
	data := styledRangeData{
		Rows: 1,
		Cols: 2,
		Cells: [][]styledCell{
			{
				{
					Text:         "MERGED CELL LONG TEXT",
					FontSize:     10,
					WrapStrategy: "CLIP",
				},
				defaultStyledCell(),
			},
		},
		Merges: []mergeRegion{
			{
				StartRow: 0,
				EndRow:   1,
				StartCol: 0,
				EndCol:   2,
			},
		},
	}
	mergeMap := buildMergeIndexMap(data.Rows, data.Cols, data.Merges)
	widths := autoFitColumnWidths(data, []int{30, 30}, mergeMap, 1)
	if widths[0] <= 30 || widths[1] <= 30 {
		t.Fatalf("expected merged auto-fit to expand both columns, got %#v", widths)
	}
}

func TestAutoFitColumnWidthsShrinksOverwideColumns(t *testing.T) {
	data := styledRangeData{
		Rows: 1,
		Cols: 2,
		Cells: [][]styledCell{
			{
				{
					Text:         "SOC",
					FontSize:     10,
					WrapStrategy: "CLIP",
				},
				{
					Text:         "12",
					FontSize:     10,
					WrapStrategy: "CLIP",
				},
			},
		},
	}
	mergeMap := buildMergeIndexMap(data.Rows, data.Cols, data.Merges)
	widths := autoFitColumnWidths(data, []int{420, 380}, mergeMap, 1)
	if widths[0] >= 420 || widths[1] >= 380 {
		t.Fatalf("expected auto-fit to shrink overwide columns, got %#v", widths)
	}
}

func TestParseSummaryRangeListWithOptionalTabPrefix(t *testing.T) {
	ranges, err := parseSummaryRangeList("[SOC5] SOCPacked_Dashboard!A1:U9, B142:T167")
	if err != nil {
		t.Fatalf("parseSummaryRangeList error: %v", err)
	}
	if len(ranges) != 2 {
		t.Fatalf("expected 2 ranges, got %d", len(ranges))
	}
	if ranges[0] != "A1:U9" || ranges[1] != "B142:T167" {
		t.Fatalf("unexpected ranges: %#v", ranges)
	}
}

func TestParseSummaryRangeListRejectsInvalidRange(t *testing.T) {
	_, err := parseSummaryRangeList("A1")
	if err == nil {
		t.Fatalf("expected parseSummaryRangeList to fail for invalid A1 range")
	}
}

func TestBuildDestinationSnapshotHashDeterministic(t *testing.T) {
	pending := [][]string{{"A", "B"}, {"C", "D"}}
	packed := [][]string{{"E", "F"}}
	noLH := [][]string{{"G", "H"}}

	first, err := buildDestinationSnapshotHash(pending, packed, noLH)
	if err != nil {
		t.Fatalf("buildDestinationSnapshotHash first error: %v", err)
	}
	second, err := buildDestinationSnapshotHash(pending, packed, noLH)
	if err != nil {
		t.Fatalf("buildDestinationSnapshotHash second error: %v", err)
	}
	if first == "" || second == "" {
		t.Fatalf("expected non-empty hashes")
	}
	if first != second {
		t.Fatalf("expected deterministic hash, got %q != %q", first, second)
	}
}

func TestBuildDestinationSnapshotHashChangesWhenRowsChange(t *testing.T) {
	base, err := buildDestinationSnapshotHash(
		[][]string{{"A", "B"}},
		[][]string{{"E", "F"}},
		[][]string{{"G", "H"}},
	)
	if err != nil {
		t.Fatalf("buildDestinationSnapshotHash base error: %v", err)
	}
	changed, err := buildDestinationSnapshotHash(
		[][]string{{"A", "B"}, {"X", "Y"}},
		[][]string{{"E", "F"}},
		[][]string{{"G", "H"}},
	)
	if err != nil {
		t.Fatalf("buildDestinationSnapshotHash changed error: %v", err)
	}
	if base == changed {
		t.Fatalf("expected hash to change when rows change")
	}
}

func TestStitchStyledRangeSegmentsStacksAndAlignsColumns(t *testing.T) {
	seg1 := styledRangeSegment{
		Bounds: sheetRange{startCol: 1, endCol: 3, startRow: 1, endRow: 1},
		Data: styledRangeData{
			Rows:       1,
			Cols:       3,
			RowHeights: []int{30},
			ColWidths:  []int{120, 130, 140},
			Cells: [][]styledCell{
				{
					{Text: "top-a"},
					{Text: "top-b"},
					{Text: "top-c"},
				},
			},
		},
	}
	seg2 := styledRangeSegment{
		Bounds: sheetRange{startCol: 2, endCol: 4, startRow: 142, endRow: 143},
		Data: styledRangeData{
			Rows:       2,
			Cols:       3,
			RowHeights: []int{24, 26},
			ColWidths:  []int{150, 160, 170},
			Cells: [][]styledCell{
				{
					{Text: "body-b1"},
					{Text: "body-c1"},
					{Text: "body-d1"},
				},
				{
					{Text: "body-b2"},
					{Text: "body-c2"},
					{Text: "body-d2"},
				},
			},
			Merges: []mergeRegion{
				{StartRow: 0, EndRow: 2, StartCol: 0, EndCol: 2},
			},
		},
	}

	got, err := stitchStyledRangeSegments([]styledRangeSegment{seg1, seg2})
	if err != nil {
		t.Fatalf("stitchStyledRangeSegments error: %v", err)
	}
	if got.Rows != 3 || got.Cols != 4 {
		t.Fatalf("unexpected stitched dimensions rows=%d cols=%d", got.Rows, got.Cols)
	}
	if got.ColWidths[0] != 120 || got.ColWidths[1] != 150 || got.ColWidths[2] != 160 || got.ColWidths[3] != 170 {
		t.Fatalf("unexpected stitched column widths: %#v", got.ColWidths)
	}
	if got.Cells[0][0].Text != "top-a" || got.Cells[1][1].Text != "body-b1" || got.Cells[2][3].Text != "body-d2" {
		t.Fatalf("unexpected stitched cell mapping: row0col0=%q row1col1=%q row2col3=%q", got.Cells[0][0].Text, got.Cells[1][1].Text, got.Cells[2][3].Text)
	}
	if len(got.Merges) != 1 {
		t.Fatalf("expected 1 stitched merge, got %d", len(got.Merges))
	}
	if got.Merges[0].StartRow != 1 || got.Merges[0].EndRow != 3 || got.Merges[0].StartCol != 1 || got.Merges[0].EndCol != 3 {
		t.Fatalf("unexpected stitched merge: %#v", got.Merges[0])
	}
}

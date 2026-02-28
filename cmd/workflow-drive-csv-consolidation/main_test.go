package main

import (
	"testing"
	"time"
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
	want := "Outbound Pending for Dispatch as of [9:07 PM Feb-28 (local)]"
	if got != want {
		t.Fatalf("unexpected caption: got=%q want=%q", got, want)
	}
}

func TestBuildSummaryCaptionForBot(t *testing.T) {
	ts := time.Date(2026, 2, 28, 21, 7, 0, 0, time.FixedZone("UTC+8", 8*3600))
	got := buildSummaryCaptionForBot(ts)
	want := "<mention-tag target=\"seatalk://user?id=0\"/> Outbound Pending for Dispatch as of [9:07 PM Feb-28 (local)]"
	if got != want {
		t.Fatalf("unexpected caption: got=%q want=%q", got, want)
	}
}

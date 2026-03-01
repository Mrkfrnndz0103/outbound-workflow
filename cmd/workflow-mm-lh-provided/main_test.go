package main

import (
	"context"
	"io"
	"log"
	"net/http"
	"testing"
	"time"
)

func TestParseRow(t *testing.T) {
	row := parseRow([]interface{}{
		"NEW", "2026-02-20 10:00", "MNL", "10W", "OPS", "", "25", "LH-A", "2026-02-20 10:30", "", "", "", "Dock 11",
	}, 5654)

	if row.RowNumber != 5654 {
		t.Fatalf("unexpected row number: %d", row.RowNumber)
	}
	if row.Cluster != "MNL" {
		t.Fatalf("unexpected cluster: %q", row.Cluster)
	}
	if row.PlateNumber != "" {
		t.Fatalf("expected empty plate number, got %q", row.PlateNumber)
	}
	if row.DockLabel != "Dock 11" {
		t.Fatalf("unexpected dock label: %q", row.DockLabel)
	}
}

func TestBuildMessage(t *testing.T) {
	row := sheetRow{
		RequestTime:       "2026-02-20 10:00",
		Cluster:           "Taytay Hub",
		PlateNumber:       "ABC1234",
		FleetSizeProvided: "6WH",
		LHType:            "Drylease",
		ProvideTime:       "2026-02-20 10:30",
		RequestedBy:       "Joan",
		DockLabel:         "Dock 11",
	}

	got := buildMessage(row)
	want := "<mention-tag target=\"seatalk://user?id=0\"/> For Docking\n\n      **Taytay Hub - Dock 11**\n      **Plate #: ABC1234**\n      6WH - Drylease\n      pvd_tme: 10:30:00 Feb-20"
	if got != want {
		t.Fatalf("unexpected message:\nwant: %q\n got: %q", want, got)
	}
}

func TestHasRequiredMessageFields(t *testing.T) {
	valid := sheetRow{
		RequestTime:       "rq",
		Cluster:           "cluster",
		FleetSizeProvided: "10",
		LHType:            "lh",
		ProvideTime:       "pv",
	}
	if !valid.hasRequiredMessageFields() {
		t.Fatalf("expected required fields to be valid")
	}

	invalid := valid
	invalid.ProvideTime = ""
	if invalid.hasRequiredMessageFields() {
		t.Fatalf("expected required fields to be invalid")
	}
}

func TestProvideTimeGroupingIgnoresSeconds(t *testing.T) {
	first, ok := parseProvideTime("2/25/2026 12:25:06")
	if !ok {
		t.Fatalf("expected first provide time to parse")
	}
	second, ok := parseProvideTime("2/25/2026 12:25:22")
	if !ok {
		t.Fatalf("expected second provide time to parse")
	}

	if first.Truncate(time.Minute) != second.Truncate(time.Minute) {
		t.Fatalf("expected times to match by minute")
	}
}

func TestBuildMergedMessage(t *testing.T) {
	rows := []sheetRow{
		{
			Cluster:           "SOC 11,SOC 8",
			DockLabel:         "Dock 68",
			PlateNumber:       "CBN 6469",
			FleetSizeProvided: "6WH",
			LHType:            "Wetlease",
		},
		{
			Cluster:           "Gumaca Hub,San Narciso Hub,San Andres Hub",
			DockLabel:         "Dock 75",
			PlateNumber:       "CCK 4754",
			FleetSizeProvided: "6WH",
			LHType:            "Wetlease",
		},
		{
			Cluster:           "SOC 4,SOC 6",
			DockLabel:         "Dock 59",
			PlateNumber:       "NAN 3523",
			FleetSizeProvided: "10WH",
			LHType:            "Wetlease",
		},
	}

	provideTS, ok := parseProvideTime("2/25/2026 12:25:08")
	if !ok {
		t.Fatalf("expected provide time to parse")
	}

	got := buildMergedMessage(rows, formatProvideTimeMinute(provideTS.Truncate(time.Minute)))
	want := "<mention-tag target=\"seatalk://user?id=0\"/> For Docking\n\n" +
		"      **SOC 11,SOC 8 - Dock 68**\n" +
		"      **Plate_#: CBN 6469**\n" +
		"      6WH-Wetlease\n\n" +
		"      **Gumaca Hub,San Narciso Hub,San Andres Hub - Dock 75**\n" +
		"      **Plate_#: CCK 4754**\n" +
		"      6WH-Wetlease\n\n" +
		"      **SOC 4,SOC 6 - Dock 59**\n" +
		"      **Plate_#: NAN 3523**\n" +
		"      10WH-Wetlease\n\n" +
		"Provided Time: 2/25/2026 12:25 PM"
	if got != want {
		t.Fatalf("unexpected merged message:\nwant: %q\n got: %q", want, got)
	}
}

func TestIsDoubleRequestText(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"DOUBLE REQUEST", true},
		{"Double", true},
		{"double request", true},
		{"  double   request  ", true},
		{"PLEASE DOUBLE", true},
		{"NIE 1506", false},
	}

	for _, tc := range cases {
		got := isDoubleRequestText(tc.in)
		if got != tc.want {
			t.Fatalf("isDoubleRequestText(%q)=%v want=%v", tc.in, got, tc.want)
		}
	}
}

func TestBuildDoubleRequestMessage(t *testing.T) {
	row := sheetRow{
		Cluster:   "Lemery Hub,Taal Hub",
		DockLabel: "Dock 40",
	}

	got := buildDoubleRequestMessage(row)
	want := "Double Request!\nLemery Hub,Taal Hub - Dock 40"
	if got != want {
		t.Fatalf("unexpected double request message:\nwant: %q\n got: %q", want, got)
	}
}

func TestBaselineDoesNotResendOnNextCycle(t *testing.T) {
	logger := log.New(io.Discard, "", 0)
	cfg := workflowConfig{
		BootstrapSendExisting: false,
		DryRun:                true,
		ForceSendAfter:        5 * time.Minute,
		MaxReadyAge:           5 * time.Minute,
	}
	state := workflowState{
		RowPlates:         map[string]string{},
		RowFirstSeenAt:    map[string]string{},
		RowReadyAt:        map[string]string{},
		RowSentForPlate:   map[string]string{},
		RowForcedForPlate: map[string]string{},
	}
	rows := []sheetRow{
		{
			RowNumber:         427,
			RequestTime:       "2/25/2026 12:10:00",
			Cluster:           "Cluster A",
			PlateNumber:       "NIE 1506",
			FleetSizeProvided: "6WH",
			LHType:            "Wetlease",
			ProvideTime:       "2/25/2026 12:25:08",
			DockLabel:         "Dock 11",
		},
	}

	first := processRows(context.Background(), cfg, &http.Client{}, nil, rows, state, false, logger)
	if first.Sent != 0 {
		t.Fatalf("expected no sends on baseline cycle, got sent=%d", first.Sent)
	}
	if state.RowSentForPlate["427"] != "NIE 1506" {
		t.Fatalf("expected baseline to mark row as sent for current plate, got %q", state.RowSentForPlate["427"])
	}

	second := processRows(context.Background(), cfg, &http.Client{}, nil, rows, state, true, logger)
	if second.Sent != 0 {
		t.Fatalf("expected no resend on next cycle, got sent=%d", second.Sent)
	}
	if second.AlreadySentSkipped != 1 {
		t.Fatalf("expected row to be skipped as already sent, got already_sent_skipped=%d", second.AlreadySentSkipped)
	}
}

func TestIsCandidateStaleForSend(t *testing.T) {
	now := time.Date(2026, 2, 28, 17, 40, 0, 0, time.UTC)
	state := workflowState{
		RowReadyAt: map[string]string{
			"1": now.Add(-10 * time.Minute).Format(time.RFC3339),
			"2": now.Add(-2 * time.Minute).Format(time.RFC3339),
		},
		RowFirstSeenAt: map[string]string{
			"3": now.Add(-11 * time.Minute).Format(time.RFC3339),
		},
	}

	if !isCandidateStaleForSend("1", true, false, state, 5*time.Minute, now) {
		t.Fatalf("expected complete candidate to be stale")
	}
	if isCandidateStaleForSend("2", true, false, state, 5*time.Minute, now) {
		t.Fatalf("expected complete candidate to be fresh")
	}
	if !isCandidateStaleForSend("3", false, true, state, 5*time.Minute, now) {
		t.Fatalf("expected force candidate to be stale")
	}
	if isCandidateStaleForSend("1", true, false, state, 0, now) {
		t.Fatalf("expected staleness filter disabled when maxAge<=0")
	}
}

func TestIsProvideTimePastAge(t *testing.T) {
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

	if !isProvideTimePastAge("3/1/2026 11:55:00", 5*time.Minute, now) {
		t.Fatalf("expected provide time exactly 5 minutes old to be eligible")
	}
	if isProvideTimePastAge("3/1/2026 11:57:30", 5*time.Minute, now) {
		t.Fatalf("expected provide time newer than 5 minutes to be ineligible")
	}
	if isProvideTimePastAge("not-a-time", 5*time.Minute, now) {
		t.Fatalf("expected invalid provide time to be ineligible")
	}
	if !isProvideTimePastAge("not-a-time", 0, now) {
		t.Fatalf("expected minAge<=0 to disable provide-time age filter")
	}
}

func TestSendSeaTalkTextByBotAtAllPrefix(t *testing.T) {
	content := "Double Request!\nCluster A"
	cfg := workflowConfig{
		SeaTalkMode:    "bot",
		SeaTalkGroupID: "group-x",
	}
	err := sendSeaTalkText(context.Background(), &http.Client{}, nil, cfg, content, true)
	if err == nil {
		t.Fatalf("expected nil bot client error")
	}
}

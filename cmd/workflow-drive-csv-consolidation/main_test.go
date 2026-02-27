package main

import "testing"

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

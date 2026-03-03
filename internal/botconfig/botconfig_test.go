package botconfig

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"
)

func TestLoadRowsFromSheet(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		if !strings.Contains(r.URL.Path, "/values/") {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"range":          "bot_config!A2:I",
			"majorDimension": "ROWS",
			"values": [][]string{
				{"bot", "wf1", "GROUP-1", "", "", "APP-1", "SECRET-1", "SIGN-1", ""},
				{"webhook", "wf2", "", "", "", "", "", "", "https://example/webhook"},
				{"", "", "", "", "", "", "", "", ""},
			},
		})
	}))
	defer server.Close()

	svc, err := sheets.NewService(
		context.Background(),
		option.WithHTTPClient(server.Client()),
		option.WithEndpoint(server.URL+"/"),
		option.WithoutAuthentication(),
	)
	if err != nil {
		t.Fatalf("new sheets service: %v", err)
	}

	rows, err := LoadRowsFromSheet(context.Background(), svc, "sheet-1", "bot_config")
	if err != nil {
		t.Fatalf("LoadRowsFromSheet error: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	if rows[0].Mode != "bot" || rows[0].Workflow != "wf1" || rows[0].AppSecret != "SECRET-1" {
		t.Fatalf("unexpected first row: %+v", rows[0])
	}
	if rows[1].Mode != "webhook" || rows[1].WebhookURL == "" {
		t.Fatalf("unexpected second row: %+v", rows[1])
	}
}

func TestLoadRowsFromSheetInvalidMode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"values": [][]string{
				{"invalid_mode", "wf1", "group", "", "", "app", "secret", "", ""},
			},
		})
	}))
	defer server.Close()

	svc, err := sheets.NewService(
		context.Background(),
		option.WithHTTPClient(server.Client()),
		option.WithEndpoint(server.URL+"/"),
		option.WithoutAuthentication(),
	)
	if err != nil {
		t.Fatalf("new sheets service: %v", err)
	}

	_, err = LoadRowsFromSheet(context.Background(), svc, "sheet-1", "bot_config")
	if err == nil {
		t.Fatal("expected invalid mode error")
	}
}

func TestResolveForWorkflow(t *testing.T) {
	rows := []Row{
		{Mode: "bot", Workflow: "wf1", SourceRow: 2},
		{Mode: "webhook", Workflow: "wf2", SourceRow: 3},
	}
	row, err := ResolveForWorkflow(rows, "WF2")
	if err != nil {
		t.Fatalf("ResolveForWorkflow error: %v", err)
	}
	if row.Workflow != "wf2" {
		t.Fatalf("expected wf2 row, got %+v", row)
	}

	_, err = ResolveForWorkflow(rows, "wf9")
	if err == nil {
		t.Fatal("expected not found error")
	}

	_, err = ResolveForWorkflow([]Row{
		{Mode: "bot", Workflow: "wf1"},
		{Mode: "webhook", Workflow: "wf1"},
	}, "wf1")
	if err == nil {
		t.Fatal("expected duplicate row error")
	}
}

func TestValidateResolvedRow(t *testing.T) {
	if err := ValidateResolvedRow(Row{
		Mode:        "bot",
		Workflow:    "wf1",
		TargetGroup: "GROUP-1",
		AppID:       "APP",
		AppSecret:   "SECRET",
		SourceRow:   2,
	}); err != nil {
		t.Fatalf("unexpected bot row error: %v", err)
	}

	if err := ValidateResolvedRow(Row{
		Mode:       "webhook",
		Workflow:   "wf2",
		WebhookURL: "https://example/hook",
		SourceRow:  3,
	}); err != nil {
		t.Fatalf("unexpected webhook row error: %v", err)
	}

	if err := ValidateResolvedRow(Row{
		Mode:      "webhook",
		Workflow:  "wf2",
		SourceRow: 4,
	}); err == nil {
		t.Fatal("expected webhook validation error")
	}
}

func TestBuildOwnedGroupRows(t *testing.T) {
	got := BuildOwnedGroupRows([]OwnedGroup{
		{GroupID: "G2", BotName: "wf2"},
		{GroupID: "G1", BotName: "wf1"},
		{GroupID: "G1", BotName: "wf1"},
		{GroupID: "  ", BotName: "wf2"},
	})
	if len(got) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(got))
	}
	if got[0].BotName != "wf1" || got[0].GroupID != "G1" {
		t.Fatalf("unexpected first row: %+v", got[0])
	}
	if got[1].BotName != "wf2" || got[1].GroupID != "G2" {
		t.Fatalf("unexpected second row: %+v", got[1])
	}
}

package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/spxph4227/go-bot-server/internal/botconfig"
	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"
)

func TestBuildSyncTargets(t *testing.T) {
	targets, err := buildSyncTargets([]botconfig.Row{
		{Mode: "bot", Workflow: "wf1", AppID: "a1", AppSecret: "s1"},
		{Mode: "webhook", Workflow: "wf2", AppID: "a2", AppSecret: "s2"},
		{Mode: "bot", Workflow: "wf21", AppID: "", AppSecret: "s3"},
		{Mode: "bot", Workflow: "wf2", AppID: "a2", AppSecret: "s2"},
	})
	if err != nil {
		t.Fatalf("buildSyncTargets error: %v", err)
	}

	if len(targets) != 2 {
		t.Fatalf("expected 2 targets, got %d", len(targets))
	}
	if targets[0].Workflow != "wf1" || targets[1].Workflow != "wf2" {
		t.Fatalf("unexpected targets: %+v", targets)
	}

	_, err = buildSyncTargets([]botconfig.Row{
		{Mode: "bot", Workflow: "wf1", AppID: "a1", AppSecret: "s1"},
		{Mode: "bot", Workflow: "wf1", AppID: "a2", AppSecret: "s2"},
	})
	if err == nil {
		t.Fatal("expected duplicate workflow error")
	}
}

func TestReplaceOwnedGroupInventoryWritesRows(t *testing.T) {
	clearCalls := 0
	updateCalls := 0
	var gotValues [][]interface{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, ":clear"):
			clearCalls++
			_ = json.NewEncoder(w).Encode(map[string]any{})
		case r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/values/"):
			updateCalls++
			var body struct {
				Values [][]interface{} `json:"values"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode update body: %v", err)
			}
			gotValues = body.Values
			_ = json.NewEncoder(w).Encode(map[string]any{
				"updatedRows": len(body.Values),
			})
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
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

	err = replaceOwnedGroupInventory(context.Background(), svc, "sheet-1", "bot_config", []botconfig.OwnedGroup{
		{GroupID: "g-1", BotName: "wf1"},
		{GroupID: "g-2", BotName: "wf2"},
	})
	if err != nil {
		t.Fatalf("replaceOwnedGroupInventory error: %v", err)
	}

	if clearCalls != 1 {
		t.Fatalf("expected 1 clear call, got %d", clearCalls)
	}
	if updateCalls != 1 {
		t.Fatalf("expected 1 update call, got %d", updateCalls)
	}
	if len(gotValues) != 2 {
		t.Fatalf("expected 2 rows written, got %+v", gotValues)
	}
}

func TestReplaceOwnedGroupInventoryClearsWhenEmpty(t *testing.T) {
	clearCalls := 0
	updateCalls := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, ":clear"):
			clearCalls++
			_ = json.NewEncoder(w).Encode(map[string]any{})
		case r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/values/"):
			updateCalls++
			_ = json.NewEncoder(w).Encode(map[string]any{})
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
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

	err = replaceOwnedGroupInventory(context.Background(), svc, "sheet-1", "bot_config", nil)
	if err != nil {
		t.Fatalf("replaceOwnedGroupInventory error: %v", err)
	}
	if clearCalls != 1 {
		t.Fatalf("expected 1 clear call, got %d", clearCalls)
	}
	if updateCalls != 0 {
		t.Fatalf("expected 0 update calls, got %d", updateCalls)
	}
}

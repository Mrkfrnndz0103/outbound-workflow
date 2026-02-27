package workflow

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDefinitions_FiltersDisabledWorkflows(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "workflows.yaml")
	content := `workflows:
  enabled_job:
    command: cmd
    args: ["/C", "echo", "ok"]
    enabled: true
  disabled_job:
    command: cmd
    args: ["/C", "echo", "no"]
    enabled: false
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write temp workflows file: %v", err)
	}

	defs, err := LoadDefinitions(path)
	if err != nil {
		t.Fatalf("LoadDefinitions error: %v", err)
	}
	if len(defs) != 1 {
		t.Fatalf("expected 1 enabled workflow, got %d", len(defs))
	}
	if _, ok := defs["enabled_job"]; !ok {
		t.Fatalf("enabled workflow should be present")
	}
	if _, ok := defs["disabled_job"]; ok {
		t.Fatalf("disabled workflow should be filtered out")
	}
}

func TestLoadDefinitions_DefaultEnabledWhenFieldMissing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "workflows.yaml")
	content := `workflows:
  implicit_enabled:
    command: cmd
    args: ["/C", "echo", "ok"]
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write temp workflows file: %v", err)
	}

	defs, err := LoadDefinitions(path)
	if err != nil {
		t.Fatalf("LoadDefinitions error: %v", err)
	}
	if _, ok := defs["implicit_enabled"]; !ok {
		t.Fatalf("workflow should default to enabled when field is missing")
	}
}

func TestLoadDefinitions_DisabledWorkflowCanHaveEmptyCommand(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "workflows.yaml")
	content := `workflows:
  disabled_without_command:
    enabled: false
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write temp workflows file: %v", err)
	}

	defs, err := LoadDefinitions(path)
	if err != nil {
		t.Fatalf("LoadDefinitions error: %v", err)
	}
	if len(defs) != 0 {
		t.Fatalf("expected no enabled workflows, got %d", len(defs))
	}
}

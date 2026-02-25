package bot

import "testing"

func TestParseCommandWithPrefix(t *testing.T) {
	cmd, ok := parseCommand("/run deploy now", "/")
	if !ok {
		t.Fatal("expected command")
	}
	if cmd.name != "run" {
		t.Fatalf("expected run, got %q", cmd.name)
	}
	if len(cmd.args) != 2 || cmd.args[0] != "deploy" || cmd.args[1] != "now" {
		t.Fatalf("unexpected args: %#v", cmd.args)
	}
}

func TestParseCommandWithoutPrefix(t *testing.T) {
	cmd, ok := parseCommand("list", "")
	if !ok {
		t.Fatal("expected command")
	}
	if cmd.name != "list" {
		t.Fatalf("expected list, got %q", cmd.name)
	}
}

func TestParseCommandInvalid(t *testing.T) {
	_, ok := parseCommand("run deploy", "/")
	if ok {
		t.Fatal("expected parse to fail when prefix is missing")
	}
}

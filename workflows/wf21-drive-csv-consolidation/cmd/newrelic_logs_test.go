package main

import "testing"

func TestNormalizeNewRelicLogAPIURLUsesDefaultWhenEmpty(t *testing.T) {
	got, err := normalizeNewRelicLogAPIURL("")
	if err != nil {
		t.Fatalf("normalizeNewRelicLogAPIURL returned error: %v", err)
	}
	if got != defaultNewRelicLogAPIURL {
		t.Fatalf("unexpected default URL: got=%q want=%q", got, defaultNewRelicLogAPIURL)
	}
}

func TestNormalizeNewRelicLogAPIURLAddsDefaultPath(t *testing.T) {
	got, err := normalizeNewRelicLogAPIURL("https://log-api.newrelic.com")
	if err != nil {
		t.Fatalf("normalizeNewRelicLogAPIURL returned error: %v", err)
	}
	want := "https://log-api.newrelic.com/log/v1"
	if got != want {
		t.Fatalf("unexpected normalized URL: got=%q want=%q", got, want)
	}
}

func TestNormalizeNewRelicLogAPIURLRejectsMissingScheme(t *testing.T) {
	if _, err := normalizeNewRelicLogAPIURL("log-api.newrelic.com/log/v1"); err == nil {
		t.Fatalf("expected error for missing URL scheme")
	}
}

func TestRedactNewRelicLogAPIURLRemovesQuery(t *testing.T) {
	got := redactNewRelicLogAPIURL("https://log-api.newrelic.com/log/v1?apiKey=secret")
	want := "https://log-api.newrelic.com/log/v1"
	if got != want {
		t.Fatalf("unexpected redacted URL: got=%q want=%q", got, want)
	}
}

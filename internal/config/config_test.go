package config

import "testing"

func TestLoadFromEnv_BotModeRequiresSigningSecret(t *testing.T) {
	t.Setenv("SEATALK_MODE", "bot")
	t.Setenv("SEATALK_SIGNING_SECRET", "")

	_, err := LoadFromEnv()
	if err == nil {
		t.Fatal("expected error when signing secret is missing in bot mode")
	}
}

func TestLoadFromEnv_SystemAccountModeRequiresWebhookURL(t *testing.T) {
	t.Setenv("SEATALK_MODE", "system_account")
	t.Setenv("SEATALK_SYSTEM_WEBHOOK_URL", "")

	_, err := LoadFromEnv()
	if err == nil {
		t.Fatal("expected error when webhook URL is missing in system_account mode")
	}
}

func TestLoadFromEnv_SystemAccountModeSuccess(t *testing.T) {
	t.Setenv("SEATALK_MODE", "system_account")
	t.Setenv("SEATALK_SYSTEM_WEBHOOK_URL", "https://openapi.seatalk.io/webhook/group/abc")
	t.Setenv("SEATALK_APP_ID", "")
	t.Setenv("SEATALK_APP_SECRET", "")

	cfg, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Mode != ModeSystemAccount {
		t.Fatalf("expected mode=%s, got %s", ModeSystemAccount, cfg.Mode)
	}
}

func TestLoadFromEnv_SystemAccountModeIgnoresAppCredentialPairing(t *testing.T) {
	t.Setenv("SEATALK_MODE", "system_account")
	t.Setenv("SEATALK_SYSTEM_WEBHOOK_URL", "https://openapi.seatalk.io/webhook/group/abc")
	t.Setenv("SEATALK_APP_ID", "")
	t.Setenv("SEATALK_APP_SECRET", "leftover-secret")

	if _, err := LoadFromEnv(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

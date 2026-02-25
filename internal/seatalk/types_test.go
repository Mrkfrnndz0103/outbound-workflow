package seatalk

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestVerifySignature(t *testing.T) {
	body := []byte(`{"event":"ok"}`)
	secret := "top-secret"

	sum := sha256.Sum256(append(body, []byte(secret)...))
	signature := hex.EncodeToString(sum[:])

	if !VerifySignature(body, secret, signature) {
		t.Fatal("expected signature to match")
	}
	if VerifySignature(body, secret, "bad-signature") {
		t.Fatal("expected signature mismatch")
	}
}

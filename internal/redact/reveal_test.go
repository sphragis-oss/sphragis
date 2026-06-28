// SPDX-License-Identifier: Apache-2.0

package redact

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sphragis-oss/sphragis/internal/vault"
)

func TestRevealResponseNoVault(t *testing.T) {
	ConfigureVault(nil)
	in := []byte(`{"text":"[EMAIL_1]"}`)
	out, err := RevealResponse(in)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != string(in) {
		t.Fatalf("no vault must pass body through: %s", out)
	}
}

func TestRevealResponseRoundTrip(t *testing.T) {
	var key [32]byte
	v, err := vault.Open(filepath.Join(t.TempDir(), "vault.bin"), key)
	if err != nil {
		t.Fatal(err)
	}
	ConfigureVault(v)
	defer ConfigureVault(nil)

	if got := Redact("mail a@b.com").Text; got != "mail [EMAIL_1]" {
		t.Fatalf("setup redaction: %q", got)
	}
	out, err := RevealResponse([]byte(`{"content":[{"text":"reply to [EMAIL_1]"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "a@b.com") || strings.Contains(string(out), "[EMAIL_1]") {
		t.Fatalf("token not revealed: %s", out)
	}
}

func TestRevealResponseEscapesMultilineOriginal(t *testing.T) {
	var key [32]byte
	v, err := vault.Open(filepath.Join(t.TempDir(), "vault.bin"), key)
	if err != nil {
		t.Fatal(err)
	}
	ConfigureVault(v)
	defer ConfigureVault(nil)

	pem := "-----BEGIN PRIVATE KEY-----\nABC\nDEF\n-----END PRIVATE KEY-----"
	v.Assign("PRIVATEKEY", pem) // PRIVATEKEY_1, value has newlines

	out, err := RevealResponse([]byte(`{"text":"[PRIVATEKEY_1]"}`))
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("reveal produced invalid JSON: %v\n%s", err, out)
	}
	if got.Text != pem {
		t.Fatalf("multiline original not restored: %q", got.Text)
	}
}

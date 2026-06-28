// SPDX-License-Identifier: Apache-2.0

package vault

import (
	"path/filepath"
	"testing"
)

func TestVaultRoundTripAndPersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vault.bin")
	var key [32]byte
	for i := range key {
		key[i] = byte(i)
	}

	v, err := Open(path, key)
	if err != nil {
		t.Fatal(err)
	}
	t1 := v.Assign("EMAIL", "john@x.com")
	t2 := v.Assign("EMAIL", "jane@x.com")
	if t1 != "EMAIL_1" || t2 != "EMAIL_2" {
		t.Fatalf("global numbering wrong: %q %q", t1, t2)
	}
	if again := v.Assign("EMAIL", "john@x.com"); again != "EMAIL_1" {
		t.Fatalf("dedupe failed: %q", again)
	}
	if err := v.Flush(); err != nil {
		t.Fatal(err)
	}

	// Reopen from disk: mapping and counter survive, numbering continues.
	v2, err := Open(path, key)
	if err != nil {
		t.Fatal(err)
	}
	got := v2.Reveal("contact [EMAIL_1] or [EMAIL_2], not [EMAIL_9]")
	want := "contact john@x.com or jane@x.com, not [EMAIL_9]"
	if got != want {
		t.Fatalf("reveal: got %q want %q", got, want)
	}
	if next := v2.Assign("EMAIL", "new@x.com"); next != "EMAIL_3" {
		t.Fatalf("counter did not resume: %q", next)
	}
}

func TestVaultWrongKeyFails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vault.bin")
	var k1, k2 [32]byte
	k2[0] = 1
	v, _ := Open(path, k1)
	v.Assign("EMAIL", "john@x.com")
	if err := v.Flush(); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(path, k2); err == nil {
		t.Fatal("expected decryption failure with wrong key")
	}
}

func TestLoadKey(t *testing.T) {
	if _, ok, _ := LoadKey("", ""); ok {
		t.Fatal("no key should report disabled")
	}
	if _, _, err := LoadKey("dG9vc2hvcnQ=", ""); err == nil {
		t.Fatal("short key should error")
	}
	// 32 bytes base64-encoded resolves.
	b64 := "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="
	if _, ok, err := LoadKey(b64, ""); !ok || err != nil {
		t.Fatalf("valid 32B key: ok=%v err=%v", ok, err)
	}
}

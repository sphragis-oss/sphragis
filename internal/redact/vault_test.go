// SPDX-License-Identifier: Apache-2.0

package redact

import (
	"path/filepath"
	"testing"

	"github.com/sphragis-oss/sphragis/internal/vault"
)

func TestRedactWithVaultGlobalAndReversible(t *testing.T) {
	var key [32]byte
	v, err := vault.Open(filepath.Join(t.TempDir(), "vault.bin"), key)
	if err != nil {
		t.Fatal(err)
	}
	r := New(nil)
	r.SetVault(v)

	// Numbering is global across separate Redact calls (unlike per-field mode).
	r1 := r.Redact("first a@b.com")
	r2 := r.Redact("second c@d.com and a@b.com again")
	if r1.Text != "first [EMAIL_1]" {
		t.Fatalf("call 1: %q", r1.Text)
	}
	if r2.Text != "second [EMAIL_2] and [EMAIL_1] again" {
		t.Fatalf("call 2 global numbering wrong: %q", r2.Text)
	}
	if got := v.Reveal(r2.Text); got != "second c@d.com and a@b.com again" {
		t.Fatalf("reveal: %q", got)
	}
}

func TestRedactWithoutVaultIsPerField(t *testing.T) {
	r := New(nil)
	a := r.Redact("a@b.com")
	b := r.Redact("c@d.com")
	if a.Text != "[EMAIL_1]" || b.Text != "[EMAIL_1]" {
		t.Fatalf("per-field numbering should reset: %q %q", a.Text, b.Text)
	}
}

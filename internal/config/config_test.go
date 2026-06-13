// SPDX-License-Identifier: Apache-2.0

package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadCustomTerms(t *testing.T) {
	p := filepath.Join(t.TempDir(), "terms.txt")
	content := "Ada Lovelace\n\n# a comment\nProject Zeus\n  Acme Ltd  \n"
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	terms, err := LoadCustomTerms(p)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"Ada Lovelace", "Project Zeus", "Acme Ltd"}
	if len(terms) != len(want) {
		t.Fatalf("got %v, want %v", terms, want)
	}
	for i := range want {
		if terms[i] != want[i] {
			t.Fatalf("term %d = %q, want %q", i, terms[i], want[i])
		}
	}
}

func TestLoadCustomTermsMissingFile(t *testing.T) {
	if _, err := LoadCustomTerms(filepath.Join(t.TempDir(), "nope.txt")); err == nil {
		t.Fatal("expected error for missing file")
	}
}

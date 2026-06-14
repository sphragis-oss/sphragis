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

func TestLoadConfigFileAndEnvPrecedence(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "sphragis.yaml")
	yaml := "" +
		"listen_addr: \":9999\"\n" +
		"# a comment\n" +
		"ner_url: http://ner.local:5000\n" +
		"ots_calendars:\n" +
		"  - https://a.example\n" +
		"  - https://b.example\n"
	if err := os.WriteFile(cfgPath, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SPHRAGIS_CONFIG", cfgPath)
	t.Setenv("SPHRAGIS_NER_URL", "http://override:5000") // env wins over file

	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.ListenAddr != ":9999" {
		t.Errorf("listen_addr from file: got %q", c.ListenAddr)
	}
	if c.NERURL != "http://override:5000" {
		t.Errorf("env should override file: got %q", c.NERURL)
	}
	if len(c.OTSCalendars) != 2 || c.OTSCalendars[0] != "https://a.example" {
		t.Errorf("ots_calendars list: got %v", c.OTSCalendars)
	}
	if c.AnthropicBaseURL != "https://api.anthropic.com" {
		t.Errorf("default should remain: got %q", c.AnthropicBaseURL)
	}
}

func TestLoadConfigUnknownKey(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "sphragis.yaml")
	if err := os.WriteFile(cfgPath, []byte("bogus_key: x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SPHRAGIS_CONFIG", cfgPath)
	if _, err := Load(); err == nil {
		t.Fatal("expected error on unknown config key")
	}
}

// SPDX-License-Identifier: Apache-2.0

package redact

import (
	"strings"
	"testing"
)

func TestCredentialDetectors(t *testing.T) {
	r := New(nil)
	cases := []struct {
		name string
		in   string
		kind Kind
		gone string
	}{
		{"openai", "key sk-ant-abcdefghijklmnopqrstuvwxyz0123", APIKey, "sk-ant-abcdefghijklmnopqrstuvwxyz0123"},
		{"aws", "id AKIAIOSFODNN7EXAMPLE here", APIKey, "AKIAIOSFODNN7EXAMPLE"},
		{"github", "tok ghp_012345678901234567890123456789012345 ok", APIKey, "ghp_012345678901234567890123456789012345"},
		{"jwt", "t eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c x", JWT, "eyJhbGciOiJIUzI1NiJ9"},
		{"ssn", "ssn 123-45-6789 end", SSN, "123-45-6789"},
		{"ip", "host 192.168.1.10 down", IP, "192.168.1.10"},
	}
	for _, c := range cases {
		res := r.Redact(c.in)
		if res.Counts[c.kind] != 1 {
			t.Errorf("%s: want 1 %s, got %d (text=%q)", c.name, c.kind, res.Counts[c.kind], res.Text)
		}
		if strings.Contains(res.Text, c.gone) {
			t.Errorf("%s: value leaked: %s", c.name, res.Text)
		}
	}
}

func TestKeyedSecretValue(t *testing.T) {
	r := New(nil)
	res := r.Redact(`password=hunter2 and "api_key": "abcd1234efgh"`)
	if res.Counts[Secret] < 2 {
		t.Fatalf("want >=2 secrets, got %d (%s)", res.Counts[Secret], res.Text)
	}
	if strings.Contains(res.Text, "hunter2") || strings.Contains(res.Text, "abcd1234efgh") {
		t.Fatalf("secret value leaked: %s", res.Text)
	}
	if !strings.Contains(res.Text, "password=") {
		t.Fatalf("key label should be kept: %s", res.Text)
	}
}

func TestPrivateKeyBlock(t *testing.T) {
	r := New(nil)
	in := "x -----BEGIN RSA PRIVATE KEY-----\nMIIBOgIBAAJBAKj34\nabc\n-----END RSA PRIVATE KEY----- y"
	res := r.Redact(in)
	if res.Counts[PrivateKey] != 1 {
		t.Fatalf("want 1 private key, got %d", res.Counts[PrivateKey])
	}
	if strings.Contains(res.Text, "MIIBOgIBAAJBAKj34") {
		t.Fatalf("private key body leaked: %s", res.Text)
	}
}

func TestCardLuhn(t *testing.T) {
	r := New(nil)
	if got := r.Redact("pay 4111 1111 1111 1111").Counts[Card]; got != 1 {
		t.Fatalf("valid card should redact, got %d", got)
	}
	if got := r.Redact("ref 4111 1111 1111 1112").Counts[Card]; got != 0 {
		t.Fatalf("luhn-invalid number should not redact, got %d", got)
	}
	if got := r.Redact("amex 3782 822463 10005").Counts[Card]; got != 1 {
		t.Fatalf("valid 15-digit amex should redact, got %d", got)
	}
}

func TestCustomTerms(t *testing.T) {
	r := New([]string{"Ada Lovelace", "Project Zeus"})
	res := r.Redact("ping Ada Lovelace about Project Zeus")
	if res.Counts[Name] != 2 {
		t.Fatalf("want 2 name matches, got %d (%s)", res.Counts[Name], res.Text)
	}
	if strings.Contains(res.Text, "Ada Lovelace") {
		t.Fatalf("custom term leaked: %s", res.Text)
	}
}

func TestConfigureAffectsPackageRedact(t *testing.T) {
	defer Configure(nil, false)
	Configure([]string{"Zephyr Corp"}, false)
	res := Redact("deal with Zephyr Corp today")
	if res.Counts[Name] != 1 {
		t.Fatalf("Configure terms not applied via package Redact: %v", res)
	}
}

func TestNoFalsePositivesOnPlainText(t *testing.T) {
	r := New(nil)
	res := r.Redact("Let us meet at 3pm to review the Q3 roadmap and ship v1.2.")
	if len(res.Counts) != 0 {
		t.Fatalf("plain prose should redact nothing, got %v (%s)", res.Counts, res.Text)
	}
}

func TestMoreProviderKeys(t *testing.T) {
	r := New(nil)
	cases := []struct{ in, gone string }{
		{"stripe sk_live_0123456789abcdefghij here", "sk_live_0123456789abcdefghij"},
		{"slack xoxb-1234567890-abcdEFGH down", "xoxb-1234567890-abcdEFGH"},
	}
	for _, c := range cases {
		res := r.Redact(c.in)
		if res.Counts[APIKey] != 1 {
			t.Errorf("want 1 apikey for %q, got %d (%s)", c.in, res.Counts[APIKey], res.Text)
		}
		if strings.Contains(res.Text, c.gone) {
			t.Errorf("key leaked: %s", res.Text)
		}
	}
}

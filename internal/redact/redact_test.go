// SPDX-License-Identifier: Apache-2.0

package redact

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRedactDetectsAllKinds(t *testing.T) {
	in := "mail a@b.com, call +30 694 1234567, card 4111 1111 1111 1111, iban GR16 0110 XXXX XXXX XXXX XXXX XXX"
	got := Redact(in)
	for _, k := range []Kind{Email, Phone, Card, IBAN} {
		if got.Counts[k] != 1 {
			t.Errorf("kind %s: got count %d, want 1 (text=%q)", k, got.Counts[k], got.Text)
		}
	}
	for _, leaked := range []string{"a@b.com", "4111 1111 1111 1111", "+30 694 1234567"} {
		if strings.Contains(got.Text, leaked) {
			t.Errorf("redacted text still contains %q: %s", leaked, got.Text)
		}
	}
}

func TestRedactDedupesStableTokens(t *testing.T) {
	got := Redact("a@b.com then again a@b.com and c@d.com")
	if got.Counts[Email] != 3 {
		t.Fatalf("want 3 email hits, got %d", got.Counts[Email])
	}
	if strings.Count(got.Text, "[EMAIL_1]") != 2 || !strings.Contains(got.Text, "[EMAIL_2]") {
		t.Fatalf("unstable token numbering: %s", got.Text)
	}
}

func TestRedactChatRequest(t *testing.T) {
	body := `{"model":"gpt-4o","messages":[{"role":"user","content":"reach me at x@y.eu"}]}`
	out, counts, err := RedactChatRequest([]byte(body))
	if err != nil {
		t.Fatal(err)
	}
	if counts[Email] != 1 {
		t.Fatalf("want 1 email, got %d", counts[Email])
	}
	if strings.Contains(string(out), "x@y.eu") {
		t.Fatalf("PII survived in chat body: %s", out)
	}
	var parsed map[string]any
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("output not valid json: %v", err)
	}
}

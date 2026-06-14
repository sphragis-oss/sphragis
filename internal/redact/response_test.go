// SPDX-License-Identifier: Apache-2.0

package redact

import (
	"strings"
	"testing"
)

func TestRedactResponseByFormat(t *testing.T) {
	cases := []struct{ name, path, body string }{
		{"chat", "/v1/chat/completions", `{"choices":[{"message":{"role":"assistant","content":"write to a@b.com"}}]}`},
		{"anthropic", "/v1/messages", `{"content":[{"type":"text","text":"write to a@b.com"}]}`},
		{"responses", "/v1/responses", `{"output":[{"type":"message","content":[{"type":"output_text","text":"write to a@b.com"}]}]}`},
		{"complete", "/v1/complete", `{"completion":"write to a@b.com"}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, counts, err := RedactResponse(c.path, []byte(c.body))
			if err != nil {
				t.Fatal(err)
			}
			if counts[Email] != 1 {
				t.Fatalf("want 1 email, got %d", counts[Email])
			}
			if strings.Contains(string(out), "a@b.com") {
				t.Fatalf("PII survived: %s", out)
			}
			if !strings.Contains(string(out), "[EMAIL_1]") {
				t.Fatalf("expected token: %s", out)
			}
		})
	}
}

func TestRedactResponseNonJSON(t *testing.T) {
	if _, _, err := RedactResponse("/v1/messages", []byte("not json")); err == nil {
		t.Fatal("expected error on non-JSON body")
	}
}

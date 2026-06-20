// SPDX-License-Identifier: Apache-2.0

package redact

import (
	"strings"
	"testing"
)

const geminiPath = "/v1beta/models/gemini-1.5-pro:generateContent"

func TestRedactGeminiRequest(t *testing.T) {
	body := []byte(`{"systemInstruction":{"parts":[{"text":"note for boss@corp.com"}]},` +
		`"contents":[{"role":"user","parts":[{"text":"email me at a@b.com"}]}]}`)
	out, counts, err := RedactRequest(geminiPath, body)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if strings.Contains(s, "a@b.com") || strings.Contains(s, "boss@corp.com") {
		t.Fatalf("gemini request PII leaked: %s", s)
	}
	if counts[Email] != 2 {
		t.Fatalf("want 2 emails redacted, got %v", counts)
	}
}

func TestRedactGeminiResponse(t *testing.T) {
	body := []byte(`{"candidates":[{"content":{"role":"model","parts":[{"text":"reply to a@b.com"}]}}]}`)
	out, counts, err := RedactResponse(geminiPath, body)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), "a@b.com") {
		t.Fatalf("gemini response PII leaked: %s", out)
	}
	if counts[Email] != 1 {
		t.Fatalf("want 1 email redacted, got %v", counts)
	}
}

func TestStreamGemini(t *testing.T) {
	sr := NewStreamRedactor("/v1beta/models/gemini-1.5-pro:streamGenerateContent")
	var buf strings.Builder
	body := strings.NewReader("data: " +
		`{"candidates":[{"content":{"parts":[{"text":"reach me at a@b.com"}]}}]}` + "\n\n")
	sr.Process(&buf, func() {}, body)
	out := buf.String()
	if strings.Contains(out, "a@b.com") || !strings.Contains(out, "[EMAIL_1]") {
		t.Fatalf("gemini stream not redacted: %s", out)
	}
}

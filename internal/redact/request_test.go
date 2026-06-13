// SPDX-License-Identifier: Apache-2.0

package redact

import (
	"strings"
	"testing"
)

func mustRedact(t *testing.T, path, body string) (string, map[Kind]int) {
	t.Helper()
	out, counts, err := RedactRequest(path, []byte(body))
	if err != nil {
		t.Fatalf("RedactRequest(%s): %v", path, err)
	}
	return string(out), counts
}

func TestOpenAIChatArrayContent(t *testing.T) {
	out, c := mustRedact(t, "/v1/chat/completions",
		`{"model":"gpt-4o","messages":[{"role":"user","content":[{"type":"text","text":"mail a@b.com"}]}]}`)
	if c[Email] != 1 {
		t.Fatalf("want 1 email, got %d", c[Email])
	}
	if strings.Contains(out, "a@b.com") {
		t.Fatalf("PII leaked: %s", out)
	}
}

func TestOpenAIResponsesStringInput(t *testing.T) {
	out, c := mustRedact(t, "/v1/responses",
		`{"model":"gpt-5","input":"card 4111 1111 1111 1111"}`)
	if c[Card] != 1 {
		t.Fatalf("want 1 card, got %d", c[Card])
	}
	if strings.Contains(out, "4111 1111 1111 1111") {
		t.Fatalf("PII leaked: %s", out)
	}
}

func TestOpenAIResponsesArrayInputAndInstructions(t *testing.T) {
	out, c := mustRedact(t, "/v1/responses",
		`{"model":"gpt-5","instructions":"reply to a@b.com","input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"iban GR16 0110 XXXX XXXX XXXX XXXX XXX"}]}]}`)
	if c[Email] != 1 || c[IBAN] != 1 {
		t.Fatalf("want email=1 iban=1, got email=%d iban=%d", c[Email], c[IBAN])
	}
	if strings.Contains(out, "a@b.com") {
		t.Fatalf("instruction PII leaked: %s", out)
	}
}

func TestAnthropicMessagesStringAndSystem(t *testing.T) {
	out, c := mustRedact(t, "/v1/messages",
		`{"model":"claude-3","system":"sys a@b.com","messages":[{"role":"user","content":"call +30 694 1234567"}]}`)
	if c[Email] != 1 || c[Phone] != 1 {
		t.Fatalf("want email=1 phone=1, got email=%d phone=%d", c[Email], c[Phone])
	}
	if strings.Contains(out, "a@b.com") || strings.Contains(out, "+30 694 1234567") {
		t.Fatalf("PII leaked: %s", out)
	}
}

func TestAnthropicArrayBlocks(t *testing.T) {
	out, c := mustRedact(t, "/v1/messages",
		`{"model":"claude-3","messages":[{"role":"user","content":[{"type":"text","text":"card 4111 1111 1111 1111"}]}]}`)
	if c[Card] != 1 {
		t.Fatalf("want 1 card, got %d", c[Card])
	}
	if strings.Contains(out, "4111 1111 1111 1111") {
		t.Fatalf("PII leaked: %s", out)
	}
}

func TestUnknownPathPassthrough(t *testing.T) {
	out, c := mustRedact(t, "/v1/embeddings", `{"foo":"a@b.com"}`)
	if len(c) != 0 {
		t.Fatalf("unknown path should redact nothing, got %v", c)
	}
	if !strings.Contains(out, "a@b.com") {
		t.Fatalf("unknown path should pass through unchanged: %s", out)
	}
}

func TestAnthropicDocumentBlock(t *testing.T) {
	out, c := mustRedact(t, "/v1/messages",
		`{"model":"claude-3","messages":[{"role":"user","content":[{"type":"document","source":{"type":"text","media_type":"text/plain","data":"holder a@b.com card 4111 1111 1111 1111"}}]}]}`)
	if c[Email] != 1 || c[Card] != 1 {
		t.Fatalf("document not redacted: %v out=%s", c, out)
	}
	if strings.Contains(out, "a@b.com") || strings.Contains(out, "4111 1111 1111 1111") {
		t.Fatalf("document PII leaked: %s", out)
	}
}

func TestAnthropicToolUseInput(t *testing.T) {
	out, c := mustRedact(t, "/v1/messages",
		`{"model":"claude-3","messages":[{"role":"assistant","content":[{"type":"tool_use","id":"toolu_1","name":"lookup","input":{"email":"a@b.com","note":"call +30 694 1234567"}}]}]}`)
	if c[Email] != 1 || c[Phone] != 1 {
		t.Fatalf("tool_use input not redacted: %v", c)
	}
	if strings.Contains(out, "a@b.com") {
		t.Fatalf("tool_use PII leaked: %s", out)
	}
}

func TestAnthropicToolResult(t *testing.T) {
	out, c := mustRedact(t, "/v1/messages",
		`{"model":"claude-3","messages":[{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_1","content":[{"type":"text","text":"customer a@b.com"}]}]}]}`)
	if c[Email] != 1 {
		t.Fatalf("tool_result not redacted: %v", c)
	}
	if strings.Contains(out, "a@b.com") {
		t.Fatalf("tool_result PII leaked: %s", out)
	}
}

func TestAnthropicThinkingPreserved(t *testing.T) {
	out, c := mustRedact(t, "/v1/messages",
		`{"model":"claude-3","messages":[{"role":"assistant","content":[{"type":"thinking","thinking":"user is a@b.com","signature":"sig123"}]}]}`)
	if c[Email] != 0 {
		t.Fatalf("thinking block must not be redacted (signed), got %v", c)
	}
	if !strings.Contains(out, "a@b.com") || !strings.Contains(out, "sig123") {
		t.Fatalf("thinking block must be preserved intact: %s", out)
	}
}

func TestAnthropicCountTokens(t *testing.T) {
	out, c := mustRedact(t, "/v1/messages/count_tokens",
		`{"model":"claude-3","messages":[{"role":"user","content":"mail a@b.com"}]}`)
	if c[Email] != 1 {
		t.Fatalf("count_tokens not redacted: %v", c)
	}
	if strings.Contains(out, "a@b.com") {
		t.Fatalf("count_tokens PII leaked: %s", out)
	}
}

func TestAnthropicBatch(t *testing.T) {
	out, c := mustRedact(t, "/v1/messages/batches",
		`{"requests":[{"custom_id":"a","params":{"model":"claude-3","messages":[{"role":"user","content":"card 4111 1111 1111 1111"}]}}]}`)
	if c[Card] != 1 {
		t.Fatalf("batch not redacted: %v", c)
	}
	if strings.Contains(out, "4111 1111 1111 1111") {
		t.Fatalf("batch PII leaked: %s", out)
	}
}

func TestAnthropicLegacyComplete(t *testing.T) {
	out, c := mustRedact(t, "/v1/complete",
		`{"model":"claude-2","prompt":"\n\nHuman: mail a@b.com\n\nAssistant:"}`)
	if c[Email] != 1 {
		t.Fatalf("complete not redacted: %v", c)
	}
	if strings.Contains(out, "a@b.com") {
		t.Fatalf("complete PII leaked: %s", out)
	}
}

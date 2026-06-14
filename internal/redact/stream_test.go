// SPDX-License-Identifier: Apache-2.0

package redact

import (
	"strings"
	"testing"
)

// run feeds an SSE stream through a fresh StreamRedactor and returns the output.
func run(path, in string) string {
	var b strings.Builder
	NewStreamRedactor(path).Process(&b, func() {}, strings.NewReader(in))
	return b.String()
}

func TestStreamAnthropicSplitAcrossDeltas(t *testing.T) {
	in := "event: content_block_delta\n" +
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"mail jo"}}` + "\n\n" +
		"event: content_block_delta\n" +
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hn@x.com"}}` + "\n\n" +
		"event: content_block_stop\n" +
		`data: {"type":"content_block_stop","index":0}` + "\n\n"
	out := run("/v1/messages", in)
	if strings.Contains(out, "john@x.com") {
		t.Fatalf("PII split across deltas leaked: %s", out)
	}
	if !strings.Contains(out, "[EMAIL_1]") {
		t.Fatalf("expected token in stream: %s", out)
	}
}

func TestStreamOpenAIChatSplitAcrossDeltas(t *testing.T) {
	in := `data: {"choices":[{"index":0,"delta":{"content":"mail jo"}}]}` + "\n\n" +
		`data: {"choices":[{"index":0,"delta":{"content":"hn@x.com"}}]}` + "\n\n" +
		`data: {"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}` + "\n\n" +
		"data: [DONE]\n\n"
	out := run("/v1/chat/completions", in)
	if strings.Contains(out, "john@x.com") {
		t.Fatalf("PII split across deltas leaked: %s", out)
	}
	if !strings.Contains(out, "[EMAIL_1]") {
		t.Fatalf("expected token in stream: %s", out)
	}
	if !strings.Contains(out, "[DONE]") {
		t.Fatalf("terminal marker dropped: %s", out)
	}
}

func TestStreamPassesThroughNonJSON(t *testing.T) {
	in := "event: a\ndata: 1\n\nevent: b\ndata: 2\n\n"
	out := run("/v1/messages", in)
	if !strings.Contains(out, "data: 1") || !strings.Contains(out, "data: 2") {
		t.Fatalf("non-JSON SSE not passed through: %q", out)
	}
}

func TestStreamFlushesAtNewline(t *testing.T) {
	in := "event: content_block_delta\n" +
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"a@b.com here\nmore"}}` + "\n\n" +
		"event: content_block_stop\n" +
		`data: {"type":"content_block_stop","index":0}` + "\n\n"
	out := run("/v1/messages", in)
	if strings.Contains(out, "a@b.com") {
		t.Fatalf("email leaked: %s", out)
	}
	if c := strings.Count(out, "[EMAIL_1]"); c != 1 {
		t.Fatalf("want 1 stable token, got %d: %s", c, out)
	}
}

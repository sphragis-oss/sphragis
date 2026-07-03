// SPDX-License-Identifier: Apache-2.0

package proxy

import (
	"bytes"
	"strings"
	"testing"
)

func TestSplitAgent(t *testing.T) {
	cases := []struct{ path, agent, rest string }{
		{"/agent/coder/v1/messages", "coder", "/v1/messages"},
		{"/agent/my-role.2/v1/chat/completions", "my-role.2", "/v1/chat/completions"},
		{"/v1/messages", "", "/v1/messages"},
		{"/agent//v1/messages", "", "/agent//v1/messages"},
		{"/agent/coder", "", "/agent/coder"},
		{"/agent/bad name/v1/messages", "", "/agent/bad name/v1/messages"},
		{"/agent/" + strings.Repeat("x", 65) + "/v1/messages", "", "/agent/" + strings.Repeat("x", 65) + "/v1/messages"},
	}
	for _, c := range cases {
		agent, rest := splitAgent(c.path)
		if agent != c.agent || rest != c.rest {
			t.Errorf("splitAgent(%q) = %q, %q; want %q, %q", c.path, agent, rest, c.agent, c.rest)
		}
	}
}

func TestTallyAbsorbJSON(t *testing.T) {
	cases := []struct {
		name, body string
		want       tokenTally
	}{
		{"anthropic", `{"usage":{"input_tokens":10,"output_tokens":20,"cache_creation_input_tokens":3,"cache_read_input_tokens":400}}`,
			tokenTally{input: 10, output: 20, cacheCreation: 3, cacheRead: 400}},
		{"openai", `{"usage":{"prompt_tokens":15,"completion_tokens":25}}`,
			tokenTally{input: 15, output: 25}},
		{"gemini", `{"usageMetadata":{"promptTokenCount":7,"candidatesTokenCount":9}}`,
			tokenTally{input: 7, output: 9}},
		{"no usage", `{"ok":true}`, tokenTally{}},
		{"not json", `oops`, tokenTally{}},
	}
	for _, c := range cases {
		var got tokenTally
		got.absorbJSON([]byte(c.body))
		if got != c.want {
			t.Errorf("%s: tally = %+v, want %+v", c.name, got, c.want)
		}
	}
}

func TestUsageScannerAnthropicStream(t *testing.T) {
	stream := "event: message_start\n" +
		`data: {"type":"message_start","message":{"usage":{"input_tokens":25,"cache_read_input_tokens":100,"output_tokens":1}}}` + "\n\n" +
		"event: content_block_delta\n" +
		`data: {"type":"content_block_delta","delta":{"text":"hi"}}` + "\n\n" +
		"event: message_delta\n" +
		`data: {"type":"message_delta","usage":{"output_tokens":12}}` + "\n\n" +
		`data: {"type":"message_delta","usage":{"output_tokens":42}}` + "\n\n" +
		"data: [DONE]\n"
	var out bytes.Buffer
	tally := &tokenTally{}
	s := newUsageScanner(&out, tally)
	// write in tiny chunks to exercise line reassembly across writes
	for i := 0; i < len(stream); i += 7 {
		end := min(i+7, len(stream))
		if _, err := s.Write([]byte(stream[i:end])); err != nil {
			t.Fatal(err)
		}
	}
	if out.String() != stream {
		t.Fatal("scanner altered the relayed stream")
	}
	want := tokenTally{input: 25, output: 42, cacheRead: 100}
	if *tally != want {
		t.Fatalf("tally = %+v, want %+v", *tally, want)
	}
}

func TestUsageScannerSkipsOversizedLines(t *testing.T) {
	tally := &tokenTally{}
	s := newUsageScanner(&bytes.Buffer{}, tally)
	huge := "data: " + strings.Repeat("x", maxUsageLine+1) + "\n"
	if _, err := s.Write([]byte(huge)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Write([]byte(`data: {"usage":{"prompt_tokens":5,"completion_tokens":6}}` + "\n")); err != nil {
		t.Fatal(err)
	}
	want := tokenTally{input: 5, output: 6}
	if *tally != want {
		t.Fatalf("tally after oversized line = %+v, want %+v", *tally, want)
	}
}

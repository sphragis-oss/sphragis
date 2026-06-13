// SPDX-License-Identifier: Apache-2.0

package redact

import (
	"encoding/json"
	"strings"
)

// RedactRequest redacts a request body, dispatching by path; unknown paths pass through.
func RedactRequest(path string, body []byte) ([]byte, map[Kind]int, error) {
	var req map[string]any
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, nil, err
	}
	total := map[Kind]int{}
	switch {
	case strings.Contains(path, "/chat/completions"):
		redactMessages(req, total)
	case strings.Contains(path, "/responses"):
		redactResponses(req, total)
	case strings.Contains(path, "/messages/batches"):
		redactAnthropicBatch(req, total)
	case strings.Contains(path, "/messages"):
		redactAnthropic(req, total)
	case strings.Contains(path, "/complete"):
		redactComplete(req, total)
	}
	out, err := json.Marshal(req)
	if err != nil {
		return nil, nil, err
	}
	return out, total, nil
}

// RedactChatRequest redacts an OpenAI-compatible chat-completions body.
func RedactChatRequest(body []byte) ([]byte, map[Kind]int, error) {
	return RedactRequest("/v1/chat/completions", body)
}

func redactString(s string, total map[Kind]int) string {
	res := Redact(s)
	for k, v := range res.Counts {
		total[k] += v
	}
	return res.Text
}

// redactContent handles a string or an array of content blocks.
func redactContent(v any, total map[Kind]int) any {
	switch c := v.(type) {
	case string:
		return redactString(c, total)
	case []any:
		for _, item := range c {
			if m, ok := item.(map[string]any); ok {
				redactBlock(m, total)
			}
		}
		return c
	default:
		return v
	}
}

// OpenAI chat completions: messages[].content (string or text parts).
func redactMessages(req map[string]any, total map[Kind]int) {
	msgs, ok := req["messages"].([]any)
	if !ok {
		return
	}
	for _, m := range msgs {
		if mm, ok := m.(map[string]any); ok {
			mm["content"] = redactContent(mm["content"], total)
		}
	}
}

// OpenAI Responses: instructions (string) + input (string or message items).
func redactResponses(req map[string]any, total map[Kind]int) {
	if s, ok := req["instructions"].(string); ok {
		req["instructions"] = redactString(s, total)
	}
	switch input := req["input"].(type) {
	case string:
		req["input"] = redactString(input, total)
	case []any:
		for _, item := range input {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if _, has := m["content"]; has {
				m["content"] = redactContent(m["content"], total)
			}
			if s, ok := m["text"].(string); ok {
				m["text"] = redactString(s, total)
			}
		}
	}
}

// Anthropic Messages: system (string or blocks) + messages[].content.
func redactAnthropic(req map[string]any, total map[Kind]int) {
	switch sys := req["system"].(type) {
	case string:
		req["system"] = redactString(sys, total)
	case []any:
		for _, b := range sys {
			if m, ok := b.(map[string]any); ok {
				if s, ok := m["text"].(string); ok {
					m["text"] = redactString(s, total)
				}
			}
		}
	}
	if msgs, ok := req["messages"].([]any); ok {
		for _, m := range msgs {
			if mm, ok := m.(map[string]any); ok {
				mm["content"] = redactContent(mm["content"], total)
			}
		}
	}
}

// redactBlock redacts one content block; signed thinking blocks are left intact.
func redactBlock(m map[string]any, total map[Kind]int) {
	if t, _ := m["type"].(string); t == "thinking" || t == "redacted_thinking" {
		return
	}
	if s, ok := m["text"].(string); ok {
		m["text"] = redactString(s, total)
	}
	if in, ok := m["input"]; ok { // tool_use arguments
		m["input"] = redactAny(in, total)
	}
	if src, ok := m["source"].(map[string]any); ok { // document source
		redactSource(src, total)
	}
	if s, ok := m["title"].(string); ok {
		m["title"] = redactString(s, total)
	}
	if s, ok := m["context"].(string); ok {
		m["context"] = redactString(s, total)
	}
	if _, ok := m["content"]; ok { // tool_result / nested
		m["content"] = redactContent(m["content"], total)
	}
}

func redactSource(src map[string]any, total map[Kind]int) {
	switch t, _ := src["type"].(string); t {
	case "text":
		if s, ok := src["data"].(string); ok {
			src["data"] = redactString(s, total)
		}
	case "content":
		if _, ok := src["content"]; ok {
			src["content"] = redactContent(src["content"], total)
		}
	}
}

// redactAny redacts every string in an arbitrary JSON value (used for tool_use input).
func redactAny(v any, total map[Kind]int) any {
	switch x := v.(type) {
	case string:
		return redactString(x, total)
	case []any:
		for i, e := range x {
			x[i] = redactAny(e, total)
		}
		return x
	case map[string]any:
		for k, e := range x {
			x[k] = redactAny(e, total)
		}
		return x
	default:
		return v
	}
}

// redactAnthropicBatch redacts each requests[].params messages body.
func redactAnthropicBatch(req map[string]any, total map[Kind]int) {
	reqs, ok := req["requests"].([]any)
	if !ok {
		return
	}
	for _, it := range reqs {
		if m, ok := it.(map[string]any); ok {
			if params, ok := m["params"].(map[string]any); ok {
				redactAnthropic(params, total)
			}
		}
	}
}

// redactComplete handles the legacy Text Completions API: a single prompt.
func redactComplete(req map[string]any, total map[Kind]int) {
	if s, ok := req["prompt"].(string); ok {
		req["prompt"] = redactString(s, total)
	}
}

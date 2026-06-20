// SPDX-License-Identifier: Apache-2.0

package redact

import (
	"encoding/json"
	"strings"
)

// RedactResponse redacts a non-streaming response body, dispatching by path; unknown paths pass through.
func RedactResponse(path string, body []byte) ([]byte, map[Kind]int, error) {
	var resp map[string]any
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, nil, err
	}
	total := map[Kind]int{}
	switch {
	case isGeminiPath(path):
		redactGeminiResponse(resp, total)
	case strings.Contains(path, "/chat/completions"):
		redactChatResponse(resp, total)
	case strings.Contains(path, "/responses"):
		redactResponsesResponse(resp, total)
	case strings.Contains(path, "/messages/batches"):
		// Batch results are fetched as a separate JSONL file, not this body.
	case strings.Contains(path, "/messages"):
		redactAnthropicResponse(resp, total)
	case strings.Contains(path, "/complete"):
		redactCompleteResponse(resp, total)
	}
	out, err := json.Marshal(resp)
	if err != nil {
		return nil, nil, err
	}
	return out, total, nil
}

// OpenAI chat completions: choices[].message.content.
func redactChatResponse(resp map[string]any, total map[Kind]int) {
	choices, ok := resp["choices"].([]any)
	if !ok {
		return
	}
	for _, c := range choices {
		cm, ok := c.(map[string]any)
		if !ok {
			continue
		}
		if msg, ok := cm["message"].(map[string]any); ok {
			msg["content"] = redactContent(msg["content"], total)
		}
	}
}

// OpenAI Responses: output[].content[] text blocks (+ item-level text).
func redactResponsesResponse(resp map[string]any, total map[Kind]int) {
	out, ok := resp["output"].([]any)
	if !ok {
		return
	}
	for _, item := range out {
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

// Anthropic Messages: content[] blocks (text, tool_use input, ...).
func redactAnthropicResponse(resp map[string]any, total map[Kind]int) {
	if _, ok := resp["content"]; ok {
		resp["content"] = redactContent(resp["content"], total)
	}
}

// Anthropic legacy Text Completions: a single completion string.
func redactCompleteResponse(resp map[string]any, total map[Kind]int) {
	if s, ok := resp["completion"].(string); ok {
		resp["completion"] = redactString(s, total)
	}
}

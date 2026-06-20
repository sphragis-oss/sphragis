// SPDX-License-Identifier: Apache-2.0

package redact

import "strings"

// isGeminiPath reports whether a path is a Google Generative Language API call.
func isGeminiPath(path string) bool {
	return strings.HasPrefix(path, "/v1beta/") ||
		strings.Contains(path, ":generateContent") ||
		strings.Contains(path, ":streamGenerateContent")
}

// Gemini request: contents[].parts[].text and systemInstruction.parts[].text.
func redactGemini(req map[string]any, total map[Kind]int) {
	if si, ok := req["systemInstruction"].(map[string]any); ok {
		redactGeminiParts(si, total)
	}
	if si, ok := req["system_instruction"].(map[string]any); ok {
		redactGeminiParts(si, total)
	}
	if contents, ok := req["contents"].([]any); ok {
		for _, c := range contents {
			if cm, ok := c.(map[string]any); ok {
				redactGeminiParts(cm, total)
			}
		}
	}
}

// Gemini response: candidates[].content.parts[].text.
func redactGeminiResponse(resp map[string]any, total map[Kind]int) {
	cands, ok := resp["candidates"].([]any)
	if !ok {
		return
	}
	for _, c := range cands {
		if cm, ok := c.(map[string]any); ok {
			if content, ok := cm["content"].(map[string]any); ok {
				redactGeminiParts(content, total)
			}
		}
	}
}

// redactGeminiParts redacts the text and tool args/results in a container's parts.
func redactGeminiParts(container map[string]any, total map[Kind]int) {
	parts, ok := container["parts"].([]any)
	if !ok {
		return
	}
	for _, p := range parts {
		pm, ok := p.(map[string]any)
		if !ok {
			continue
		}
		if s, ok := pm["text"].(string); ok {
			pm["text"] = redactString(s, total)
		}
		if fc, ok := pm["functionCall"].(map[string]any); ok {
			if args, ok := fc["args"]; ok {
				fc["args"] = redactAny(args, total)
			}
		}
		if fr, ok := pm["functionResponse"].(map[string]any); ok {
			if r, ok := fr["response"]; ok {
				fr["response"] = redactAny(r, total)
			}
		}
	}
}

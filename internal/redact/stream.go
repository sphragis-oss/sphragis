// SPDX-License-Identifier: Apache-2.0

package redact

import (
	"bufio"
	"encoding/json"
	"io"
	"strings"
)

// streamFormat selects how SSE delta text is located per provider protocol.
type streamFormat int

const (
	formatOther streamFormat = iota
	formatAnthropic
	formatOpenAIChat
)

// streamFormatFor classifies a request path; only Anthropic Messages and OpenAI
// chat get cross-chunk holdback, others fall back to per-event redaction.
func streamFormatFor(path string) streamFormat {
	switch {
	case strings.Contains(path, "/chat/completions"):
		return formatOpenAIChat
	case strings.Contains(path, "/messages"):
		return formatAnthropic
	default:
		return formatOther
	}
}

// StreamRedactor redacts an SSE response stream. Assistant text is buffered
// across events and flushed at newline boundaries so values split across chunks
// (e.g. an email arriving as "jo" then "hn@x.com") are still tokenized. Token
// numbering is stable for the whole stream. NER is not run on streamed bodies.
type StreamRedactor struct {
	r      *Redactor
	format streamFormat
	counts map[Kind]int
	seen   map[Kind]map[string]int

	carry     string
	flushTmpl func(text string) (name string, data []byte)
}

// NewStreamRedactor builds a stream redactor for the default redactor and path.
func NewStreamRedactor(path string) *StreamRedactor {
	return &StreamRedactor{
		r:      defaultRedactor,
		format: streamFormatFor(path),
		counts: map[Kind]int{},
		seen:   map[Kind]map[string]int{},
	}
}

// Counts returns the redaction tally accumulated over the stream, keyed by kind name.
func (s *StreamRedactor) Counts() map[string]int {
	out := make(map[string]int, len(s.counts))
	for k, v := range s.counts {
		out[string(k)] = v
	}
	return out
}

// redactStateful applies the regex/custom detectors with carried-over numbering; NER is skipped.
func (s *StreamRedactor) redactStateful(text string) string {
	for _, d := range s.r.detectors {
		text = d.apply(s.r, text, s.counts, s.seen)
	}
	return text
}

// Process reads the upstream SSE body, redacts it, and writes to w, flushing after each line.
func (s *StreamRedactor) Process(w io.Writer, flush func(), body io.Reader) {
	br := bufio.NewReaderSize(body, 64*1024)
	for {
		line, err := br.ReadString('\n')
		if line != "" {
			s.handleLine(w, line)
			flush()
		}
		if err != nil {
			break
		}
	}
	if s.carry != "" {
		s.flushCarry(w)
		flush()
	}
}

func (s *StreamRedactor) handleLine(w io.Writer, line string) {
	trimmed := strings.TrimRight(line, "\r\n")
	if !strings.HasPrefix(trimmed, "data:") {
		io.WriteString(w, line)
		return
	}
	payload := strings.TrimSpace(trimmed[len("data:"):])
	if payload == "" {
		io.WriteString(w, line)
		return
	}
	if payload == "[DONE]" {
		s.flushCarry(w)
		io.WriteString(w, line)
		return
	}
	var obj map[string]any
	if json.Unmarshal([]byte(payload), &obj) != nil {
		io.WriteString(w, line)
		return
	}
	if s.isFlushTrigger(obj) {
		s.flushCarry(w)
	}
	if !s.transform(obj) {
		io.WriteString(w, line)
		return
	}
	nb, err := json.Marshal(obj)
	if err != nil {
		io.WriteString(w, line)
		return
	}
	io.WriteString(w, "data: "+string(nb)+"\n")
}

// transform redacts the text in one event in place; returns whether it changed obj.
func (s *StreamRedactor) transform(obj map[string]any) bool {
	switch s.format {
	case formatAnthropic:
		return s.transformAnthropic(obj)
	case formatOpenAIChat:
		return s.transformOpenAIChat(obj)
	default:
		return s.redactEventInPlace(obj)
	}
}

func (s *StreamRedactor) transformAnthropic(obj map[string]any) bool {
	if t, _ := obj["type"].(string); t == "content_block_delta" {
		d, ok := obj["delta"].(map[string]any)
		if !ok {
			return false
		}
		if txt, ok := d["text"].(string); ok {
			idx := obj["index"]
			s.flushTmpl = func(text string) (string, []byte) {
				b, _ := json.Marshal(map[string]any{
					"type": "content_block_delta", "index": idx,
					"delta": map[string]any{"type": "text_delta", "text": text},
				})
				return "content_block_delta", b
			}
			d["text"] = s.feedText(txt)
			return true
		}
		if pj, ok := d["partial_json"].(string); ok { // tool_use args, per-event
			d["partial_json"] = s.redactStateful(pj)
			return true
		}
	}
	return false
}

func (s *StreamRedactor) transformOpenAIChat(obj map[string]any) bool {
	choices, ok := obj["choices"].([]any)
	if !ok {
		return false
	}
	changed := false
	for _, c := range choices {
		cm, ok := c.(map[string]any)
		if !ok {
			continue
		}
		d, ok := cm["delta"].(map[string]any)
		if !ok {
			continue
		}
		if txt, ok := d["content"].(string); ok {
			s.flushTmpl = func(text string) (string, []byte) {
				b, _ := json.Marshal(map[string]any{
					"choices": []any{map[string]any{"index": 0, "delta": map[string]any{"content": text}}},
				})
				return "", b
			}
			d["content"] = s.feedText(txt)
			changed = true
		}
	}
	return changed
}

// redactEventInPlace redacts obvious text fields per-event (no cross-chunk holdback).
func (s *StreamRedactor) redactEventInPlace(obj map[string]any) bool {
	changed := false
	for _, key := range []string{"delta", "text"} {
		if v, ok := obj[key].(string); ok {
			obj[key] = s.redactStateful(v)
			changed = true
		}
	}
	if d, ok := obj["delta"].(map[string]any); ok {
		for _, key := range []string{"text", "partial_json"} {
			if v, ok := d[key].(string); ok {
				d[key] = s.redactStateful(v)
				changed = true
			}
		}
	}
	return changed
}

// feedText appends to the carry buffer and returns the redaction of the now-safe prefix.
func (s *StreamRedactor) feedText(t string) string {
	s.carry += t
	c := s.carry
	if strings.Count(c, "-----BEGIN") > strings.Count(c, "-----END") {
		return "" // hold an unterminated PEM block until it closes
	}
	i := strings.LastIndexByte(c, '\n')
	if i < 0 {
		return ""
	}
	s.carry = c[i+1:]
	return s.redactStateful(c[:i+1])
}

// flushCarry emits any buffered text as a synthetic delta event built from the last text event.
func (s *StreamRedactor) flushCarry(w io.Writer) {
	if s.carry == "" || s.flushTmpl == nil {
		return
	}
	red := s.redactStateful(s.carry)
	s.carry = ""
	name, data := s.flushTmpl(red)
	if name != "" {
		io.WriteString(w, "event: "+name+"\n")
	}
	io.WriteString(w, "data: "+string(data)+"\n\n")
}

// isFlushTrigger reports events that end a content block, so held text is emitted first.
func (s *StreamRedactor) isFlushTrigger(obj map[string]any) bool {
	switch s.format {
	case formatAnthropic:
		t, _ := obj["type"].(string)
		return t == "content_block_stop" || t == "message_stop"
	case formatOpenAIChat:
		choices, ok := obj["choices"].([]any)
		if !ok {
			return false
		}
		for _, c := range choices {
			if cm, ok := c.(map[string]any); ok {
				if fr, ok := cm["finish_reason"].(string); ok && fr != "" {
					return true
				}
			}
		}
	}
	return false
}

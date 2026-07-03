// SPDX-License-Identifier: Apache-2.0

package proxy

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"

	"github.com/sphragis-oss/sphragis/internal/metrics"
)

// maxUsageLine bounds SSE line buffering; longer lines are skipped, usage events are small.
const maxUsageLine = 256 * 1024

// splitAgent strips an /agent/<name> path prefix, returning the caller name and the API path.
func splitAgent(path string) (agent, rest string) {
	const prefix = "/agent/"
	if !strings.HasPrefix(path, prefix) {
		return "", path
	}
	tail := path[len(prefix):]
	i := strings.IndexByte(tail, '/')
	if i <= 0 {
		return "", path
	}
	name := tail[:i]
	if len(name) > 64 || !validAgentName(name) {
		return "", path
	}
	return name, tail[i:]
}

func validAgentName(name string) bool {
	for _, r := range name {
		ok := r == '.' || r == '_' || r == '-' ||
			(r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
		if !ok {
			return false
		}
	}
	return true
}

// usageBlock covers the Anthropic and OpenAI usage object shapes.
type usageBlock struct {
	InputTokens              int64 `json:"input_tokens"`
	OutputTokens             int64 `json:"output_tokens"`
	CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
	PromptTokens             int64 `json:"prompt_tokens"`
	CompletionTokens         int64 `json:"completion_tokens"`
}

type geminiUsage struct {
	PromptTokenCount     int64 `json:"promptTokenCount"`
	CandidatesTokenCount int64 `json:"candidatesTokenCount"`
}

// usagePayload matches a response body or SSE event from any supported provider.
type usagePayload struct {
	Usage   *usageBlock `json:"usage"`
	Message *struct {
		Usage *usageBlock `json:"usage"`
	} `json:"message"`
	UsageMetadata *geminiUsage `json:"usageMetadata"`
}

// tokenTally accumulates provider-reported token counts for one response.
type tokenTally struct {
	input, output, cacheCreation, cacheRead int64
}

// absorb folds one payload in; providers report cumulative counts, so keep maxima.
func (t *tokenTally) absorb(p usagePayload) {
	for _, u := range []*usageBlock{p.Usage, func() *usageBlock {
		if p.Message != nil {
			return p.Message.Usage
		}
		return nil
	}()} {
		if u == nil {
			continue
		}
		t.input = max(t.input, u.InputTokens, u.PromptTokens)
		t.output = max(t.output, u.OutputTokens, u.CompletionTokens)
		t.cacheCreation = max(t.cacheCreation, u.CacheCreationInputTokens)
		t.cacheRead = max(t.cacheRead, u.CacheReadInputTokens)
	}
	if g := p.UsageMetadata; g != nil {
		t.input = max(t.input, g.PromptTokenCount)
		t.output = max(t.output, g.CandidatesTokenCount)
	}
}

func (t *tokenTally) absorbJSON(body []byte) {
	var p usagePayload
	if json.Unmarshal(body, &p) == nil {
		t.absorb(p)
	}
}

// observe records the tally; zero-valued directions add no metric series.
func (t *tokenTally) observe(agent, model string) {
	for dir, n := range map[string]int64{
		"input": t.input, "output": t.output,
		"cache_creation": t.cacheCreation, "cache_read": t.cacheRead,
	} {
		if n > 0 {
			metrics.ObserveTokens(agent, model, dir, n)
		}
	}
}

// usageScanner tees an SSE stream, folding `data: {...}` events into a tally.
type usageScanner struct {
	w        io.Writer
	tally    *tokenTally
	buf      []byte
	skipping bool
}

func newUsageScanner(w io.Writer, tally *tokenTally) *usageScanner {
	return &usageScanner{w: w, tally: tally}
}

func (s *usageScanner) Write(p []byte) (int, error) {
	n, err := s.w.Write(p)
	if n > 0 {
		s.scan(p[:n])
	}
	return n, err
}

func (s *usageScanner) scan(p []byte) {
	for len(p) > 0 {
		i := bytes.IndexByte(p, '\n')
		if i < 0 {
			if s.skipping || len(s.buf)+len(p) > maxUsageLine {
				s.skipping = true
				s.buf = nil
			} else {
				s.buf = append(s.buf, p...)
			}
			return
		}
		if !s.skipping {
			s.line(append(s.buf, p[:i]...))
		}
		s.buf, s.skipping = nil, false
		p = p[i+1:]
	}
}

func (s *usageScanner) line(line []byte) {
	line = bytes.TrimSpace(line)
	data, ok := bytes.CutPrefix(line, []byte("data:"))
	if !ok {
		return
	}
	s.tally.absorbJSON(bytes.TrimSpace(data))
}

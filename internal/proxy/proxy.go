// SPDX-License-Identifier: Apache-2.0

package proxy

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/sphragis-oss/sphragis/internal/audit"
	"github.com/sphragis-oss/sphragis/internal/redact"
)

type Handler struct {
	Anthropic string
	OpenAI    string
	Override  string
	APIKey    string
	Log       *audit.Log
	Logger    *slog.Logger
	Client    *http.Client
}

func New(anthropic, openai, override, apiKey string, log *audit.Log, logger *slog.Logger) *Handler {
	return &Handler{
		Anthropic: strings.TrimRight(anthropic, "/"),
		OpenAI:    strings.TrimRight(openai, "/"),
		Override:  strings.TrimRight(override, "/"),
		APIKey:    apiKey,
		Log:       log,
		Logger:    logger,
		Client:    &http.Client{},
	}
}

// upstreamFor routes by request path: Anthropic (Claude Code) vs OpenAI (Codex).
func (h *Handler) upstreamFor(path string) string {
	if h.Override != "" {
		return h.Override
	}
	if strings.HasPrefix(path, "/v1/messages") || strings.HasPrefix(path, "/v1/complete") {
		return h.Anthropic
	}
	return h.OpenAI
}

var hopHeaders = map[string]bool{
	"Connection": true, "Proxy-Connection": true, "Keep-Alive": true,
	"Proxy-Authenticate": true, "Proxy-Authorization": true, "Te": true,
	"Trailer": true, "Transfer-Encoding": true, "Upgrade": true,
	"Host": true, "Content-Length": true,
}

// copyHeaders forwards client headers (provider auth: x-api-key, anthropic-version, Authorization).
func copyHeaders(dst, src http.Header) {
	for k, vs := range src {
		if hopHeaders[http.CanonicalHeaderKey(k)] {
			continue
		}
		for _, v := range vs {
			dst.Add(k, v)
		}
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read error", http.StatusBadRequest)
		return
	}
	r.Body.Close()

	model := extractModel(body)
	redacted, counts, err := redact.RedactRequest(r.URL.Path, body)
	if err != nil {
		// Not a JSON request we can parse: forward unchanged, redact nothing.
		redacted = body
		counts = map[redact.Kind]int{}
	}
	sum := sha256.Sum256(redacted)
	payloadHash := hex.EncodeToString(sum[:])

	if h.Log != nil {
		cc := make(map[string]int, len(counts))
		for k, v := range counts {
			cc[string(k)] = v
		}
		if _, err := h.Log.Append(audit.Entry{
			Method:      r.Method,
			Path:        r.URL.Path,
			Model:       model,
			PIIRedacted: cc,
			PayloadHash: payloadHash,
		}); err != nil {
			// Fail closed: never forward a call we could not record.
			h.Logger.Error("audit append failed", "err", err)
			http.Error(w, "audit log error", http.StatusInternalServerError)
			return
		}
	}

	upstream := h.upstreamFor(r.URL.Path)
	req, err := http.NewRequestWithContext(r.Context(), r.Method, upstream+r.URL.Path, bytes.NewReader(redacted))
	if err != nil {
		http.Error(w, "upstream request error", http.StatusInternalServerError)
		return
	}
	copyHeaders(req.Header, r.Header)
	if req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if h.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+h.APIKey)
	}

	resp, err := h.Client.Do(req)
	if err != nil {
		h.Logger.Error("upstream error", "err", err)
		http.Error(w, "upstream error", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	for k, vs := range resp.Header {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	if strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
		flushStream(w, resp.Body)
	} else {
		io.Copy(w, resp.Body)
	}
}

// flushStream copies an SSE body, flushing each read so tokens arrive live.
func flushStream(w http.ResponseWriter, r io.Reader) {
	flusher, _ := w.(http.Flusher)
	buf := make([]byte, 4096)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			w.Write(buf[:n])
			if flusher != nil {
				flusher.Flush()
			}
		}
		if err != nil {
			return
		}
	}
}

func extractModel(body []byte) string {
	var m struct {
		Model string `json:"model"`
	}
	_ = json.Unmarshal(body, &m)
	return m.Model
}

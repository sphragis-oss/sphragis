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
	"strconv"
	"strings"
	"time"

	"github.com/sphragis-oss/sphragis/internal/audit"
	"github.com/sphragis-oss/sphragis/internal/metrics"
	"github.com/sphragis-oss/sphragis/internal/redact"
)

type Handler struct {
	Anthropic  string
	OpenAI     string
	Google     string
	Override   string
	APIKey     string
	Autodetect bool // use auth headers, query key, and model name to route, not just the path
	AutoReveal bool // re-identify [KIND_n] tokens in responses before relaying to the client
	Log        *audit.Log
	Logger     *slog.Logger
	Client     *http.Client
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

// provider resolves the target LLM from the path, plus auth headers, query key, and model name when autodetect is on.
func provider(r *http.Request, model string, autodetect bool) string {
	path := r.URL.Path
	switch {
	case isGeminiPath(path):
		return "google"
	case strings.HasPrefix(path, "/v1/messages"), strings.HasPrefix(path, "/v1/complete"):
		return "anthropic"
	}
	if autodetect {
		switch {
		case r.Header.Get("x-goog-api-key") != "", r.URL.Query().Get("key") != "", strings.HasPrefix(model, "gemini"):
			return "google"
		case r.Header.Get("anthropic-version") != "", strings.HasPrefix(model, "claude"):
			return "anthropic"
		}
	}
	return "openai"
}

// route resolves the upstream base URL and a metrics tag for a request.
func (h *Handler) route(r *http.Request, model string) (upstream, tag string) {
	if h.Override != "" {
		return h.Override, "override"
	}
	switch provider(r, model, h.Autodetect) {
	case "google":
		if h.Google != "" {
			return h.Google, "google"
		}
		return h.OpenAI, "openai"
	case "anthropic":
		return h.Anthropic, "anthropic"
	default:
		return h.OpenAI, "openai"
	}
}

// isGeminiPath reports a Google Generative Language API call.
func isGeminiPath(path string) bool {
	return strings.HasPrefix(path, "/v1beta/") ||
		strings.Contains(path, ":generateContent") ||
		strings.Contains(path, ":streamGenerateContent")
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
	if model == "" {
		model = modelFromPath(r.URL.Path) // Gemini puts the model in the path
	}
	redacted, counts, err := redact.RedactRequest(r.URL.Path, body)
	if err != nil {
		// Not a JSON request we can parse: forward unchanged, redact nothing.
		redacted = body
		counts = map[redact.Kind]int{}
	}
	redact.FlushVault() // persist this request's token assignments once, not per token
	cc := make(map[string]int, len(counts))
	for k, v := range counts {
		cc[string(k)] = v
	}
	metrics.ObserveRedactions("request", cc)
	sum := sha256.Sum256(redacted)
	payloadHash := hex.EncodeToString(sum[:])

	if h.Log != nil {
		if _, err := h.Log.Append(audit.Entry{
			Method:      r.Method,
			Path:        r.URL.Path,
			Model:       model,
			PIIRedacted: cc,
			PayloadHash: payloadHash,
		}); err != nil {
			// Fail closed: never forward a call we could not record.
			metrics.AuditAppendFailed()
			h.Logger.Error("audit append failed", "err", err)
			http.Error(w, "audit log error", http.StatusInternalServerError)
			return
		}
	}

	upstream, tag := h.route(r, model)
	metrics.ObserveRequest(r.URL.Path, tag)
	// RequestURI keeps the query string (Gemini ?key=, Azure ?api-version=).
	req, err := http.NewRequestWithContext(r.Context(), r.Method, upstream+r.URL.RequestURI(), bytes.NewReader(redacted))
	if err != nil {
		http.Error(w, "upstream request error", http.StatusInternalServerError)
		return
	}
	copyHeaders(req.Header, r.Header)
	// Take identity encoding so the response body is plaintext we can redact.
	req.Header.Del("Accept-Encoding")
	if req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if h.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+h.APIKey)
	}

	start := time.Now()
	resp, err := h.Client.Do(req)
	if err != nil {
		metrics.UpstreamError()
		h.Logger.Error("upstream error", "err", err)
		http.Error(w, "upstream error", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	metrics.ObserveResponse(r.URL.Path, resp.StatusCode, time.Since(start))
	h.writeResponse(w, resp, r.URL.Path)
}

// writeResponse redacts the upstream response (SSE or JSON) before relaying it.
func (h *Handler) writeResponse(w http.ResponseWriter, resp *http.Response, path string) {
	ct := resp.Header.Get("Content-Type")
	copyResponseHeaders(w.Header(), resp.Header)

	if strings.HasPrefix(ct, "text/event-stream") {
		w.Header().Del("Content-Length")
		w.WriteHeader(resp.StatusCode)
		flush := func() {}
		if fl, ok := w.(http.Flusher); ok {
			flush = fl.Flush
		}
		sr := redact.NewStreamRedactor(path)
		sr.SetReveal(h.AutoReveal)
		sr.Process(w, flush, resp.Body)
		redact.FlushVault()
		metrics.ObserveRedactions("response", sr.Counts())
		return
	}

	if strings.HasPrefix(ct, "application/json") {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		if red, counts, rerr := redact.RedactResponse(path, body); rerr == nil {
			body = red
			redact.FlushVault()
			metrics.ObserveRedactions("response", kindCounts(counts))
		}
		if h.AutoReveal {
			if rev, rerr := redact.RevealResponse(body); rerr == nil {
				body = rev
			}
		}
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		w.WriteHeader(resp.StatusCode)
		w.Write(body)
		return
	}

	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

// kindCounts converts redaction counts to string-keyed counts for metrics.
func kindCounts(counts map[redact.Kind]int) map[string]int {
	out := make(map[string]int, len(counts))
	for k, v := range counts {
		out[string(k)] = v
	}
	return out
}

func copyResponseHeaders(dst, src http.Header) {
	for k, vs := range src {
		for _, v := range vs {
			dst.Add(k, v)
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

// modelFromPath pulls the model from a Gemini path (/models/<model>:method).
func modelFromPath(path string) string {
	i := strings.Index(path, "/models/")
	if i < 0 {
		return ""
	}
	seg := path[i+len("/models/"):]
	if j := strings.IndexByte(seg, ':'); j >= 0 {
		seg = seg[:j]
	}
	if j := strings.IndexByte(seg, '/'); j >= 0 {
		seg = seg[:j]
	}
	return seg
}

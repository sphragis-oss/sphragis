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
	Upstream string
	APIKey   string
	Log      *audit.Log
	Logger   *slog.Logger
	Client   *http.Client
}

func New(upstream, apiKey string, log *audit.Log, logger *slog.Logger) *Handler {
	return &Handler{
		Upstream: strings.TrimRight(upstream, "/"),
		APIKey:   apiKey,
		Log:      log,
		Logger:   logger,
		Client:   &http.Client{},
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

	req, err := http.NewRequestWithContext(r.Context(), r.Method, h.Upstream+r.URL.Path, bytes.NewReader(redacted))
	if err != nil {
		http.Error(w, "upstream request error", http.StatusInternalServerError)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	if h.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+h.APIKey)
	} else if a := r.Header.Get("Authorization"); a != "" {
		req.Header.Set("Authorization", a)
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

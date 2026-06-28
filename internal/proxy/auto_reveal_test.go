// SPDX-License-Identifier: Apache-2.0

package proxy_test

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sphragis-oss/sphragis/internal/audit"
	"github.com/sphragis-oss/sphragis/internal/proxy"
	"github.com/sphragis-oss/sphragis/internal/redact"
	"github.com/sphragis-oss/sphragis/internal/vault"
)

// withVault attaches a fresh vault to the default redactor and restores none after.
func withVault(t *testing.T) {
	t.Helper()
	var key [32]byte
	v, err := vault.Open(filepath.Join(t.TempDir(), "vault.bin"), key)
	if err != nil {
		t.Fatal(err)
	}
	redact.ConfigureVault(v)
	t.Cleanup(func() { redact.ConfigureVault(nil) })
}

func TestAutoRevealJSONAndAuditStaysRedacted(t *testing.T) {
	withVault(t)
	const email = "secret@corp.com"

	var upstreamGot string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		upstreamGot = string(b)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"content":[{"type":"text","text":"reply to [EMAIL_1]"}]}`))
	}))
	defer upstream.Close()

	logPath := filepath.Join(t.TempDir(), "audit.jsonl")
	lg, _ := audit.Open(logPath)
	defer lg.Close()
	h := proxy.New(upstream.URL, upstream.URL, "", "", lg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	h.AutoReveal = true

	body := `{"model":"claude-3","messages":[{"role":"user","content":"mail ` + email + `"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	// Upstream got the redacted request, never the raw email.
	if strings.Contains(upstreamGot, email) || !strings.Contains(upstreamGot, "[EMAIL_1]") {
		t.Fatalf("request not redacted upstream: %s", upstreamGot)
	}
	// Client got the original back (auto-reveal).
	out := rec.Body.String()
	if !strings.Contains(out, email) || strings.Contains(out, "[EMAIL_1]") {
		t.Fatalf("response not revealed to client: %s", out)
	}
	// Audit log stays redacted: the original must not be on disk.
	logBytes, _ := os.ReadFile(logPath)
	if strings.Contains(string(logBytes), email) {
		t.Fatalf("raw PII leaked into audit log: %s", logBytes)
	}
}

func TestAutoRevealStream(t *testing.T) {
	withVault(t)
	const email = "secret@corp.com"

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		io.WriteString(w, "event: content_block_delta\n"+
			`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"reply to [EMAIL_1]"}}`+"\n\n")
		io.WriteString(w, "event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\n")
	}))
	defer upstream.Close()

	lg, _ := audit.Open(filepath.Join(t.TempDir(), "audit.jsonl"))
	defer lg.Close()
	h := proxy.New(upstream.URL, upstream.URL, "", "", lg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	h.AutoReveal = true

	body := `{"model":"claude-3","stream":true,"messages":[{"role":"user","content":"mail ` + email + `"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	out := rec.Body.String()
	if !strings.Contains(out, email) || strings.Contains(out, "[EMAIL_1]") {
		t.Fatalf("stream not revealed to client: %s", out)
	}
}

func TestAutoRevealOffLeavesTokens(t *testing.T) {
	withVault(t)
	const email = "secret@corp.com"

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"content":[{"type":"text","text":"reply to [EMAIL_1]"}]}`))
	}))
	defer upstream.Close()

	lg, _ := audit.Open(filepath.Join(t.TempDir(), "audit.jsonl"))
	defer lg.Close()
	h := proxy.New(upstream.URL, upstream.URL, "", "", lg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	// AutoReveal defaults to false.

	body := `{"model":"claude-3","messages":[{"role":"user","content":"mail ` + email + `"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	out := rec.Body.String()
	if strings.Contains(out, email) || !strings.Contains(out, "[EMAIL_1]") {
		t.Fatalf("default off must keep tokens in the response: %s", out)
	}
}

// SPDX-License-Identifier: Apache-2.0

package proxy_test

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sphragis-oss/sphragis/internal/audit"
	"github.com/sphragis-oss/sphragis/internal/proxy"
)

func TestProxyRedactsAndAudits(t *testing.T) {
	var gotBody string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer upstream.Close()

	logPath := filepath.Join(t.TempDir(), "a.jsonl")
	lg, err := audit.Open(logPath)
	if err != nil {
		t.Fatal(err)
	}
	defer lg.Close()

	h := proxy.New(upstream.URL, upstream.URL, "", "test-key", lg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	reqBody := `{"model":"gpt-4o","messages":[{"role":"user","content":"email me at a@b.com"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(reqBody))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	if strings.Contains(gotBody, "a@b.com") {
		t.Fatalf("PII leaked to upstream: %s", gotBody)
	}
	if !strings.Contains(gotBody, "[EMAIL_1]") {
		t.Fatalf("expected redaction token in upstream body: %s", gotBody)
	}
	n, _, err := audit.Verify(logPath)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if n != 1 {
		t.Fatalf("want 1 audit record, got %d", n)
	}
}

func TestProxyRedactsAnthropic(t *testing.T) {
	var gotBody string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer upstream.Close()

	logPath := filepath.Join(t.TempDir(), "a.jsonl")
	lg, err := audit.Open(logPath)
	if err != nil {
		t.Fatal(err)
	}
	defer lg.Close()

	h := proxy.New(upstream.URL, upstream.URL, "", "k", lg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	body := `{"model":"claude-3-5-sonnet","messages":[{"role":"user","content":"mail me at a@b.com"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	if strings.Contains(gotBody, "a@b.com") {
		t.Fatalf("PII leaked to upstream on /v1/messages: %s", gotBody)
	}
	if !strings.Contains(gotBody, "[EMAIL_1]") {
		t.Fatalf("expected redaction token: %s", gotBody)
	}
}

func TestProxyRoutesByPath(t *testing.T) {
	hit := func(name string, dst *string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			*dst = name
			w.Write([]byte(`{"ok":true}`))
		}))
	}
	var got string
	anth := hit("anthropic", &got)
	defer anth.Close()
	oai := hit("openai", &got)
	defer oai.Close()

	logPath := filepath.Join(t.TempDir(), "a.jsonl")
	lg, _ := audit.Open(logPath)
	defer lg.Close()
	h := proxy.New(anth.URL, oai.URL, "", "", lg, slog.New(slog.NewTextHandler(io.Discard, nil)))

	cases := []struct{ path, want string }{
		{"/v1/messages", "anthropic"},
		{"/v1/complete", "anthropic"},
		{"/v1/chat/completions", "openai"},
		{"/v1/responses", "openai"},
	}
	for _, c := range cases {
		got = ""
		req := httptest.NewRequest(http.MethodPost, c.path, strings.NewReader(`{"model":"x"}`))
		h.ServeHTTP(httptest.NewRecorder(), req)
		if got != c.want {
			t.Fatalf("%s routed to %q, want %q", c.path, got, c.want)
		}
	}
}

func TestProxyForwardsAnthropicAuthHeaders(t *testing.T) {
	var apiKey, ver string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiKey = r.Header.Get("x-api-key")
		ver = r.Header.Get("anthropic-version")
		w.Write([]byte(`{"ok":true}`))
	}))
	defer upstream.Close()

	logPath := filepath.Join(t.TempDir(), "a.jsonl")
	lg, _ := audit.Open(logPath)
	defer lg.Close()
	h := proxy.New(upstream.URL, upstream.URL, "", "", lg, slog.New(slog.NewTextHandler(io.Discard, nil)))

	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{"model":"claude"}`))
	req.Header.Set("x-api-key", "sk-ant-secret")
	req.Header.Set("anthropic-version", "2023-06-01")
	h.ServeHTTP(httptest.NewRecorder(), req)

	if apiKey != "sk-ant-secret" || ver != "2023-06-01" {
		t.Fatalf("anthropic auth headers not forwarded: key=%q ver=%q", apiKey, ver)
	}
}

func TestProxyStreamsSSE(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fl, _ := w.(http.Flusher)
		for _, chunk := range []string{"event: a\ndata: 1\n\n", "event: b\ndata: 2\n\n"} {
			w.Write([]byte(chunk))
			if fl != nil {
				fl.Flush()
			}
		}
	}))
	defer upstream.Close()

	logPath := filepath.Join(t.TempDir(), "a.jsonl")
	lg, err := audit.Open(logPath)
	if err != nil {
		t.Fatal(err)
	}
	defer lg.Close()

	h := proxy.New(upstream.URL, upstream.URL, "", "k", lg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	body := `{"model":"claude-3","stream":true,"messages":[{"role":"user","content":"hi a@b.com"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("content-type not preserved: %s", ct)
	}
	got := rec.Body.String()
	if !strings.Contains(got, "data: 1") || !strings.Contains(got, "data: 2") {
		t.Fatalf("SSE not passed through: %q", got)
	}
	if !rec.Flushed {
		t.Fatal("expected the stream to be flushed")
	}
}

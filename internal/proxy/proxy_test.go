// SPDX-License-Identifier: Apache-2.0

package proxy_test

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

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

func TestStreamDeliversBeforeUpstreamFinishes(t *testing.T) {
	// Upstream streams a long no-newline paragraph, then pauses before the terminal event.
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fl := w.(http.Flusher)
		words := strings.Fields("the quarterly figures were sent to joe@acme.com along with a very long unbroken paragraph that keeps going without any newline characters at all so the old redactor would have buffered the entire thing")
		for _, word := range words {
			ev := `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"` + word + ` "}}`
			io.WriteString(w, "event: content_block_delta\ndata: "+ev+"\n\n")
			fl.Flush()
		}
		time.Sleep(400 * time.Millisecond) // hold the terminal event
		io.WriteString(w, "event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\n")
		fl.Flush()
	}))
	defer upstream.Close()

	lg, _ := audit.Open(filepath.Join(t.TempDir(), "a.jsonl"))
	defer lg.Close()
	h := proxy.New(upstream.URL, upstream.URL, "", "", lg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	gw := httptest.NewServer(h)
	defer gw.Close()

	body := `{"model":"claude-3","stream":true,"messages":[{"role":"user","content":"hi"}]}`
	resp, err := http.Post(gw.URL+"/v1/messages", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var mu sync.Mutex
	var got []byte
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := resp.Body.Read(buf)
			mu.Lock()
			got = append(got, buf[:n]...)
			mu.Unlock()
			if err != nil {
				return
			}
		}
	}()

	time.Sleep(200 * time.Millisecond) // snapshot mid-pause, before the terminal event is sent
	mu.Lock()
	early := string(got)
	mu.Unlock()

	if !strings.Contains(early, "[EMAIL_1]") {
		t.Fatalf("redacted text not delivered before upstream finished (held back): %q", early)
	}
	if strings.Contains(early, "joe@acme.com") {
		t.Fatalf("raw email leaked into the stream: %q", early)
	}
	if !strings.Contains(early, "quarterly") {
		t.Fatalf("paragraph body not streamed early: %q", early)
	}
}

func TestProxyAutodetectsSharedModelsPath(t *testing.T) {
	hit := func(name string, dst *string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			*dst = name
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"ok":true}`))
		}))
	}
	var got string
	oai := hit("openai", &got)
	defer oai.Close()
	google := hit("google", &got)
	defer google.Close()

	lg, _ := audit.Open(filepath.Join(t.TempDir(), "a.jsonl"))
	defer lg.Close()
	h := proxy.New(oai.URL, oai.URL, "", "", lg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	h.Google = google.URL
	h.Autodetect = true

	// Plain /v1/models has no Gemini signal: stays on OpenAI.
	got = ""
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/v1/models", nil))
	if got != "openai" {
		t.Fatalf("plain /v1/models routed to %q, want openai", got)
	}

	// Same path with a Gemini API-key header autodetects Google.
	got = ""
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.Header.Set("x-goog-api-key", "SECRET")
	h.ServeHTTP(httptest.NewRecorder(), req)
	if got != "google" {
		t.Fatalf("gemini-signalled /v1/models routed to %q, want google", got)
	}

	// And with the ?key= query Gemini uses.
	got = ""
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/v1/models?key=SECRET", nil))
	if got != "google" {
		t.Fatalf("?key= /v1/models routed to %q, want google", got)
	}
}

func TestProxyRoutesGemini(t *testing.T) {
	var hitGoogle bool
	var gotQuery, gotBody string
	google := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hitGoogle = true
		gotQuery = r.URL.RawQuery
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true}`))
	}))
	defer google.Close()
	openai := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("gemini request must not hit the OpenAI upstream")
		w.Write([]byte(`{}`))
	}))
	defer openai.Close()

	logPath := filepath.Join(t.TempDir(), "a.jsonl")
	lg, _ := audit.Open(logPath)
	defer lg.Close()
	h := proxy.New(openai.URL, openai.URL, "", "", lg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	h.Google = google.URL

	body := `{"contents":[{"role":"user","parts":[{"text":"mail a@b.com"}]}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-1.5-pro:generateContent?key=SECRET", strings.NewReader(body))
	h.ServeHTTP(httptest.NewRecorder(), req)

	if !hitGoogle {
		t.Fatal("gemini path not routed to the Google upstream")
	}
	if gotQuery != "key=SECRET" {
		t.Fatalf("query string not forwarded upstream: %q", gotQuery)
	}
	if strings.Contains(gotBody, "a@b.com") || !strings.Contains(gotBody, "[EMAIL_1]") {
		t.Fatalf("gemini body not redacted: %s", gotBody)
	}
	if recs, _ := audit.Recent(logPath, 1); len(recs) != 1 || recs[0].Model != "gemini-1.5-pro" {
		t.Fatalf("model not taken from path: %+v", recs)
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

func TestProxyRedactsJSONResponse(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"reply to a@b.com"}}]}`))
	}))
	defer upstream.Close()

	logPath := filepath.Join(t.TempDir(), "a.jsonl")
	lg, _ := audit.Open(logPath)
	defer lg.Close()
	h := proxy.New(upstream.URL, upstream.URL, "", "k", lg, slog.New(slog.NewTextHandler(io.Discard, nil)))

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-4o","messages":[]}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	got := rec.Body.String()
	if strings.Contains(got, "a@b.com") {
		t.Fatalf("model output PII leaked to client: %s", got)
	}
	if !strings.Contains(got, "[EMAIL_1]") {
		t.Fatalf("expected redacted response: %s", got)
	}
	if cl := rec.Header().Get("Content-Length"); cl != strconv.Itoa(len(got)) {
		t.Fatalf("Content-Length %q does not match body length %d", cl, len(got))
	}
}

func TestProxyRedactsSSEResponse(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fl, _ := w.(http.Flusher)
		chunks := []string{
			"event: content_block_delta\n" + `data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"mail jo"}}` + "\n\n",
			"event: content_block_delta\n" + `data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hn@x.com"}}` + "\n\n",
			"event: content_block_stop\n" + `data: {"type":"content_block_stop","index":0}` + "\n\n",
		}
		for _, c := range chunks {
			w.Write([]byte(c))
			if fl != nil {
				fl.Flush()
			}
		}
	}))
	defer upstream.Close()

	logPath := filepath.Join(t.TempDir(), "a.jsonl")
	lg, _ := audit.Open(logPath)
	defer lg.Close()
	h := proxy.New(upstream.URL, upstream.URL, "", "k", lg, slog.New(slog.NewTextHandler(io.Discard, nil)))

	body := `{"model":"claude-3","stream":true,"messages":[]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	got := rec.Body.String()
	if strings.Contains(got, "john@x.com") {
		t.Fatalf("model output PII leaked in stream: %s", got)
	}
	if !strings.Contains(got, "[EMAIL_1]") {
		t.Fatalf("expected redacted token in stream: %s", got)
	}
	if !rec.Flushed {
		t.Fatal("expected the stream to be flushed")
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

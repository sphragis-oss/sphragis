// SPDX-License-Identifier: Apache-2.0

package ui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sphragis-oss/sphragis/internal/audit"
	"github.com/sphragis-oss/sphragis/internal/redact"
)

func newTestHandler(t *testing.T) (*Handler, string) {
	t.Helper()
	logPath := filepath.Join(t.TempDir(), "audit.jsonl")
	return New(redact.NewConfigured(nil, true, false), logPath), logPath
}

func TestServePage(t *testing.T) {
	h, _ := newTestHandler(t)
	mux := http.NewServeMux()
	h.Register(mux)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/ui", nil))
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "Sphragis") {
		t.Fatalf("page: code=%d body=%.40q", rr.Code, rr.Body.String())
	}
}

func TestRedactEndpoint(t *testing.T) {
	h, _ := newTestHandler(t)
	mux := http.NewServeMux()
	h.Register(mux)
	body := strings.NewReader(`{"text":"mail a@b.com vat EL123456783"}`)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/ui/redact", body))
	if rr.Code != http.StatusOK {
		t.Fatalf("redact code=%d", rr.Code)
	}
	var resp redactResp
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(resp.Text, "a@b.com") || !strings.Contains(resp.Text, "[EMAIL_1]") {
		t.Fatalf("email not redacted: %q", resp.Text)
	}
	if resp.Counts["EMAIL"] != 1 || resp.Counts["VAT"] != 1 {
		t.Fatalf("counts wrong (EU pack should apply): %v", resp.Counts)
	}
}

func TestAuditEndpoint(t *testing.T) {
	h, logPath := newTestHandler(t)
	log, err := audit.Open(logPath)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if _, err := log.Append(audit.Entry{Method: "POST", Path: "/v1/messages", Model: "claude", PayloadHash: "h", PIIRedacted: map[string]int{"EMAIL": 1}}); err != nil {
			t.Fatal(err)
		}
	}
	log.Close()

	mux := http.NewServeMux()
	h.Register(mux)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/ui/audit", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("audit code=%d", rr.Code)
	}
	var resp auditResp
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.Exists || resp.Records != 2 || !resp.ChainOK {
		t.Fatalf("audit summary wrong: %+v", resp)
	}
	if len(resp.Recent) != 2 || resp.Recent[0].Seq != 2 {
		t.Fatalf("recent should be newest-first, 2 records: %+v", resp.Recent)
	}
	if resp.PerKind["EMAIL"] != 2 {
		t.Fatalf("perKind wrong: %v", resp.PerKind)
	}
}

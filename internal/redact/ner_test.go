// SPDX-License-Identifier: Apache-2.0

package redact

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNERDetector(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := map[string]any{"entities": []map[string]string{
			{"type": "PERSON", "text": "Maria Papadopoulou"},
			{"type": "LOCATION", "text": "Athens"},
		}}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	r := New(nil)
	r.SetNER(srv.URL)
	res := r.Redact("Maria Papadopoulou from Athens emailed a@b.com")
	if res.Counts[Name] != 1 || res.Counts[Address] != 1 {
		t.Fatalf("want name=1 address=1, got %v", res.Counts)
	}
	if res.Counts[Email] != 1 {
		t.Fatalf("regex detectors must still run, email=%d", res.Counts[Email])
	}
	if strings.Contains(res.Text, "Maria Papadopoulou") || strings.Contains(res.Text, "Athens") {
		t.Fatalf("NER PII leaked: %s", res.Text)
	}
}

func TestNEROverlappingEntitiesAndTokenSafety(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := map[string]any{"entities": []map[string]string{
			{"type": "PERSON", "text": "Maria"},
			{"type": "PERSON", "text": "Maria Papadopoulou"},
			{"type": "MISC", "text": "1"}, // would corrupt [EMAIL_1] under naive replace
		}}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	r := New(nil)
	r.SetNER(srv.URL)
	res := r.Redact("Maria Papadopoulou wrote to a@b.com")
	if strings.Contains(res.Text, "Papadopoulou") {
		t.Fatalf("substring leak: %q", res.Text)
	}
	if !strings.Contains(res.Text, "[EMAIL_1]") {
		t.Fatalf("existing token corrupted: %q", res.Text)
	}
	if res.Counts[Name] != 1 {
		t.Fatalf("want name=1, got %v", res.Counts)
	}
}

func TestNERFailOpen(t *testing.T) {
	r := New(nil)
	r.SetNER("http://127.0.0.1:1")
	res := r.Redact("hello a@b.com")
	if res.Counts[Email] != 1 {
		t.Fatalf("regex must still work when NER is down: %v", res.Counts)
	}
}

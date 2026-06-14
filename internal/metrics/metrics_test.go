// SPDX-License-Identifier: Apache-2.0

package metrics

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestExposition(t *testing.T) {
	ObserveRequest("/v1/messages", "anthropic")
	ObserveResponse("/v1/messages", 200, 120*time.Millisecond)
	ObserveRedactions("request", map[string]int{"EMAIL": 2})
	AuditAppendFailed()

	rec := httptest.NewRecorder()
	Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/metrics", nil))
	body := rec.Body.String()

	for _, want := range []string{
		`sphragis_requests_total{path="/v1/messages",upstream="anthropic"} 1`,
		`sphragis_redactions_total{kind="EMAIL",direction="request"} 2`,
		`sphragis_audit_append_failures_total 1`,
		`sphragis_upstream_request_duration_seconds_bucket{path="/v1/messages",le="0.25"} 1`,
		`sphragis_upstream_request_duration_seconds_count{path="/v1/messages"} 1`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("exposition missing %q\n---\n%s", want, body)
		}
	}
}

func TestNormalizePathBoundsCardinality(t *testing.T) {
	if got := NormalizePath("/v1/foo/bar"); got != "other" {
		t.Fatalf("unknown path should be 'other', got %q", got)
	}
	if got := NormalizePath("/v1/messages/count_tokens"); got != "/v1/messages/count_tokens" {
		t.Fatalf("got %q", got)
	}
}

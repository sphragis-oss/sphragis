// SPDX-License-Identifier: Apache-2.0

package proxy_test

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/sphragis-oss/sphragis/internal/audit"
	"github.com/sphragis-oss/sphragis/internal/proxy"
)

// TestRequestNeverLeaksRealPIIToUpstream proves the gateway strips PII from the
// request body BEFORE forwarding to the LLM. Run: go test ./internal/proxy/ -run LeaksRealPII -v
func TestRequestNeverLeaksRealPIIToUpstream(t *testing.T) {
	// Real PII a user might paste into a prompt; none of it may reach the LLM.
	const (
		email = "john.doe@gmail.com"
		ssn   = "123-45-6789"
		card  = "4111 1111 1111 1111"
		phone = "+1 415 5551234"
	)

	// Stand-in LLM: records the exact bytes it receives so we can inspect them.
	var mu sync.Mutex
	var upstreamGot string
	llm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		upstreamGot = string(b)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true}`))
	}))
	defer llm.Close()

	lg, err := audit.Open(filepath.Join(t.TempDir(), "audit.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer lg.Close()

	// Real gateway over real TCP so the hop is a genuine network request.
	h := proxy.New(llm.URL, llm.URL, "", "", lg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	gw := httptest.NewServer(h)
	defer gw.Close()

	clientSent := `{"model":"claude-3","messages":[{"role":"user","content":` +
		`"My email is ` + email + `, SSN ` + ssn + `, card ` + card + `, call ` + phone + `"}]}`
	resp, err := http.Post(gw.URL+"/v1/messages", "application/json", strings.NewReader(clientSent))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	mu.Lock()
	got := upstreamGot
	mu.Unlock()

	t.Logf("\n--- CLIENT  ->  GATEWAY (what you typed) ---\n%s", clientSent)
	t.Logf("\n--- GATEWAY ->  LLM (what actually left the box) ---\n%s", got)

	// No raw PII may appear in what the LLM received.
	for _, raw := range []string{email, ssn, card, phone} {
		if strings.Contains(got, raw) {
			t.Fatalf("LEAK: %q reached the LLM:\n%s", raw, got)
		}
	}
	// Each value must have become a reversible token instead.
	for _, tok := range []string{"[EMAIL_1]", "[SSN_1]", "[CARD_1]", "[PHONE_1]"} {
		if !strings.Contains(got, tok) {
			t.Fatalf("expected %s in upstream body:\n%s", tok, got)
		}
	}
	t.Log("\nPASS: all four PII values were tokenized; none reached the LLM.")
}

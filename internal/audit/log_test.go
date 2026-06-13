// SPDX-License-Identifier: Apache-2.0

package audit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func appendN(t *testing.T, l *Log, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		if _, err := l.Append(Entry{Method: "POST", Path: "/v1/chat/completions", Model: "gpt-4o", PayloadHash: strings.Repeat("a", 64)}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestAppendAndVerify(t *testing.T) {
	p := filepath.Join(t.TempDir(), "a.jsonl")
	l, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	appendN(t, l, 3)
	l.Close()

	n, root, err := Verify(p)
	if err != nil {
		t.Fatalf("verify failed: %v", err)
	}
	if n != 3 {
		t.Fatalf("want 3 records, got %d", n)
	}
	if root == "" {
		t.Fatal("want non-empty merkle root")
	}
}

func TestRecoverSeqOnReopen(t *testing.T) {
	p := filepath.Join(t.TempDir(), "a.jsonl")
	l, _ := Open(p)
	appendN(t, l, 2)
	l.Close()

	l2, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	r, err := l2.Append(Entry{Method: "POST", Path: "/v1/x", PayloadHash: strings.Repeat("b", 64)})
	if err != nil {
		t.Fatal(err)
	}
	l2.Close()
	if r.Seq != 3 {
		t.Fatalf("want seq 3 after reopen, got %d", r.Seq)
	}
	if n, _, err := Verify(p); err != nil || n != 3 {
		t.Fatalf("verify after reopen: n=%d err=%v", n, err)
	}
}

func TestVerifyDetectsTamper(t *testing.T) {
	p := filepath.Join(t.TempDir(), "a.jsonl")
	l, _ := Open(p)
	appendN(t, l, 3)
	l.Close()

	raw, _ := os.ReadFile(p)
	tampered := strings.Replace(string(raw), `"model":"gpt-4o"`, `"model":"evil"`, 1)
	if tampered == string(raw) {
		t.Fatal("test setup: nothing replaced")
	}
	os.WriteFile(p, []byte(tampered), 0o600)

	if _, _, err := Verify(p); err == nil {
		t.Fatal("expected verify to detect tampering, got nil error")
	}
}

func TestVerifyDetectsPIICountTamper(t *testing.T) {
	p := filepath.Join(t.TempDir(), "a.jsonl")
	l, _ := Open(p)
	if _, err := l.Append(Entry{Method: "POST", Path: "/v1/messages", Model: "claude", PIIRedacted: map[string]int{"EMAIL": 1}, PayloadHash: strings.Repeat("a", 64)}); err != nil {
		t.Fatal(err)
	}
	l.Close()

	raw, _ := os.ReadFile(p)
	tampered := strings.Replace(string(raw), `"EMAIL":1`, `"EMAIL":0`, 1)
	if tampered == string(raw) {
		t.Fatal("test setup: nothing replaced")
	}
	os.WriteFile(p, []byte(tampered), 0o600)

	if _, _, err := Verify(p); err == nil {
		t.Fatal("expected verify to detect pii-count tampering, got nil error")
	}
}

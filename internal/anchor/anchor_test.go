// SPDX-License-Identifier: Apache-2.0

package anchor

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nbd-wtf/opentimestamps"
	"github.com/sphragis-oss/sphragis/internal/audit"
)

func writeLog(t *testing.T, n int) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "a.jsonl")
	l, err := audit.Open(p)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < n; i++ {
		if _, err := l.Append(audit.Entry{Method: "POST", Path: "/v1/x", PayloadHash: strings.Repeat("a", 64)}); err != nil {
			t.Fatal(err)
		}
	}
	l.Close()
	return p
}

func TestRootDigest(t *testing.T) {
	d, n, err := RootDigest(writeLog(t, 3))
	if err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Fatalf("want 3 records, got %d", n)
	}
	if d == ([32]byte{}) {
		t.Fatal("root digest is zero")
	}
}

func TestRootDigestEmpty(t *testing.T) {
	if _, _, err := RootDigest(writeLog(t, 0)); err == nil {
		t.Fatal("empty log should error")
	}
}

func TestAnchorWritesProof(t *testing.T) {
	p := writeLog(t, 2)
	ots := p + ".ots"
	stamp := func(_ context.Context, _ string, d [32]byte) (opentimestamps.Sequence, error) {
		return opentimestamps.Sequence{}, nil
	}
	root, err := Anchor(context.Background(), p, ots, []string{"cal"}, stamp)
	if err != nil {
		t.Fatal(err)
	}
	if len(root) != 64 {
		t.Fatalf("root should be 64 hex chars, got %q", root)
	}
	if _, err := os.Stat(ots); err != nil {
		t.Fatalf("proof file not written: %v", err)
	}
}

func TestAnchorAllCalendarsFail(t *testing.T) {
	stamp := func(_ context.Context, _ string, _ [32]byte) (opentimestamps.Sequence, error) {
		return nil, errors.New("boom")
	}
	if _, err := Anchor(context.Background(), writeLog(t, 2), filepath.Join(t.TempDir(), "x.ots"), []string{"cal"}, stamp); err == nil {
		t.Fatal("expected error when all calendars fail")
	}
}

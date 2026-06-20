// SPDX-License-Identifier: Apache-2.0

package audit

import (
	"path/filepath"
	"testing"
)

func TestRecentCapsAndOrders(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	if recs, err := Recent(path, 5); err != nil || recs != nil {
		t.Fatalf("missing log should be empty: recs=%v err=%v", recs, err)
	}
	log, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 7; i++ {
		if _, err := log.Append(Entry{Method: "POST", Path: "/v1/messages", PayloadHash: "h"}); err != nil {
			t.Fatal(err)
		}
	}
	log.Close()

	recs, err := Recent(path, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 3 || recs[0].Seq != 5 || recs[2].Seq != 7 {
		t.Fatalf("want last 3 (seq 5..7) oldest-first: %+v", recs)
	}
}

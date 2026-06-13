// SPDX-License-Identifier: Apache-2.0

package audit

import (
	"strings"
	"testing"
)

func TestMerkleRoot(t *testing.T) {
	a := strings.Repeat("11", 32)
	b := strings.Repeat("22", 32)

	if MerkleRoot(nil) != "" {
		t.Fatal("empty leaves should give empty root")
	}
	if MerkleRoot([]string{a}) != a {
		t.Fatal("single leaf should be its own root")
	}
	two := MerkleRoot([]string{a, b})
	if two == a || two == b || len(two) != 64 {
		t.Fatalf("two-leaf root looks wrong: %s", two)
	}
	if MerkleRoot([]string{a, b}) != two {
		t.Fatal("merkle root must be deterministic")
	}
	if MerkleRoot([]string{a, b, a}) == two {
		t.Fatal("different leaf set should give different root")
	}
}

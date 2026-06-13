// SPDX-License-Identifier: Apache-2.0

package audit

import (
	"crypto/sha256"
	"encoding/hex"
)

// MerkleRoot folds leaf hashes pairwise, duplicating the last on an odd count.
func MerkleRoot(leaves []string) string {
	if len(leaves) == 0 {
		return ""
	}
	level := append([]string(nil), leaves...)
	for len(level) > 1 {
		var next []string
		for i := 0; i < len(level); i += 2 {
			if i+1 == len(level) {
				next = append(next, pairHash(level[i], level[i]))
			} else {
				next = append(next, pairHash(level[i], level[i+1]))
			}
		}
		level = next
	}
	return level[0]
}

func pairHash(a, b string) string {
	ab, _ := hex.DecodeString(a)
	bb, _ := hex.DecodeString(b)
	h := sha256.New()
	h.Write(ab)
	h.Write(bb)
	return hex.EncodeToString(h.Sum(nil))
}

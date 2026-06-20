// SPDX-License-Identifier: Apache-2.0

package audit

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// Recent returns up to n of the most recent records, oldest-first. A missing
// log yields no records and no error.
func Recent(path string, n int) ([]Record, error) {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	var recs []Record
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var r Record
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			return nil, fmt.Errorf("corrupt audit line: %w", err)
		}
		recs = append(recs, r)
		if len(recs) > n {
			recs = recs[1:]
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return recs, nil
}

// Verify replays the log checking sequence, links and hashes; returns count and Merkle root.
func Verify(path string) (uint64, string, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, "", err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	prev := genesisHash
	var expectSeq uint64
	var leaves []string
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var r Record
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			return 0, "", fmt.Errorf("corrupt json near record %d: %w", expectSeq+1, err)
		}
		expectSeq++
		if r.Seq != expectSeq {
			return 0, "", fmt.Errorf("seq %d: out of order (expected %d)", r.Seq, expectSeq)
		}
		if r.PrevHash != prev {
			return 0, "", fmt.Errorf("seq %d: broken chain link", r.Seq)
		}
		if chainHash(r) != r.Hash {
			return 0, "", fmt.Errorf("seq %d: hash mismatch, record tampered", r.Seq)
		}
		leaves = append(leaves, r.PayloadHash)
		prev = r.Hash
	}
	if err := sc.Err(); err != nil {
		return 0, "", err
	}
	return expectSeq, MerkleRoot(leaves), nil
}

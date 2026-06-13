// SPDX-License-Identifier: Apache-2.0

package audit

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
)

// Summary is an at-a-glance view of an audit log for status reporting.
type Summary struct {
	Exists     bool
	Records    uint64
	ChainOK    bool
	ChainErr   error
	PerKind    map[string]int
	LastTime   string
	MerkleRoot string
}

// Summarize reads the log once, verifying the chain and aggregating counts.
func Summarize(path string) (Summary, error) {
	s := Summary{PerKind: map[string]int{}, ChainOK: true}
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	defer f.Close()
	s.Exists = true
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
			s.ChainOK, s.ChainErr = false, fmt.Errorf("corrupt json near record %d: %w", expectSeq+1, err)
			return s, nil
		}
		expectSeq++
		if r.Seq != expectSeq {
			s.ChainOK, s.ChainErr = false, fmt.Errorf("seq %d: out of order (expected %d)", r.Seq, expectSeq)
			return s, nil
		}
		if r.PrevHash != prev || chainHash(r) != r.Hash {
			s.ChainOK, s.ChainErr = false, fmt.Errorf("seq %d: chain broken or record tampered", r.Seq)
			return s, nil
		}
		for k, v := range r.PIIRedacted {
			s.PerKind[k] += v
		}
		s.LastTime = r.Time
		leaves = append(leaves, r.PayloadHash)
		prev = r.Hash
	}
	if err := sc.Err(); err != nil {
		return s, err
	}
	s.Records = expectSeq
	s.MerkleRoot = MerkleRoot(leaves)
	return s, nil
}

// SortedKinds returns the PerKind keys sorted by descending count then name.
func (s Summary) SortedKinds() []string {
	keys := make([]string, 0, len(s.PerKind))
	for k := range s.PerKind {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if s.PerKind[keys[i]] != s.PerKind[keys[j]] {
			return s.PerKind[keys[i]] > s.PerKind[keys[j]]
		}
		return keys[i] < keys[j]
	})
	return keys
}

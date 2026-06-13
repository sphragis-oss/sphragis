// SPDX-License-Identifier: Apache-2.0

package audit

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

const genesisHash = "0000000000000000000000000000000000000000000000000000000000000000"

type Record struct {
	Seq         uint64         `json:"seq"`
	Time        string         `json:"time"`
	Method      string         `json:"method"`
	Path        string         `json:"path"`
	Model       string         `json:"model"`
	PIIRedacted map[string]int `json:"pii_redacted"`
	PayloadHash string         `json:"payload_hash"`
	PrevHash    string         `json:"prev_hash"`
	Hash        string         `json:"hash"`
}

// Entry is the caller-supplied portion of a record.
type Entry struct {
	Method      string
	Path        string
	Model       string
	PIIRedacted map[string]int
	PayloadHash string
}

type Log struct {
	mu       sync.Mutex
	f        *os.File
	lastHash string
	seq      uint64
}

func chainHash(r Record) string {
	in := fmt.Sprintf("%d\n%s\n%s\n%s\n%s\n%s\n%s",
		r.Seq, r.Time, r.Method, r.Path, r.Model, r.PayloadHash, r.PrevHash)
	sum := sha256.Sum256([]byte(in))
	return hex.EncodeToString(sum[:])
}

// Open opens or creates an append-only audit log, recovering chain state.
func Open(path string) (*Log, error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return nil, err
	}
	l := &Log{f: f, lastHash: genesisHash}
	if _, err := f.Seek(0, 0); err != nil {
		f.Close()
		return nil, err
	}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var r Record
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			f.Close()
			return nil, fmt.Errorf("corrupt audit line: %w", err)
		}
		l.seq = r.Seq
		l.lastHash = r.Hash
	}
	if err := sc.Err(); err != nil {
		f.Close()
		return nil, err
	}
	if _, err := f.Seek(0, 2); err != nil {
		f.Close()
		return nil, err
	}
	return l, nil
}

// Append writes one hash-chained record and fsyncs before returning.
func (l *Log) Append(e Entry) (Record, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	r := Record{
		Seq:         l.seq + 1,
		Time:        time.Now().UTC().Format(time.RFC3339Nano),
		Method:      e.Method,
		Path:        e.Path,
		Model:       e.Model,
		PIIRedacted: e.PIIRedacted,
		PayloadHash: e.PayloadHash,
		PrevHash:    l.lastHash,
	}
	r.Hash = chainHash(r)
	line, err := json.Marshal(r)
	if err != nil {
		return Record{}, err
	}
	if _, err := l.f.Write(append(line, '\n')); err != nil {
		return Record{}, err
	}
	if err := l.f.Sync(); err != nil {
		return Record{}, err
	}
	l.seq = r.Seq
	l.lastHash = r.Hash
	return r, nil
}

func (l *Log) Close() error { return l.f.Close() }

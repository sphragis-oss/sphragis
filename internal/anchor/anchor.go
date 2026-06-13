// SPDX-License-Identifier: Apache-2.0

package anchor

import (
	"context"
	"encoding/hex"
	"errors"
	"os"

	"github.com/nbd-wtf/opentimestamps"
	"github.com/sphragis-oss/sphragis/internal/audit"
)

// DefaultCalendars are the public OpenTimestamps aggregators.
var DefaultCalendars = []string{
	"https://alice.btc.calendar.opentimestamps.org",
	"https://bob.btc.calendar.opentimestamps.org",
}

// StampFunc stamps a digest at one calendar; swappable for tests.
type StampFunc func(ctx context.Context, calendarURL string, digest [32]byte) (opentimestamps.Sequence, error)

// RootDigest verifies the audit log and returns its Merkle root as 32 bytes.
func RootDigest(logPath string) ([32]byte, uint64, error) {
	var digest [32]byte
	n, root, err := audit.Verify(logPath)
	if err != nil {
		return digest, 0, err
	}
	if n == 0 {
		return digest, 0, errors.New("audit log is empty, nothing to anchor")
	}
	b, err := hex.DecodeString(root)
	if err != nil || len(b) != 32 {
		return digest, 0, errors.New("invalid merkle root")
	}
	copy(digest[:], b)
	return digest, n, nil
}

// Anchor verifies the log, stamps its Merkle root at the calendars and writes a .ots proof.
func Anchor(ctx context.Context, logPath, otsPath string, calendars []string, stamp StampFunc) (string, error) {
	digest, _, err := RootDigest(logPath)
	if err != nil {
		return "", err
	}
	if stamp == nil {
		stamp = opentimestamps.Stamp
	}
	if len(calendars) == 0 {
		calendars = DefaultCalendars
	}
	var seqs []opentimestamps.Sequence
	for _, cal := range calendars {
		seq, err := stamp(ctx, cal, digest)
		if err != nil {
			continue
		}
		seqs = append(seqs, seq)
	}
	if len(seqs) == 0 {
		return "", errors.New("all calendar servers failed")
	}
	file := opentimestamps.File{Digest: digest[:], Sequences: seqs}
	if err := os.WriteFile(otsPath, file.SerializeToFile(), 0o644); err != nil {
		return "", err
	}
	return hex.EncodeToString(digest[:]), nil
}

// SPDX-License-Identifier: Apache-2.0

package redact

import "regexp"

// builtins run in order; overlap-sensitive (IBAN before CARD, keyed secrets before bare keys).
var builtins = []detector{
	{kind: PrivateKey, re: regexp.MustCompile(`(?s)-----BEGIN [A-Z0-9 ]*PRIVATE KEY-----.*?-----END [A-Z0-9 ]*PRIVATE KEY-----`)},
	{kind: JWT, re: regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\b`)},
	{kind: Secret, group: 1, re: regexp.MustCompile(`(?i)\b(?:password|passwd|pwd|secret|api[_-]?key|access[_-]?token|auth[_-]?token|client[_-]?secret|token)\b["']?\s*[:=]\s*["']?([^"'\s,;}{]{3,})`)},
	{kind: Secret, group: 1, re: regexp.MustCompile(`(?i)\bbearer\s+([A-Za-z0-9._-]{8,})`)},
	{kind: APIKey, re: regexp.MustCompile(`\bsk-(?:ant-|proj-)?[A-Za-z0-9_-]{20,}\b`)},
	{kind: APIKey, re: regexp.MustCompile(`\b(?:AKIA|ASIA|AGPA|AIDA|AROA|AIPA|ANPA|ANVA)[A-Z0-9]{16}\b`)},
	{kind: APIKey, re: regexp.MustCompile(`\b(?:ghp|gho|ghu|ghs|ghr)_[A-Za-z0-9]{36}\b`)},
	{kind: APIKey, re: regexp.MustCompile(`\bgithub_pat_[A-Za-z0-9_]{22,}\b`)},
	{kind: APIKey, re: regexp.MustCompile(`\bAIza[0-9A-Za-z_-]{35}\b`)},
	{kind: APIKey, re: regexp.MustCompile(`\bxox[baprs]-[A-Za-z0-9-]{10,}\b`)},
	{kind: APIKey, re: regexp.MustCompile(`\b(?:sk|pk|rk)_(?:live|test)_[A-Za-z0-9]{16,}\b`)},
	{kind: APIKey, re: regexp.MustCompile(`\bSG\.[A-Za-z0-9_-]{22}\.[A-Za-z0-9_-]{43}\b`)},
	{kind: SSN, re: regexp.MustCompile(`\b\d{3}-\d{2}-\d{4}\b`)},
	{kind: IBAN, re: regexp.MustCompile(`\b[A-Z]{2}\d{2}(?:[ ]?[A-Z0-9]{2,4}){2,8}\b`)},
	{kind: Card, valid: luhnValid, re: regexp.MustCompile(`\b\d(?:[ -]?\d){12,18}\b`)},
	{kind: IP, re: regexp.MustCompile(`\b(?:(?:25[0-5]|2[0-4]\d|1?\d?\d)\.){3}(?:25[0-5]|2[0-4]\d|1?\d?\d)\b`)},
	{kind: Email, re: regexp.MustCompile(`\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}\b`)},
	{kind: Phone, re: regexp.MustCompile(`\+\d{1,3}[ ]\d{2,4}[ ]\d{5,8}`)},
}

// luhnValid checks the Luhn checksum to cut false-positive card matches.
func luhnValid(s string) bool {
	sum, alt, digits := 0, false, 0
	for i := len(s) - 1; i >= 0; i-- {
		c := s[i]
		if c < '0' || c > '9' {
			continue
		}
		d := int(c - '0')
		digits++
		if alt {
			if d *= 2; d > 9 {
				d -= 9
			}
		}
		sum += d
		alt = !alt
	}
	return digits >= 13 && sum%10 == 0
}

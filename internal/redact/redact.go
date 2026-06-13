// SPDX-License-Identifier: Apache-2.0

package redact

import (
	"regexp"
	"strconv"
	"strings"
)

type Kind string

const (
	Email      Kind = "EMAIL"
	Phone      Kind = "PHONE"
	IBAN       Kind = "IBAN"
	Card       Kind = "CARD"
	Secret     Kind = "SECRET"
	APIKey     Kind = "APIKEY"
	PrivateKey Kind = "PRIVATEKEY"
	JWT        Kind = "JWT"
	SSN        Kind = "SSN"
	IP         Kind = "IP"
	Name       Kind = "NAME"
	Address    Kind = "ADDRESS"
	Health     Kind = "HEALTH"
)

type detector struct {
	kind  Kind
	re    *regexp.Regexp
	group int               // if >0, redact only this capture group (keep the rest)
	valid func(string) bool // optional validator; failing matches are left intact
}

type Result struct {
	Text   string
	Counts map[Kind]int
}

// Redactor applies an ordered set of detectors. Build one with New.
type Redactor struct {
	detectors []detector
	ner       *nerClient
}

// New builds a Redactor from the built-ins plus optional exact-match custom terms.
func New(customTerms []string) *Redactor {
	ds := append([]detector(nil), builtins...)
	var quoted []string
	for _, t := range customTerms {
		if t = strings.TrimSpace(t); t != "" {
			quoted = append(quoted, regexp.QuoteMeta(t))
		}
	}
	if len(quoted) > 0 {
		re := regexp.MustCompile(`(?i)\b(?:` + strings.Join(quoted, "|") + `)\b`)
		ds = append(ds, detector{kind: Name, re: re})
	}
	return &Redactor{detectors: ds}
}

var defaultRedactor = New(nil)

// Configure swaps the default redactor at startup; not concurrency-safe.
func Configure(customTerms []string) { defaultRedactor = New(customTerms) }

// Redact replaces detected sensitive values with stable [KIND_n] tokens.
func (r *Redactor) Redact(s string) Result {
	counts := map[Kind]int{}
	seen := map[Kind]map[string]int{}
	for _, d := range r.detectors {
		s = d.apply(s, counts, seen)
	}
	if r.ner != nil {
		s = r.ner.redact(s, counts, seen)
	}
	return Result{Text: s, Counts: counts}
}

// SetNER attaches an external NER service for name/address/health detection.
func (r *Redactor) SetNER(url string) {
	if url == "" {
		r.ner = nil
		return
	}
	r.ner = newNERClient(url)
}

// ConfigureNER attaches an NER service to the default redactor.
func ConfigureNER(url string) { defaultRedactor.SetNER(url) }

// Redact runs the default redactor.
func Redact(s string) Result { return defaultRedactor.Redact(s) }

func (d detector) apply(s string, counts map[Kind]int, seen map[Kind]map[string]int) string {
	locs := d.re.FindAllStringSubmatchIndex(s, -1)
	if locs == nil {
		return s
	}
	var b strings.Builder
	last := 0
	for _, loc := range locs {
		gs, ge := loc[0], loc[1]
		if d.group > 0 {
			i := 2 * d.group
			if i+1 >= len(loc) || loc[i] < 0 {
				continue
			}
			gs, ge = loc[i], loc[i+1]
		}
		val := s[gs:ge]
		if d.valid != nil && !d.valid(val) {
			continue
		}
		if seen[d.kind] == nil {
			seen[d.kind] = map[string]int{}
		}
		n, ok := seen[d.kind][val]
		if !ok {
			n = len(seen[d.kind]) + 1
			seen[d.kind][val] = n
		}
		counts[d.kind]++
		b.WriteString(s[last:gs])
		b.WriteString("[" + string(d.kind) + "_" + strconv.Itoa(n) + "]")
		last = ge
	}
	b.WriteString(s[last:])
	return b.String()
}

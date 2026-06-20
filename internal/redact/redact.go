// SPDX-License-Identifier: Apache-2.0

package redact

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/sphragis-oss/sphragis/internal/vault"
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
	VAT        Kind = "VAT"
	AMKA       Kind = "AMKA"
	TaxID      Kind = "TAXID"
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
	vault     *vault.Vault
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

// BuiltinCount returns the number of built-in detectors.
func BuiltinCount() int { return len(builtins) }

// Configure swaps the default redactor at startup; not concurrency-safe.
// With euPack set, the opt-in EU detectors run before the built-ins.
func Configure(customTerms []string, euPack bool) {
	r := New(customTerms)
	if euPack {
		r.detectors = append(append([]detector(nil), euDetectors...), r.detectors...)
	}
	defaultRedactor = r
}

// Redact replaces detected sensitive values with stable [KIND_n] tokens.
func (r *Redactor) Redact(s string) Result {
	counts := map[Kind]int{}
	seen := map[Kind]map[string]int{}
	for _, d := range r.detectors {
		s = d.apply(r, s, counts, seen)
	}
	if r.ner != nil {
		s = r.ner.redact(r, s, counts, seen)
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

// SetVault enables reversible tokenization with gateway-global token numbering.
func (r *Redactor) SetVault(v *vault.Vault) { r.vault = v }

// ConfigureVault enables the reversible-tokenization vault on the default redactor.
func ConfigureVault(v *vault.Vault) { defaultRedactor.SetVault(v) }

// assign returns the bare token (e.g. "EMAIL_1") for a value. With a vault,
// numbering is gateway-global and the original is recorded; otherwise it is
// per-field and ephemeral.
func (r *Redactor) assign(kind Kind, val string, seen map[Kind]map[string]int) string {
	if r.vault != nil {
		return r.vault.Assign(string(kind), val)
	}
	if seen[kind] == nil {
		seen[kind] = map[string]int{}
	}
	n, ok := seen[kind][val]
	if !ok {
		n = len(seen[kind]) + 1
		seen[kind][val] = n
	}
	return string(kind) + "_" + strconv.Itoa(n)
}

// Redact runs the default redactor.
func Redact(s string) Result { return defaultRedactor.Redact(s) }

func (d detector) apply(r *Redactor, s string, counts map[Kind]int, seen map[Kind]map[string]int) string {
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
		counts[d.kind]++
		b.WriteString(s[last:gs])
		b.WriteString("[" + r.assign(d.kind, val, seen) + "]")
		last = ge
	}
	b.WriteString(s[last:])
	return b.String()
}

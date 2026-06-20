// SPDX-License-Identifier: Apache-2.0

package redact

import (
	_ "embed"
	"regexp"
	"strings"
)

//go:embed names.txt
var givenNamesRaw string

// nerDetectors is the opt-in, dependency-free name/address pack (precision-biased).
var nerDetectors = buildNERDetectors()

func buildNERDetectors() []detector {
	names := parseGivenNames(givenNamesRaw)
	surname := `[A-Z][\p{L}'’-]+`
	fullName := `[A-Z][\p{L}'’-]+(?:\s+[A-Z][\p{L}'’-]+)?`

	// gazetteer given name followed by a capitalized surname
	gazetteer := regexp.MustCompile(`\b(?:` + strings.Join(names, "|") + `)\b\s+` + surname)
	// a title or honorific introduces a name
	titled := regexp.MustCompile(`\b(?:Mr|Mrs|Ms|Miss|Mx|Dr|Prof|Sir|Dame)\.?\s+(` + fullName + `)`)
	// trigger phrase introduces a name; phrase is case-insensitive, the name stays capitalized
	trigger := regexp.MustCompile(`(?:(?i:name is|named|patient|client|customer|signed by))\s+(` + fullName + `)`)
	// number, capitalized words, then a street suffix
	address := regexp.MustCompile(`\b\d{1,5}\s+(?:[A-Z][\p{L}'’-]+\s+){1,3}(?:Street|St|Avenue|Ave|Road|Rd|Boulevard|Blvd|Lane|Ln|Drive|Way|Square|Sq|Court|Ct|Place|Pl)\b\.?`)

	return []detector{
		{kind: Name, re: gazetteer},
		{kind: Name, group: 1, re: titled},
		{kind: Name, group: 1, re: trigger},
		{kind: Address, re: address},
	}
}

// parseGivenNames returns the regex-quoted, non-comment names from the gazetteer.
func parseGivenNames(raw string) []string {
	var out []string
	for _, line := range strings.Split(raw, "\n") {
		if line = strings.TrimSpace(line); line != "" && !strings.HasPrefix(line, "#") {
			out = append(out, regexp.QuoteMeta(line))
		}
	}
	return out
}

// SPDX-License-Identifier: Apache-2.0

package redact

import "regexp"

// euDetectors is the opt-in EU pack. It runs before the built-ins so the
// country-prefixed VAT matcher claims values before the generic IBAN matcher.
var euDetectors = []detector{
	{kind: VAT, re: regexp.MustCompile(`\b(?:ATU\d{8}|BE0\d{9}|BG\d{9,10}|CY\d{8}[A-Z]|CZ\d{8,10}|DE\d{9}|DK\d{8}|EE\d{9}|EL\d{9}|ES[0-9A-Z]\d{7}[0-9A-Z]|FI\d{8}|FR[0-9A-Z]{2}\d{9}|HR\d{11}|HU\d{8}|IE\d{7}[A-Z]{1,2}|IT\d{11}|LT(?:\d{9}|\d{12})|LU\d{8}|LV\d{11}|MT\d{8}|NL\d{9}B\d{2}|PL\d{10}|PT\d{9}|RO\d{2,10}|SE\d{12}|SI\d{8}|SK\d{10})\b`)},
	{kind: AMKA, valid: amkaValid, re: regexp.MustCompile(`\b\d{11}\b`)},
	{kind: TaxID, valid: afmValid, re: regexp.MustCompile(`\b\d{9}\b`)},
}

// amkaValid checks a Greek AMKA: 11 digits, a plausible DDMMYY birthdate
// prefix, and a valid Luhn checksum.
func amkaValid(s string) bool {
	if len(s) != 11 {
		return false
	}
	dd := int(s[0]-'0')*10 + int(s[1]-'0')
	mm := int(s[2]-'0')*10 + int(s[3]-'0')
	if dd < 1 || dd > 31 || mm < 1 || mm > 12 {
		return false
	}
	return luhnOK(s)
}

// afmValid checks a Greek AFM (tax id): 9 digits with the official weighted
// modulo-11 check digit.
func afmValid(s string) bool {
	if len(s) != 9 || s == "000000000" {
		return false
	}
	sum := 0
	for i := 0; i < 8; i++ {
		sum += int(s[i]-'0') << (8 - i)
	}
	return (sum%11)%10 == int(s[8]-'0')
}

// luhnOK is the plain Luhn checksum (no minimum-length gate, unlike luhnValid).
func luhnOK(s string) bool {
	sum, alt := 0, false
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] < '0' || s[i] > '9' {
			continue
		}
		d := int(s[i] - '0')
		if alt {
			if d *= 2; d > 9 {
				d -= 9
			}
		}
		sum += d
		alt = !alt
	}
	return sum%10 == 0
}

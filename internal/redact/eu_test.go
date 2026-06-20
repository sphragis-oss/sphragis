// SPDX-License-Identifier: Apache-2.0

package redact

import (
	"strings"
	"testing"
)

// euRedactor builds a redactor with the EU pack prepended, like Configure(_, true).
func euRedactor() *Redactor {
	r := New(nil)
	r.detectors = append(append([]detector(nil), euDetectors...), r.detectors...)
	return r
}

func TestEUPackRedactsAMKA(t *testing.T) {
	res := euRedactor().Redact("amka 01019000122 on file")
	if res.Counts[AMKA] != 1 {
		t.Fatalf("AMKA not redacted: %q counts=%v", res.Text, res.Counts)
	}
	if strings.Contains(res.Text, "01019000122") {
		t.Fatalf("AMKA value leaked: %q", res.Text)
	}
}

func TestEUPackRedactsAFM(t *testing.T) {
	res := euRedactor().Redact("tax id 123456783 here")
	if res.Counts[TaxID] != 1 {
		t.Fatalf("AFM not redacted: %q counts=%v", res.Text, res.Counts)
	}
}

func TestEUPackRedactsVAT(t *testing.T) {
	for _, vat := range []string{"EL123456783", "DE123456789", "IT12345678901"} {
		res := euRedactor().Redact("invoice " + vat + " total")
		if res.Counts[VAT] != 1 {
			t.Fatalf("VAT %q not redacted: %q counts=%v", vat, res.Text, res.Counts)
		}
	}
}

func TestEUPackRejectsInvalidChecksums(t *testing.T) {
	// wrong AFM check digit, bad AMKA Luhn, and an implausible AMKA date
	res := euRedactor().Redact("nums 123456784 01019000123 99019000122 end")
	if res.Counts[TaxID] != 0 || res.Counts[AMKA] != 0 {
		t.Fatalf("invalid EU ids should pass through: %q counts=%v", res.Text, res.Counts)
	}
}

func TestEUPackDisabledByDefault(t *testing.T) {
	res := New(nil).Redact("amka 01019000122 tax 123456783")
	if res.Counts[AMKA] != 0 || res.Counts[TaxID] != 0 {
		t.Fatalf("EU ids redacted without the pack: counts=%v", res.Counts)
	}
}

func TestConfigureEnablesEUPack(t *testing.T) {
	defer Configure(nil, false)
	Configure(nil, true)
	if res := Redact("vat EL123456783"); res.Counts[VAT] != 1 {
		t.Fatalf("Configure euPack not applied: %q counts=%v", res.Text, res.Counts)
	}
}

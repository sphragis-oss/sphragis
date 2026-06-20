// SPDX-License-Identifier: Apache-2.0

package redact

import (
	"strings"
	"testing"
)

func nerRedactor() *Redactor { return NewConfigured(nil, false, true) }

func TestBuiltinNERGazetteer(t *testing.T) {
	res := nerRedactor().Redact("please cc John Smith on the thread")
	if res.Counts[Name] != 1 || strings.Contains(res.Text, "John Smith") {
		t.Fatalf("gazetteer name not redacted: %q %v", res.Text, res.Counts)
	}
}

func TestBuiltinNERTitleAndTrigger(t *testing.T) {
	res := nerRedactor().Redact("Dr. Robertson met the patient Anderson today")
	if res.Counts[Name] != 2 {
		t.Fatalf("title/trigger names not redacted: %q %v", res.Text, res.Counts)
	}
	if strings.Contains(res.Text, "Robertson") || strings.Contains(res.Text, "Anderson") {
		t.Fatalf("name leaked: %q", res.Text)
	}
}

func TestBuiltinNERAddress(t *testing.T) {
	res := nerRedactor().Redact("ship to 42 Oak Street and bill me")
	if res.Counts[Address] != 1 || strings.Contains(res.Text, "42 Oak Street") {
		t.Fatalf("address not redacted: %q %v", res.Text, res.Counts)
	}
}

func TestBuiltinNERDisabledByDefault(t *testing.T) {
	res := New(nil).Redact("cc John Smith, ship to 42 Oak Street")
	if res.Counts[Name] != 0 || res.Counts[Address] != 0 {
		t.Fatalf("NER ran without the pack: %v", res.Counts)
	}
}

func TestBuiltinNERLeavesPlainCapitalizedWords(t *testing.T) {
	res := nerRedactor().Redact("Our Quarterly Report covers New Features")
	if res.Counts[Name] != 0 {
		t.Fatalf("over-redacted capitalized words: %q %v", res.Text, res.Counts)
	}
}

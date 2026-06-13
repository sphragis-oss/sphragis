// SPDX-License-Identifier: Apache-2.0

package redact

import "testing"

func FuzzRedact(f *testing.F) {
	seeds := []string{
		"",
		"contact john@example.com or +1 415 5551234",
		"IBAN GB82 WEST 1234 5698 7654 32",
		"card 4111 1111 1111 1111 ssn 123-45-6789",
		"token: sk-ant-api03-abc123 host 10.0.0.1",
		"-----BEGIN PRIVATE KEY-----\nMIIBVg\n-----END PRIVATE KEY-----",
		"[EMAIL_1] already redacted [CARD_2]",
	}
	for _, s := range seeds {
		f.Add(s)
	}
	r := New([]string{"projectx", "blue falcon"})
	f.Fuzz(func(t *testing.T, s string) {
		res := r.Redact(s)
		for k, n := range res.Counts {
			if n < 0 {
				t.Fatalf("negative count for %s on input %q", k, s)
			}
		}
	})
}

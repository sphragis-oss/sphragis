// SPDX-License-Identifier: Apache-2.0

package redact

import "encoding/json"

// RevealResponse re-identifies tokens in JSON string values via the vault, re-marshaling so originals stay escaped.
func RevealResponse(body []byte) ([]byte, error) {
	if defaultRedactor.vault == nil {
		return body, nil
	}
	var v any
	if err := json.Unmarshal(body, &v); err != nil {
		return nil, err
	}
	return json.Marshal(revealAny(v))
}

// revealAny reveals tokens in every string of an arbitrary JSON value.
func revealAny(v any) any {
	switch x := v.(type) {
	case string:
		return defaultRedactor.vault.Reveal(x)
	case []any:
		for i, e := range x {
			x[i] = revealAny(e)
		}
		return x
	case map[string]any:
		for k, e := range x {
			x[k] = revealAny(e)
		}
		return x
	default:
		return v
	}
}

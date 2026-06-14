// SPDX-License-Identifier: Apache-2.0

package redact

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

type nerClient struct {
	url  string
	http *http.Client
}

type nerEntity struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func newNERClient(url string) *nerClient {
	return &nerClient{url: url, http: &http.Client{}}
}

// nerKind maps common entity types to redaction kinds.
func nerKind(t string) Kind {
	switch strings.ToUpper(t) {
	case "PERSON", "PER", "NAME":
		return Name
	case "LOCATION", "LOC", "GPE", "ADDRESS":
		return Address
	case "MEDICAL", "HEALTH", "CONDITION", "DIAGNOSIS", "MEDICAL_LICENSE":
		return Health
	default:
		return Kind(strings.ToUpper(t))
	}
}

// redact calls the external NER service and tokenizes returned entities; it is
// best-effort and fails open so an NER outage never blocks regex redaction.
func (n *nerClient) redact(r *Redactor, s string, counts map[Kind]int, seen map[Kind]map[string]int) string {
	body, _ := json.Marshal(map[string]string{"text": s})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.url, bytes.NewReader(body))
	if err != nil {
		return s
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := n.http.Do(req)
	if err != nil {
		return s
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return s
	}
	var out struct {
		Entities []nerEntity `json:"entities"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return s
	}
	for _, e := range out.Entities {
		if e.Text == "" {
			continue
		}
		c := strings.Count(s, e.Text)
		if c == 0 {
			continue
		}
		k := nerKind(e.Type)
		tok := r.assign(k, e.Text, seen)
		counts[k] += c
		s = strings.ReplaceAll(s, e.Text, "["+tok+"]")
	}
	return s
}

// SPDX-License-Identifier: Apache-2.0

package redact

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"
)

// nerTokenRe matches an already-emitted redaction token so NER never rewrites one.
var nerTokenRe = regexp.MustCompile(`\[[A-Z][A-Z_]*_\d+\]`)

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
	return replaceEntities(r, s, out.Entities, counts, seen)
}

// replaceEntities tokenizes NER entities longest-first, only inside plain-text spans, leaving existing tokens intact.
func replaceEntities(r *Redactor, s string, entities []nerEntity, counts map[Kind]int, seen map[Kind]map[string]int) string {
	kindOf := map[string]Kind{}
	var texts []string
	for _, e := range entities {
		if e.Text == "" {
			continue
		}
		if _, dup := kindOf[e.Text]; dup {
			continue
		}
		kindOf[e.Text] = nerKind(e.Type)
		texts = append(texts, e.Text)
	}
	if len(texts) == 0 {
		return s
	}
	// longest first so "Maria Papadopoulou" wins over its substring "Maria"
	sort.SliceStable(texts, func(i, j int) bool { return len(texts[i]) > len(texts[j]) })
	quoted := make([]string, len(texts))
	for i, t := range texts {
		quoted[i] = regexp.QuoteMeta(t)
	}
	re := regexp.MustCompile(strings.Join(quoted, "|"))

	var b strings.Builder
	last := 0
	for _, span := range nerTokenRe.FindAllStringIndex(s, -1) {
		b.WriteString(replacePlain(r, s[last:span[0]], re, kindOf, counts, seen))
		b.WriteString(s[span[0]:span[1]]) // copy an existing token through untouched
		last = span[1]
	}
	b.WriteString(replacePlain(r, s[last:], re, kindOf, counts, seen))
	return b.String()
}

func replacePlain(r *Redactor, s string, re *regexp.Regexp, kindOf map[string]Kind, counts map[Kind]int, seen map[Kind]map[string]int) string {
	return re.ReplaceAllStringFunc(s, func(m string) string {
		k := kindOf[m]
		counts[k]++
		return "[" + r.assign(k, m, seen) + "]"
	})
}

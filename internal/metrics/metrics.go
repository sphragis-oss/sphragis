// SPDX-License-Identifier: Apache-2.0

// Package metrics exposes gateway counters in Prometheus text format with no
// external dependencies, keeping the binary lean and the supply chain small.
package metrics

import (
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

var durationBuckets = []float64{0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}

var (
	requests   = newCounter("sphragis_requests_total", "Requests received by upstream route.", "path", "upstream")
	responses  = newCounter("sphragis_responses_total", "Responses returned by status class.", "path", "code")
	redactions = newCounter("sphragis_redactions_total", "Values redacted by kind and direction.", "kind", "direction")
	auditFails = newCounter("sphragis_audit_append_failures_total", "Audit-log append failures (fail-closed events).")
	upErrors   = newCounter("sphragis_upstream_errors_total", "Upstream request failures.")
	duration   = newHistogram("sphragis_upstream_request_duration_seconds", "Upstream round-trip latency in seconds.", durationBuckets, "path")
)

var collectors = []collector{requests, responses, redactions, auditFails, upErrors, duration}

// ObserveRequest counts one received request for a route.
func ObserveRequest(path, upstream string) { requests.add(1, NormalizePath(path), upstream) }

// ObserveResponse counts one response and records its upstream latency.
func ObserveResponse(path string, status int, d time.Duration) {
	p := NormalizePath(path)
	responses.add(1, p, statusClass(status))
	duration.observe(d.Seconds(), p)
}

// ObserveRedactions adds redaction counts for a direction (request or response).
func ObserveRedactions(direction string, counts map[string]int) {
	for kind, n := range counts {
		redactions.add(float64(n), kind, direction)
	}
}

// AuditAppendFailed counts one fail-closed audit write.
func AuditAppendFailed() { auditFails.add(1) }

// UpstreamError counts one upstream failure.
func UpstreamError() { upErrors.add(1) }

// Handler serves the Prometheus text exposition.
func Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		for _, c := range collectors {
			c.write(w)
		}
	})
}

// NormalizePath maps a request path to a known route, bounding label cardinality.
func NormalizePath(path string) string {
	known := []string{
		"/v1/messages/count_tokens", "/v1/messages/batches", "/v1/messages",
		"/v1/complete", "/v1/chat/completions", "/v1/responses",
	}
	for _, k := range known {
		if strings.HasPrefix(path, k) {
			return k
		}
	}
	return "other"
}

func statusClass(status int) string {
	switch {
	case status >= 500:
		return "5xx"
	case status >= 400:
		return "4xx"
	case status >= 300:
		return "3xx"
	case status >= 200:
		return "2xx"
	default:
		return "1xx"
	}
}

type collector interface{ write(io.Writer) }

type counterVec struct {
	name, help string
	labels     []string
	mu         sync.Mutex
	vals       map[string]float64
}

func newCounter(name, help string, labels ...string) *counterVec {
	return &counterVec{name: name, help: help, labels: labels, vals: map[string]float64{}}
}

func (c *counterVec) add(v float64, lvs ...string) {
	c.mu.Lock()
	c.vals[strings.Join(lvs, "\x00")] += v
	c.mu.Unlock()
}

func (c *counterVec) write(w io.Writer) {
	fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s counter\n", c.name, c.help, c.name)
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.labels) == 0 {
		fmt.Fprintf(w, "%s %g\n", c.name, c.vals[""])
		return
	}
	for _, key := range sortedKeys(c.vals) {
		fmt.Fprintf(w, "%s{%s} %g\n", c.name, labelPairs(c.labels, key), c.vals[key])
	}
}

type histogramVec struct {
	name, help string
	labels     []string
	buckets    []float64
	mu         sync.Mutex
	counts     map[string][]uint64
	sums       map[string]float64
}

func newHistogram(name, help string, buckets []float64, labels ...string) *histogramVec {
	return &histogramVec{
		name: name, help: help, labels: labels, buckets: buckets,
		counts: map[string][]uint64{}, sums: map[string]float64{},
	}
}

func (h *histogramVec) observe(v float64, lvs ...string) {
	key := strings.Join(lvs, "\x00")
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.counts[key] == nil {
		h.counts[key] = make([]uint64, len(h.buckets)+1)
	}
	h.sums[key] += v
	i := sort.SearchFloat64s(h.buckets, v)
	h.counts[key][i]++
}

func (h *histogramVec) write(w io.Writer) {
	fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s histogram\n", h.name, h.help, h.name)
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, key := range sortedKeys(h.counts) {
		pairs := labelPairs(h.labels, key)
		var cumulative uint64
		for i, b := range h.buckets {
			cumulative += h.counts[key][i]
			fmt.Fprintf(w, "%s_bucket{%s,le=\"%g\"} %d\n", h.name, pairs, b, cumulative)
		}
		cumulative += h.counts[key][len(h.buckets)]
		fmt.Fprintf(w, "%s_bucket{%s,le=\"+Inf\"} %d\n", h.name, pairs, cumulative)
		fmt.Fprintf(w, "%s_sum{%s} %g\n", h.name, pairs, h.sums[key])
		fmt.Fprintf(w, "%s_count{%s} %d\n", h.name, pairs, cumulative)
	}
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// labelPairs renders name="value" pairs; %q escapes \, " and newlines.
func labelPairs(names []string, key string) string {
	vals := strings.Split(key, "\x00")
	parts := make([]string, len(names))
	for i, n := range names {
		v := ""
		if i < len(vals) {
			v = vals[i]
		}
		parts[i] = fmt.Sprintf("%s=%q", n, v)
	}
	return strings.Join(parts, ",")
}

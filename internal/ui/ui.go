// SPDX-License-Identifier: Apache-2.0

package ui

import (
	_ "embed"
	"encoding/json"
	"net/http"

	"github.com/sphragis-oss/sphragis/internal/audit"
	"github.com/sphragis-oss/sphragis/internal/redact"
)

//go:embed index.html
var page []byte

// Handler serves the read-only web UI: a redaction playground and an audit view.
type Handler struct {
	preview *redact.Redactor
	logPath string
}

// New builds a UI handler; the vault-free preview means the playground never mutates state.
func New(preview *redact.Redactor, logPath string) *Handler {
	return &Handler{preview: preview, logPath: logPath}
}

// Register wires the UI routes onto mux.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /ui", h.servePage)
	mux.HandleFunc("POST /ui/redact", h.redact)
	mux.HandleFunc("GET /ui/audit", h.auditView)
}

func (h *Handler) servePage(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(page)
}

type redactReq struct {
	Text string `json:"text"`
}

type redactResp struct {
	Text   string         `json:"text"`
	Counts map[string]int `json:"counts"`
}

// redact previews redaction on submitted text. It does not log or forward.
func (h *Handler) redact(w http.ResponseWriter, r *http.Request) {
	var req redactReq
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	res := h.preview.Redact(req.Text)
	counts := make(map[string]int, len(res.Counts))
	for k, v := range res.Counts {
		counts[string(k)] = v
	}
	writeJSON(w, redactResp{Text: res.Text, Counts: counts})
}

type auditResp struct {
	Exists     bool           `json:"exists"`
	Records    uint64         `json:"records"`
	ChainOK    bool           `json:"chainOk"`
	ChainErr   string         `json:"chainErr,omitempty"`
	MerkleRoot string         `json:"merkleRoot"`
	LastTime   string         `json:"lastTime"`
	PerKind    map[string]int `json:"perKind"`
	Recent     []audit.Record `json:"recent"`
}

func (h *Handler) auditView(w http.ResponseWriter, _ *http.Request) {
	sum, err := audit.Summarize(h.logPath)
	if err != nil {
		http.Error(w, "audit read error", http.StatusInternalServerError)
		return
	}
	recent, err := audit.Recent(h.logPath, 20)
	if err != nil {
		http.Error(w, "audit read error", http.StatusInternalServerError)
		return
	}
	// newest first for display
	for i, j := 0, len(recent)-1; i < j; i, j = i+1, j-1 {
		recent[i], recent[j] = recent[j], recent[i]
	}
	resp := auditResp{
		Exists:     sum.Exists,
		Records:    sum.Records,
		ChainOK:    sum.ChainOK,
		MerkleRoot: sum.MerkleRoot,
		LastTime:   sum.LastTime,
		PerKind:    sum.PerKind,
		Recent:     recent,
	}
	if sum.ChainErr != nil {
		resp.ChainErr = sum.ChainErr.Error()
	}
	writeJSON(w, resp)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

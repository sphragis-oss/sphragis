// SPDX-License-Identifier: Apache-2.0

package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sphragis-oss/sphragis/internal/audit"
	"github.com/sphragis-oss/sphragis/internal/config"
	"github.com/sphragis-oss/sphragis/internal/control"
	"github.com/sphragis-oss/sphragis/internal/redact"
)

// errSilent signals main to exit non-zero without printing (already reported).
var errSilent = errors.New("reported")

func statusCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "status",
		Short:   "Display gateway, redaction, and audit status",
		GroupID: groupGateway,
		Args:    cobra.NoArgs,
		RunE:    func(_ *cobra.Command, _ []string) error { return runStatus() },
	}
}

func runStatus() error {
	cfg := config.FromEnv()
	st := control.LoadState()
	var errs, warns []string
	var rows []string

	rows = append(rows, paint(cBold, "Sphragis")+paint(cDim, "  EU AI Act compliance gateway"), "")

	if pid, ok := control.Running(); ok {
		rows = append(rows, row("Gateway", paint(cGreen, "running")+fmt.Sprintf(" (pid %d)", pid)))
	} else {
		rows = append(rows, row("Gateway", paint(cYellow, "stopped")))
		warns = append(warns, "gateway is not running")
	}
	rows = append(rows, row("Listen", cfg.ListenAddr))
	if cfg.UpstreamBaseURL != "" {
		rows = append(rows, row("Upstream", cfg.UpstreamBaseURL+paint(cDim, "  (override, all routes)")))
	} else {
		rows = append(rows, row("Claude Code", cfg.AnthropicBaseURL+paint(cDim, "  (/v1/messages)")))
		rows = append(rows, row("Codex/OpenAI", cfg.OpenAIBaseURL+paint(cDim, "  (/v1/chat, /v1/responses)")))
	}

	sum, err := audit.Summarize(cfg.AuditLogPath)
	switch {
	case err != nil:
		rows = append(rows, row("Audit log", paint(cRed, "error: "+err.Error())))
		errs = append(errs, "audit log: "+err.Error())
	case !sum.Exists:
		rows = append(rows, row("Audit log", paint(cDim, "none yet")))
	case !sum.ChainOK:
		rows = append(rows, row("Audit log", paint(cRed, fmt.Sprintf("%s records, CHAIN BROKEN", humanInt(sum.Records)))))
		errs = append(errs, "audit chain: "+sum.ChainErr.Error())
	default:
		rows = append(rows, row("Audit log", fmt.Sprintf("%s records, %s", humanInt(sum.Records), paint(cGreen, "chain intact"))))
	}

	if st.AutoAnchor {
		rows = append(rows, row("Auto-anchor", paint(cGreen, "on")+" (every "+st.Interval+")"))
	} else {
		rows = append(rows, row("Auto-anchor", paint(cDim, "off")))
	}

	red := fmt.Sprintf("%d builtin", redact.BuiltinCount())
	if cfg.CustomTermsFile != "" {
		if terms, err := config.LoadCustomTerms(cfg.CustomTermsFile); err == nil {
			red += fmt.Sprintf(" + %d custom", len(terms))
		}
	}
	if cfg.NERURL != "" {
		red += " + NER"
	} else {
		red += ", NER off"
	}
	rows = append(rows, row("Redactors", red))

	if sum.Exists && len(sum.PerKind) > 0 {
		var parts []string
		for _, k := range sum.SortedKinds() {
			parts = append(parts, fmt.Sprintf("%s %d", k, sum.PerKind[k]))
		}
		rows = append(rows, row("Redacted", paint(cDim, "(total) ")+strings.Join(parts, "  ")))
	}

	if sum.Exists && sum.Records > 0 {
		if _, err := os.Stat(cfg.AuditLogPath + ".ots"); err != nil {
			warns = append(warns, "audit log has never been anchored (sphragis anchor now)")
		}
	}

	rows = append(rows, "")
	rows = append(rows, issueRows("Errors", errs, cRed)...)
	rows = append(rows, issueRows("Warnings", warns, cYellow)...)

	printSideBySide(rows)
	if len(errs) > 0 {
		return errSilent
	}
	return nil
}

func issueRows(label string, items []string, color string) []string {
	if len(items) == 0 {
		return []string{row(label, paint(cDim, "0"))}
	}
	out := []string{row(label, paint(color, fmt.Sprintf("%d", len(items))))}
	for _, it := range items {
		out = append(out, "  "+paint(color, "-")+" "+it)
	}
	return out
}

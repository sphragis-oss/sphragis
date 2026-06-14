// SPDX-License-Identifier: Apache-2.0

package main

import "github.com/spf13/cobra"

const (
	groupGateway = "gateway"
	groupAudit   = "audit"
	groupOther   = "other"
)

func rootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "sphragis",
		Short:         "EU AI Act compliance gateway",
		Long:          "Sphragis - EU AI Act compliance gateway\n\nA drop-in LLM proxy that redacts personal data locally and writes a\ntamper-evident, hash-chained audit log of every model call.",
		Example:       "  sphragis start          # start the gateway in the background\n  sphragis status         # show health, redaction stats, audit chain\n  sphragis verify <log>   # verify the audit-log chain",
		SilenceErrors: true,
		SilenceUsage:  true,
		Version:       version,
	}
	root.SetVersionTemplate("sphragis {{.Version}}\n")
	root.AddGroup(
		&cobra.Group{ID: groupGateway, Title: "Gateway Commands:"},
		&cobra.Group{ID: groupAudit, Title: "Audit Commands:"},
		&cobra.Group{ID: groupOther, Title: "Other Commands:"},
	)
	root.SetHelpCommandGroupID(groupOther)
	root.SetCompletionCommandGroupID(groupOther)
	root.AddCommand(
		startCmd(), stopCmd(), restartCmd(), serveCmd(), statusCmd(),
		verifyCmd(), anchorCmd(), revealCmd(),
		versionCmd(),
	)
	return root
}

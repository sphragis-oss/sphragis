// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/sphragis-oss/sphragis/internal/audit"
)

func verifyCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "verify <audit-log-path>",
		Short:   "Verify the audit-log hash chain",
		GroupID: groupAudit,
		Args:    cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			n, root, err := audit.Verify(args[0])
			if err != nil {
				return err
			}
			fmt.Printf("%s %s records, chain intact\n", paint(cGreen, "OK:"), humanInt(n))
			fmt.Printf("merkle_root: %s\n", root)
			return nil
		},
	}
}

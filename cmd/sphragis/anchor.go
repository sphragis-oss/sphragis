// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/sphragis-oss/sphragis/internal/anchor"
	"github.com/sphragis-oss/sphragis/internal/config"
	"github.com/sphragis-oss/sphragis/internal/control"
)

func anchorCmd() *cobra.Command {
	c := &cobra.Command{
		Use:     "anchor",
		Short:   "Anchor the audit-log Merkle root to Bitcoin (OpenTimestamps)",
		GroupID: groupAudit,
	}
	c.AddCommand(anchorNowCmd(), anchorOnCmd(), anchorOffCmd(), anchorStatusCmd())
	return c
}

func anchorNowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "now [audit-log-path]",
		Short: "Anchor the Merkle root once, now",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			cfg := config.FromEnv()
			logPath := cfg.AuditLogPath
			if len(args) == 1 {
				logPath = args[0]
			}
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			root, err := anchor.Anchor(ctx, logPath, logPath+".ots", cfg.OTSCalendars, nil)
			if err != nil {
				return err
			}
			fmt.Printf("anchored merkle_root %s\nproof: %s (pending; run `ots upgrade %s` later)\n", root, logPath+".ots", logPath+".ots")
			return nil
		},
	}
}

func anchorOnCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "on [interval]",
		Short: "Enable automatic periodic anchoring (e.g. 24h)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			st := control.LoadState()
			st.AutoAnchor = true
			if len(args) == 1 {
				st.Interval = args[0]
			}
			if err := control.SaveState(st); err != nil {
				return err
			}
			fmt.Printf("auto-anchor: on (interval %s)\n", st.Interval)
			return nil
		},
	}
}

func anchorOffCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "off",
		Short: "Disable automatic anchoring",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			st := control.LoadState()
			st.AutoAnchor = false
			if err := control.SaveState(st); err != nil {
				return err
			}
			fmt.Println("auto-anchor: off")
			return nil
		},
	}
}

func anchorStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show auto-anchor state",
		Args:  cobra.NoArgs,
		Run: func(_ *cobra.Command, _ []string) {
			st := control.LoadState()
			fmt.Printf("auto-anchor: %v (interval %s)\n", st.AutoAnchor, st.Interval)
		},
	}
}

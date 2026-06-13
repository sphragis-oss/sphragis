// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
)

// version is set at build time via -ldflags -X main.version.
var version = "dev"

func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "version",
		Short:   "Display version information",
		GroupID: groupOther,
		Args:    cobra.NoArgs,
		Run: func(_ *cobra.Command, _ []string) {
			fmt.Printf("sphragis %s\n", version)
			fmt.Printf("  go:   %s\n", runtime.Version())
			fmt.Printf("  os:   %s/%s\n", runtime.GOOS, runtime.GOARCH)
		},
	}
}

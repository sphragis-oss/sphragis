// SPDX-License-Identifier: Apache-2.0

package main

import (
	"errors"
	"fmt"
	"os"
)

func main() {
	if err := rootCmd().Execute(); err != nil {
		if !errors.Is(err, errSilent) {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
		}
		os.Exit(1)
	}
}

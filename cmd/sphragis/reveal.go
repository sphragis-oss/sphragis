// SPDX-License-Identifier: Apache-2.0

package main

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/sphragis-oss/sphragis/internal/config"
	"github.com/sphragis-oss/sphragis/internal/vault"
)

func revealCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "reveal [file]",
		Short:   "Restore original values in redacted text using the local vault",
		Long:    "Reverse tokenization with the sealed local vault. Reads a redacted file (or stdin) and writes the rehydrated text to stdout. Requires the vault key (SPHRAGIS_VAULT_KEY or SPHRAGIS_VAULT_KEYFILE).",
		GroupID: groupOther,
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			key, ok, err := vault.LoadKey(os.Getenv("SPHRAGIS_VAULT_KEY"), cfg.VaultKeyfile)
			if err != nil {
				return err
			}
			if !ok {
				return errors.New("no vault key set (SPHRAGIS_VAULT_KEY or SPHRAGIS_VAULT_KEYFILE)")
			}
			v, err := vault.Open(cfg.VaultPath, key)
			if err != nil {
				return err
			}
			var in []byte
			if len(args) == 1 {
				in, err = os.ReadFile(args[0])
			} else {
				in, err = io.ReadAll(cmd.InOrStdin())
			}
			if err != nil {
				return err
			}
			fmt.Fprint(cmd.OutOrStdout(), v.Reveal(string(in)))
			return nil
		},
	}
}

//go:build !darwin

package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newMenubarCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "menubar",
		Short: "Manage the MDEMG menu bar app (macOS only)",
		Long:  "The menu bar app is only available on macOS.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("the menubar command is only available on macOS")
		},
	}
}

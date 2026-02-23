package cli

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
)

func newVersionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print MDEMG version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("mdemg %s\n", Version)
			fmt.Printf("  commit:    %s\n", Commit)
			fmt.Printf("  built:     %s\n", BuildDate)
			fmt.Printf("  go:        %s\n", runtime.Version())
			fmt.Printf("  os/arch:   %s/%s\n", runtime.GOOS, runtime.GOARCH)
		},
	}

	return cmd
}

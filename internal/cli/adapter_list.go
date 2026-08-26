// Package cli — `mdemg adapter list`
package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

func newAdapterListCmd() *cobra.Command {
	var (
		dir    string
		asJSON bool
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List saved checkpoints in an adapter directory",
		Long: `List all NNNN_adapters.safetensors checkpoints in an adapter directory.
Prints iter, file size, sha256[:12], and mtime. The frozen 'adapters.safetensors'
pointer is NOT listed as a candidate — use ` + "`mdemg adapter freeze`" + ` to pin one.

Example:
  mdemg adapter list --dir adapters/phase_e3_v1_base_v3
`,
		RunE: func(_ *cobra.Command, _ []string) error {
			abs, err := resolveAdapterDir(dir)
			if err != nil {
				return err
			}
			cps, err := enumerateCheckpoints(abs)
			if err != nil {
				return err
			}
			if asJSON {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(map[string]any{
					"dir":         abs,
					"checkpoints": cps,
				})
			}
			if len(cps) == 0 {
				fmt.Printf("no checkpoints in %s\n", abs)
				return nil
			}
			w := tabwriter.NewWriter(os.Stdout, 2, 8, 2, ' ', 0)
			fmt.Fprintln(w, "ITER\tSIZE\tSHA256[:12]\tMTIME_UTC\tPATH")
			for _, c := range cps {
				fmt.Fprintf(w, "%d\t%d\t%s\t%s\t%s\n",
					c.Iter, c.Size, c.SHA256[:12],
					c.MTime.Format("2006-01-02T15:04:05Z"), c.Path)
			}
			return w.Flush()
		},
	}
	cmd.Flags().StringVar(&dir, "dir", "", "adapter directory (required)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "emit JSON instead of table")
	_ = cmd.MarkFlagRequired("dir")
	return cmd
}

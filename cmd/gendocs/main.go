// Command gendocs generates man pages and markdown documentation from the
// MDEMG CLI command tree.
package main

import (
	"log/slog"
	"os"

	"github.com/spf13/cobra/doc"
	"mdemg/internal/cli"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))

	root := cli.NewRootCmdForDocs()

	// Generate man pages
	manDir := "./man/man1"
	if err := os.MkdirAll(manDir, 0o755); err != nil {
		slog.Error("failed to create man dir", "error", err)
		os.Exit(1)
	}

	header := &doc.GenManHeader{
		Title:   "MDEMG",
		Section: "1",
		Source:  "MDEMG " + cli.Version,
	}

	if err := doc.GenManTree(root, header, manDir); err != nil {
		slog.Error("failed to generate man pages", "error", err)
		os.Exit(1)
	}

	slog.Info("man pages generated", "dir", manDir)
}

// Command gendocs generates man pages and markdown documentation from the
// MDEMG CLI command tree.
package main

import (
	"log"
	"os"

	"github.com/spf13/cobra/doc"
	"mdemg/internal/cli"
)

func main() {
	root := cli.NewRootCmdForDocs()

	// Generate man pages
	manDir := "./man/man1"
	if err := os.MkdirAll(manDir, 0o755); err != nil {
		log.Fatalf("create man dir: %v", err)
	}

	header := &doc.GenManHeader{
		Title:   "MDEMG",
		Section: "1",
		Source:  "MDEMG " + cli.Version,
	}

	if err := doc.GenManTree(root, header, manDir); err != nil {
		log.Fatalf("generate man pages: %v", err)
	}

	log.Printf("Man pages generated in %s/", manDir)
}

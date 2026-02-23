package main

import "mdemg/internal/cli"

func main() {
	cli.RunLegacyShim("mdemg-mcp", "mdemg mcp", []string{"mcp"})
}

package main

import "mdemg/internal/cli"

func main() {
	cli.RunLegacyShim("mdemg-space-transfer", "mdemg space <subcommand>", []string{"space"})
}

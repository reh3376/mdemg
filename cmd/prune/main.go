package main

import "mdemg/internal/cli"

func main() {
	cli.RunLegacyShim("mdemg-prune", "mdemg prune", []string{"prune"})
}

package main

import "mdemg/internal/cli"

func main() {
	cli.RunLegacyShim("mdemg-plugin-scaffold", "mdemg plugin scaffold", []string{"plugin", "scaffold"})
}

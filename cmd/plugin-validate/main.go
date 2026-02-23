package main

import "mdemg/internal/cli"

func main() {
	cli.RunLegacyShim("mdemg-plugin-validate", "mdemg plugin validate", []string{"plugin", "validate"})
}

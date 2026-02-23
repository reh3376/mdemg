package main

import "mdemg/internal/cli"

func main() {
	cli.RunLegacyShim("mdemg-server", "mdemg serve", []string{"serve"})
}

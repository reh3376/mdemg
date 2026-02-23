package main

import "mdemg/internal/cli"

func main() {
	cli.RunLegacyShim("mdemg-reset-db", "mdemg db reset", []string{"db", "reset"})
}

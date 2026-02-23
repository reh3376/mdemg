package main

import "mdemg/internal/cli"

func main() {
	cli.RunLegacyShim("mdemg-watch", "mdemg watch", []string{"watch"})
}

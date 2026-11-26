package main

import (
	"fmt"
	"os"

	"tracelet.local/tracelet/internal/cli"
)

var version = "dev"
var commit = "unknown"

func main() {
	if err := cli.NewRoot(version, commit).Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "tracelet:", err)
		os.Exit(1)
	}
}

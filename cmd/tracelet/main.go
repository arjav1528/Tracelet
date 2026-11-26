package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/arjav1528/Tracelet/internal/cli"
)

var version = "dev"
var commit = "unknown"

func main() {
	if err := cli.NewRoot(version, commit).Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "tracelet:", err)
		var child *cli.ExitError
		if errors.As(err, &child) {
			os.Exit(child.Code)
		}
		os.Exit(1)
	}
}

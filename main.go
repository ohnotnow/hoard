package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/ohnotnow/hoard/internal/hoard"
)

func main() {
	if err := hoard.Run(os.Args[1:]); err != nil {
		var cliErr *hoard.CLIError
		if errors.As(err, &cliErr) {
			if cliErr.Message != "" {
				fmt.Fprintln(os.Stderr, cliErr.Message)
			}
			os.Exit(cliErr.ExitCode)
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

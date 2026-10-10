package main

import (
	"fmt"
	"os"

	"github.com/dev-nathaniel/Adroit-platform/internal/cli/cmd"
	"github.com/dev-nathaniel/Adroit-platform/internal/cli/ui"
)

func main() {
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, ui.Error(err.Error()))
		os.Exit(1)
	}
}

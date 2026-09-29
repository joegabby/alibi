package main

import (
	"fmt"
	"os"
	"github.com/joegabby/alibi/cli/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}

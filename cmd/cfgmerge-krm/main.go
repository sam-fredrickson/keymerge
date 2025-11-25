// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"
)

func main() {
	// Simple KRM function: read ResourceList from stdin, write to stdout
	if err := Run(os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "cfgmerge-krm:", err)
		os.Exit(1)
	}
}

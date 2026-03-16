package main

import (
	"fmt"
	"os"

	"github.com/mikoto2000/codew/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

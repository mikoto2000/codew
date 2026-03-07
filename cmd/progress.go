package cmd

import (
	"fmt"
	"os"
	"time"
)

func announceWorking(label string) func(success bool) {
	started := time.Now()
	fmt.Fprintf(os.Stderr, "%s...\n", label)
	return func(success bool) {
		status := "done"
		if !success {
			status = "failed"
		}
		fmt.Fprintf(os.Stderr, "%s (%s, %s)\n", label, status, time.Since(started).Round(100*time.Millisecond))
	}
}

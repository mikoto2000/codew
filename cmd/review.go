package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/mikoto2000/codew/internal/app"
)

var reviewCmd = &cobra.Command{
	Use:   "review",
	Short: "Review current git changes with severity ordering",
	RunE:  runReview,
}

func runReview(cmd *cobra.Command, _ []string) error {
	findings, missingTests, err := app.ReviewFindings(workspaceRoot)
	if err != nil {
		return err
	}
	if len(findings) == 0 {
		fmt.Println("No changed files.")
		return nil
	}

	for _, f := range findings {
		fmt.Printf("[%s] %s - %s\n", f.Severity, f.Path, f.Reason)
	}

	if len(missingTests) > 0 {
		fmt.Println("\n[Test Gap]")
		for _, p := range missingTests {
			fmt.Printf("- %s has no paired *_test.go change\n", p)
		}
	}

	return nil
}

package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var endpoint string

var rootCmd = &cobra.Command{
	Use:   "frictionax",
	Short: "CLI friction telemetry tool",
	Long:  "frictionax collects, reports, and analyzes CLI friction events.\nLearn more at https://github.com/sageox/frictionax",
}

func init() {
	rootCmd.PersistentFlags().StringVar(&endpoint, "endpoint", "http://localhost:8080", "frictionax server endpoint")
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

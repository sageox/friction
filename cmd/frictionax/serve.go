package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the frictionax server",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Fprintf(os.Stderr, "Use the 'frictionax-server' binary for the full server.\n")
		fmt.Fprintf(os.Stderr, "Install: go install github.com/sageox/frictionax/cmd/frictionax-server@latest\n")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(serveCmd)
}

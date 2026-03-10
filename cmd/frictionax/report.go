package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/sageox/frictionax"
	"github.com/spf13/cobra"
)

var reportCmd = &cobra.Command{
	Use:   "report",
	Short: "Send a friction event to the server",
	RunE:  runReport,
}

var (
	reportKind       string
	reportCommand    string
	reportSubcommand string
	reportActor      string
	reportAgentType  string
	reportInput      string
	reportErrorMsg   string
)

func init() {
	reportCmd.Flags().StringVar(&reportKind, "kind", "", "failure kind (unknown-command, unknown-flag, missing-required, invalid-arg, parse-error)")
	reportCmd.Flags().StringVar(&reportCommand, "command", "", "top-level command")
	reportCmd.Flags().StringVar(&reportSubcommand, "subcommand", "", "subcommand")
	reportCmd.Flags().StringVar(&reportActor, "actor", "human", "actor type (human, agent)")
	reportCmd.Flags().StringVar(&reportAgentType, "agent-type", "", "agent type (e.g., claude-code)")
	reportCmd.Flags().StringVar(&reportInput, "input", "", "the command input that caused friction")
	reportCmd.Flags().StringVar(&reportErrorMsg, "error-msg", "", "error message")

	_ = reportCmd.MarkFlagRequired("kind")
	_ = reportCmd.MarkFlagRequired("input")

	rootCmd.AddCommand(reportCmd)
}

func runReport(cmd *cobra.Command, args []string) error {
	event := frictionax.FrictionEvent{
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
		Kind:       frictionax.FailureKind(reportKind),
		Command:    reportCommand,
		Subcommand: reportSubcommand,
		Actor:      reportActor,
		AgentType:  reportAgentType,
		Input:      reportInput,
		ErrorMsg:   reportErrorMsg,
	}
	event.Truncate()

	req := frictionax.SubmitRequest{
		Events: []frictionax.FrictionEvent{event},
	}

	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	url := endpoint + "/api/v1/friction"
	resp, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "server returned %d: %s\n", resp.StatusCode, string(respBody))
		return fmt.Errorf("server error: %d", resp.StatusCode)
	}

	var result frictionax.FrictionResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return fmt.Errorf("parse response: %w", err)
	}

	fmt.Fprintf(os.Stdout, "accepted: %d event(s)\n", result.Accepted)
	return nil
}

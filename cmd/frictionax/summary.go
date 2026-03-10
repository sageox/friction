package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

var summaryCmd = &cobra.Command{
	Use:   "summary",
	Short: "Show aggregated friction data",
	RunE:  runSummary,
}

var (
	summaryKind  string
	summarySince string
	summaryLimit int
)

func init() {
	summaryCmd.Flags().StringVar(&summaryKind, "kind", "", "filter by failure kind")
	summaryCmd.Flags().StringVar(&summarySince, "since", "", "time window (e.g., 24h, 7d)")
	summaryCmd.Flags().IntVar(&summaryLimit, "limit", 10, "max top inputs to show")

	rootCmd.AddCommand(summaryCmd)
}

func runSummary(cmd *cobra.Command, args []string) error {
	u, err := url.Parse(endpoint + "/api/v1/friction/summary")
	if err != nil {
		return fmt.Errorf("parse endpoint: %w", err)
	}

	q := u.Query()
	if summaryKind != "" {
		q.Set("kind", summaryKind)
	}
	if summarySince != "" {
		q.Set("since", summarySince)
	}
	q.Set("limit", strconv.Itoa(summaryLimit))
	u.RawQuery = q.Encode()

	resp, err := http.Get(u.String())
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "server returned %d: %s\n", resp.StatusCode, string(body))
		return fmt.Errorf("server error: %d", resp.StatusCode)
	}

	var result struct {
		TotalEvents int            `json:"total_events"`
		ByKind      map[string]int `json:"by_kind"`
		ByActor     map[string]int `json:"by_actor"`
		TopInputs   []struct {
			Input string `json:"input"`
			Count int    `json:"count"`
			Kind  string `json:"kind"`
		} `json:"top_inputs"`
		Since string `json:"since"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("parse response: %w", err)
	}

	fmt.Fprintf(os.Stdout, "total events: %d\n\n", result.TotalEvents)

	if len(result.ByKind) > 0 {
		fmt.Fprintln(os.Stdout, "by kind:")
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		for k, c := range result.ByKind {
			fmt.Fprintf(w, "  %s\t%d\n", k, c)
		}
		w.Flush()
		fmt.Fprintln(os.Stdout)
	}

	if len(result.ByActor) > 0 {
		fmt.Fprintln(os.Stdout, "by actor:")
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		for a, c := range result.ByActor {
			fmt.Fprintf(w, "  %s\t%d\n", a, c)
		}
		w.Flush()
		fmt.Fprintln(os.Stdout)
	}

	if len(result.TopInputs) > 0 {
		fmt.Fprintln(os.Stdout, "top inputs:")
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintf(w, "  INPUT\tKIND\tCOUNT\n")
		fmt.Fprintf(w, "  %s\t%s\t%s\n", strings.Repeat("-", 30), strings.Repeat("-", 16), strings.Repeat("-", 5))
		for _, ti := range result.TopInputs {
			fmt.Fprintf(w, "  %s\t%s\t%d\n", ti.Input, ti.Kind, ti.Count)
		}
		w.Flush()
	}

	return nil
}

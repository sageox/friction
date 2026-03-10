package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/sageox/frictionax"
	"github.com/spf13/cobra"
)

var (
	buildServer       string
	buildPatternsFile string
	buildCatalog      string
	buildOutput       string
	buildMinHuman     int
	buildMinAgent     int
	buildMinTotal     int
	buildDiff         bool
	buildFormat       string
)

var catalogBuildCmd = &cobra.Command{
	Use:   "build",
	Short: "Build catalog entries from friction pattern data",
	Long: `Fetches friction patterns from a server or file, filters by thresholds,
deduplicates against an existing catalog, and outputs new candidate entries.

Exactly one of --server or --patterns-file must be specified.

The output contains CommandMapping entries with empty Target fields.
Use an LLM skill or manual review to fill in targets and confidence scores.`,
	RunE: runCatalogBuild,
}

func init() {
	catalogBuildCmd.Flags().StringVar(&buildServer, "server", "", "frictionax-server URL")
	catalogBuildCmd.Flags().StringVar(&buildPatternsFile, "patterns-file", "", "JSON file with patterns (- for stdin)")
	catalogBuildCmd.Flags().StringVar(&buildCatalog, "catalog", "default_catalog.json", "existing catalog to merge against")
	catalogBuildCmd.Flags().StringVar(&buildOutput, "output", "", "output file (default: stdout)")
	catalogBuildCmd.Flags().IntVar(&buildMinHuman, "min-human-count", 2, "minimum human event count")
	catalogBuildCmd.Flags().IntVar(&buildMinAgent, "min-agent-count", 3, "minimum agent event count")
	catalogBuildCmd.Flags().IntVar(&buildMinTotal, "min-total-count", 2, "minimum total event count")
	catalogBuildCmd.Flags().BoolVar(&buildDiff, "diff", false, "output only new entries")
	catalogBuildCmd.Flags().StringVar(&buildFormat, "format", "json", "output format: json or table")

	catalogCmd.AddCommand(catalogBuildCmd)
}

func runCatalogBuild(cmd *cobra.Command, args []string) error {
	hasServer := buildServer != ""
	hasFile := buildPatternsFile != ""
	if hasServer == hasFile {
		return fmt.Errorf("exactly one of --server or --patterns-file must be specified")
	}

	var source frictionax.PatternSource
	if hasServer {
		source = frictionax.NewHTTPSource(buildServer)
	} else {
		source = frictionax.NewFileSource(buildPatternsFile)
	}

	ctx := context.Background()
	patterns, err := source.FetchPatterns(ctx, buildMinTotal, 500)
	if err != nil {
		return fmt.Errorf("fetch patterns: %w", err)
	}

	existing := frictionax.CatalogData{}
	if _, statErr := os.Stat(buildCatalog); statErr == nil {
		data, err := os.ReadFile(buildCatalog)
		if err != nil {
			return fmt.Errorf("read catalog: %w", err)
		}
		if err := json.Unmarshal(data, &existing); err != nil {
			return fmt.Errorf("parse catalog: %w", err)
		}
	}

	cfg := frictionax.BuildConfig{
		MinHumanCount: buildMinHuman,
		MinAgentCount: buildMinAgent,
		MinTotalCount: buildMinTotal,
		SkipKinds:     []string{string(frictionax.FailureUnknownFlag)},
		DiffOnly:      buildDiff,
	}

	result, err := frictionax.Build(patterns, existing, cfg)
	if err != nil {
		return fmt.Errorf("build catalog: %w", err)
	}

	var output []byte
	switch buildFormat {
	case "table":
		output = formatBuildTable(result)
	default:
		output, err = json.MarshalIndent(result, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal result: %w", err)
		}
	}

	if buildOutput != "" {
		if err := os.WriteFile(buildOutput, output, 0644); err != nil {
			return fmt.Errorf("write output: %w", err)
		}
		fmt.Fprintf(os.Stderr, "wrote %d new entries to %s\n", len(result.NewEntries), buildOutput)
	} else {
		fmt.Fprint(os.Stdout, string(output))
	}

	return nil
}

func formatBuildTable(result *frictionax.BuildResult) []byte {
	var b strings.Builder

	if len(result.NewEntries) > 0 {
		b.WriteString("NEW ENTRIES (need target + confidence):\n")
		w := tabwriter.NewWriter(&b, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "PATTERN\tCOUNT")
		for _, e := range result.NewEntries {
			fmt.Fprintf(w, "%s\t%d\n", e.Pattern, e.Count)
		}
		w.Flush()
	}

	if len(result.Skipped) > 0 {
		fmt.Fprintf(&b, "\nSKIPPED: %d patterns\n", len(result.Skipped))
		for _, s := range result.Skipped {
			fmt.Fprintf(&b, "  %s (%s): %s\n", s.Pattern, s.Kind, s.Reason)
		}
	}

	if len(result.NewEntries) == 0 {
		b.WriteString("No new patterns found matching thresholds.\n")
	}

	return []byte(b.String())
}

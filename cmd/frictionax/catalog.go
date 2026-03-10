package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/sageox/frictionax"
	"github.com/spf13/cobra"
)

var catalogCmd = &cobra.Command{
	Use:   "catalog",
	Short: "Manage the friction catalog",
}

var catalogGetCmd = &cobra.Command{
	Use:   "get",
	Short: "Get the current catalog from the server",
	RunE:  runCatalogGet,
}

var catalogSetCmd = &cobra.Command{
	Use:   "set",
	Short: "Upload a catalog to the server",
	RunE:  runCatalogSet,
}

var catalogFile string

func init() {
	catalogSetCmd.Flags().StringVar(&catalogFile, "file", "", "path to catalog JSON file")
	_ = catalogSetCmd.MarkFlagRequired("file")

	catalogCmd.AddCommand(catalogGetCmd)
	catalogCmd.AddCommand(catalogSetCmd)
	rootCmd.AddCommand(catalogCmd)
}

func runCatalogGet(cmd *cobra.Command, args []string) error {
	url := endpoint + "/api/v1/friction/catalog"
	resp, err := http.Get(url)
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

	// pretty-print the JSON
	var pretty map[string]interface{}
	if err := json.Unmarshal(body, &pretty); err != nil {
		// fallback to raw output
		fmt.Fprintln(os.Stdout, string(body))
		return nil
	}

	out, err := json.MarshalIndent(pretty, "", "  ")
	if err != nil {
		fmt.Fprintln(os.Stdout, string(body))
		return nil
	}
	fmt.Fprintln(os.Stdout, string(out))
	return nil
}

func runCatalogSet(cmd *cobra.Command, args []string) error {
	data, err := os.ReadFile(catalogFile)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}

	// validate it parses as CatalogData
	var catalog frictionax.CatalogData
	if err := json.Unmarshal(data, &catalog); err != nil {
		return fmt.Errorf("invalid catalog JSON: %w", err)
	}

	if catalog.Version == "" {
		return fmt.Errorf("catalog must have a version field")
	}

	url := endpoint + "/api/v1/friction/catalog"
	req, err := http.NewRequest(http.MethodPut, url, strings.NewReader(string(data)))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
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

	fmt.Fprintf(os.Stdout, "catalog uploaded: version=%s\n", catalog.Version)
	return nil
}

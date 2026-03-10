package frictionax

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
)

// PatternSource fetches aggregated friction patterns from a data source.
type PatternSource interface {
	FetchPatterns(ctx context.Context, minCount int, limit int) ([]PatternDetail, error)
}

// HTTPSource fetches patterns from a frictionax-server HTTP endpoint.
type HTTPSource struct {
	baseURL string
	client  *http.Client
}

// NewHTTPSource creates a PatternSource that fetches from the given frictionax-server URL.
// The server must expose GET /api/v1/friction/patterns.
func NewHTTPSource(baseURL string) *HTTPSource {
	return &HTTPSource{
		baseURL: baseURL,
		client:  &http.Client{},
	}
}

// FetchPatterns fetches patterns from the HTTP endpoint.
func (s *HTTPSource) FetchPatterns(ctx context.Context, minCount int, limit int) ([]PatternDetail, error) {
	url := fmt.Sprintf("%s/api/v1/friction/patterns?min_count=%d&limit=%d", s.baseURL, minCount, limit)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch patterns: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("server returned %d: %s", resp.StatusCode, string(body))
	}

	var result PatternsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return result.Patterns, nil
}

// FileSource reads patterns from a JSON file or stdin.
type FileSource struct {
	path string
}

// NewFileSource creates a PatternSource that reads from a JSON file.
// Use "-" to read from stdin.
func NewFileSource(path string) *FileSource {
	return &FileSource{path: path}
}

// FetchPatterns reads patterns from the file, applying minCount and limit filters.
func (s *FileSource) FetchPatterns(ctx context.Context, minCount int, limit int) ([]PatternDetail, error) {
	var reader io.Reader
	if s.path == "-" {
		reader = os.Stdin
	} else {
		f, err := os.Open(s.path)
		if err != nil {
			return nil, fmt.Errorf("open file: %w", err)
		}
		defer f.Close()
		reader = f
	}

	var result PatternsResponse
	if err := json.NewDecoder(reader).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode patterns: %w", err)
	}

	var filtered []PatternDetail
	for _, p := range result.Patterns {
		if p.TotalCount >= int64(minCount) {
			filtered = append(filtered, p)
		}
	}

	if limit > 0 && len(filtered) > limit {
		filtered = filtered[:limit]
	}

	return filtered, nil
}

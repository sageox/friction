package frictionax

import "testing"

func TestBuild_ThresholdFiltering(t *testing.T) {
	patterns := []PatternDetail{
		{Pattern: "statu", Kind: "unknown-command", TotalCount: 5, HumanCount: 3, AgentCount: 2},
		{Pattern: "logn", Kind: "unknown-command", TotalCount: 1, HumanCount: 1, AgentCount: 0},
		{Pattern: "vesion", Kind: "unknown-command", TotalCount: 4, HumanCount: 0, AgentCount: 4},
	}
	existing := CatalogData{Version: "1.0"}
	cfg := DefaultBuildConfig()

	result, err := Build(patterns, existing, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.NewEntries) != 2 {
		t.Errorf("expected 2 new entries, got %d", len(result.NewEntries))
	}

	// "logn" should be skipped (below threshold)
	foundLogn := false
	for _, s := range result.Skipped {
		if s.Pattern == "logn" && s.Reason == "below-threshold" {
			foundLogn = true
		}
	}
	if !foundLogn {
		t.Error("expected 'logn' to be skipped with reason below-threshold")
	}
}

func TestBuild_Deduplication(t *testing.T) {
	patterns := []PatternDetail{
		{Pattern: "statu", Kind: "unknown-command", TotalCount: 5, HumanCount: 3, AgentCount: 2},
	}
	existing := CatalogData{
		Version: "1.0",
		Commands: []CommandMapping{
			{Pattern: "statu", Target: "status", Confidence: 0.95},
		},
	}
	cfg := DefaultBuildConfig()

	result, err := Build(patterns, existing, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.NewEntries) != 0 {
		t.Errorf("expected 0 new entries (dedup), got %d", len(result.NewEntries))
	}

	foundDedup := false
	for _, s := range result.Skipped {
		if s.Pattern == "statu" && s.Reason == "already-in-catalog" {
			foundDedup = true
		}
	}
	if !foundDedup {
		t.Error("expected 'statu' to be skipped with reason already-in-catalog")
	}
}

func TestBuild_SkipKinds(t *testing.T) {
	patterns := []PatternDetail{
		{Pattern: "--verbose", Kind: "unknown-flag", TotalCount: 10, HumanCount: 5, AgentCount: 5},
	}
	existing := CatalogData{Version: "1.0"}
	cfg := DefaultBuildConfig()

	result, err := Build(patterns, existing, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.NewEntries) != 0 {
		t.Errorf("expected 0 new entries (skip unknown-flag), got %d", len(result.NewEntries))
	}
}

func TestBuild_DiffOnly(t *testing.T) {
	patterns := []PatternDetail{
		{Pattern: "statu", Kind: "unknown-command", TotalCount: 5, HumanCount: 3, AgentCount: 2},
	}
	existing := CatalogData{
		Version: "1.0",
		Commands: []CommandMapping{
			{Pattern: "helpp", Target: "help", Confidence: 0.95},
		},
	}
	cfg := DefaultBuildConfig()
	cfg.DiffOnly = true

	result, err := Build(patterns, existing, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Catalog.Commands) != 1 {
		t.Errorf("expected 1 command in diff catalog, got %d", len(result.Catalog.Commands))
	}
	if result.Catalog.Commands[0].Pattern != "statu" {
		t.Errorf("expected diff to contain 'statu', got '%s'", result.Catalog.Commands[0].Pattern)
	}
}

func TestBuild_EmptyPatterns(t *testing.T) {
	result, err := Build(nil, CatalogData{}, DefaultBuildConfig())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.NewEntries) != 0 {
		t.Errorf("expected 0 new entries, got %d", len(result.NewEntries))
	}
	if len(result.Skipped) != 0 {
		t.Errorf("expected 0 skipped, got %d", len(result.Skipped))
	}
}

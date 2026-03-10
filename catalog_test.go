package frictionax

import (
	"sync"
	"testing"
)

func TestNewFrictionCatalog(t *testing.T) {
	cat := newFrictionCatalog("testcli")

	if cat == nil {
		t.Fatal("newFrictionCatalog returned nil")
	}

	if cat.version != "" {
		t.Errorf("expected empty version, got %q", cat.version)
	}

	if cat.commands == nil {
		t.Error("commands map is nil")
	}

	if cat.tokens == nil {
		t.Error("tokens map is nil")
	}

	if len(cat.commands) != 0 {
		t.Errorf("expected empty commands map, got %d entries", len(cat.commands))
	}

	if len(cat.tokens) != 0 {
		t.Errorf("expected empty tokens map, got %d entries", len(cat.tokens))
	}
}

func TestCatalogUpdate(t *testing.T) {
	tests := []struct {
		name           string
		data           CatalogData
		wantVersion    string
		wantCmdCount   int
		wantTokenCount int
	}{
		{
			name:           "empty data",
			data:           CatalogData{},
			wantVersion:    "",
			wantCmdCount:   0,
			wantTokenCount: 0,
		},
		{
			name: "version only",
			data: CatalogData{
				Version: "1.0.0",
			},
			wantVersion:    "1.0.0",
			wantCmdCount:   0,
			wantTokenCount: 0,
		},
		{
			name: "commands only",
			data: CatalogData{
				Version: "1.1.0",
				Commands: []CommandMapping{
					{Pattern: "daemons list --every", Target: "daemons show --all", Count: 5, Confidence: 0.9},
					{Pattern: "agent ls", Target: "agent list", Count: 3, Confidence: 0.8},
				},
			},
			wantVersion:    "1.1.0",
			wantCmdCount:   2,
			wantTokenCount: 0,
		},
		{
			name: "tokens only",
			data: CatalogData{
				Version: "1.2.0",
				Tokens: []TokenMapping{
					{Pattern: "depliy", Target: "deploy", Kind: FailureUnknownCommand, Count: 10, Confidence: 0.95},
					{Pattern: "satuts", Target: "status", Kind: FailureUnknownCommand, Count: 7, Confidence: 0.85},
				},
			},
			wantVersion:    "1.2.0",
			wantCmdCount:   0,
			wantTokenCount: 2,
		},
		{
			name: "commands and tokens",
			data: CatalogData{
				Version: "2.0.0",
				Commands: []CommandMapping{
					{Pattern: "testcli init --force", Target: "init --yes", Count: 15, Confidence: 0.92},
				},
				Tokens: []TokenMapping{
					{Pattern: "inti", Target: "init", Kind: FailureUnknownCommand, Count: 12, Confidence: 0.88},
					{Pattern: "--verbos", Target: "--verbose", Kind: FailureUnknownFlag, Count: 8, Confidence: 0.9},
				},
			},
			wantVersion:    "2.0.0",
			wantCmdCount:   1,
			wantTokenCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cat := newFrictionCatalog("testcli")

			err := cat.Update(tt.data)
			if err != nil {
				t.Fatalf("Update returned error: %v", err)
			}

			if cat.Version() != tt.wantVersion {
				t.Errorf("version = %q, want %q", cat.Version(), tt.wantVersion)
			}

			if len(cat.commands) != tt.wantCmdCount {
				t.Errorf("commands count = %d, want %d", len(cat.commands), tt.wantCmdCount)
			}

			if len(cat.tokens) != tt.wantTokenCount {
				t.Errorf("tokens count = %d, want %d", len(cat.tokens), tt.wantTokenCount)
			}
		})
	}
}

func TestCatalogUpdateOverwrites(t *testing.T) {
	cat := newFrictionCatalog("testcli")

	err := cat.Update(CatalogData{
		Version: "1.0.0",
		Commands: []CommandMapping{
			{Pattern: "old cmd", Target: "target1", Count: 1, Confidence: 0.5},
		},
		Tokens: []TokenMapping{
			{Pattern: "oldtypo", Target: "correct", Kind: FailureUnknownCommand, Count: 1, Confidence: 0.5},
		},
	})
	if err != nil {
		t.Fatalf("first Update failed: %v", err)
	}

	err = cat.Update(CatalogData{
		Version: "2.0.0",
		Commands: []CommandMapping{
			{Pattern: "new cmd", Target: "target2", Count: 5, Confidence: 0.9},
		},
	})
	if err != nil {
		t.Fatalf("second Update failed: %v", err)
	}

	if cat.Version() != "2.0.0" {
		t.Errorf("version = %q, want %q", cat.Version(), "2.0.0")
	}

	if cat.LookupCommand("old cmd") != nil {
		t.Error("old command should not exist after update")
	}

	if cat.LookupCommand("new cmd") == nil {
		t.Error("new command should exist after update")
	}

	if cat.LookupToken("oldtypo", FailureUnknownCommand) != nil {
		t.Error("old token should not exist after update")
	}
}

func TestLookupCommand(t *testing.T) {
	cat := newFrictionCatalog("testcli")
	err := cat.Update(CatalogData{
		Version: "1.0.0",
		Commands: []CommandMapping{
			{Pattern: "daemons list --every", Target: "daemons show --all", Count: 5, Confidence: 0.9, Description: "common mistake"},
			{Pattern: "agent ls -v", Target: "agent list --verbose", Count: 3, Confidence: 0.8},
			{Pattern: "status", Target: "doctor", Count: 2, Confidence: 0.7},
		},
	})
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	tests := []struct {
		name       string
		input      string
		wantNil    bool
		wantTarget string
	}{
		{name: "exact match", input: "daemons list --every", wantTarget: "daemons show --all"},
		{name: "with cli prefix", input: "testcli daemons list --every", wantTarget: "daemons show --all"},
		{name: "flags reordered", input: "agent -v ls", wantTarget: "agent list --verbose"},
		{name: "simple command", input: "status", wantTarget: "doctor"},
		{name: "simple command with cli prefix", input: "testcli status", wantTarget: "doctor"},
		{name: "no match", input: "nonexistent command", wantNil: true},
		{name: "empty input", input: "", wantNil: true},
		{name: "only cli name", input: "testcli", wantNil: true},
		{name: "partial match should fail", input: "daemons list", wantNil: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cat.LookupCommand(tt.input)

			if tt.wantNil {
				if result != nil {
					t.Errorf("expected nil, got %+v", result)
				}
				return
			}

			if result == nil {
				t.Fatal("expected non-nil result")
			}

			if result.Target != tt.wantTarget {
				t.Errorf("target = %q, want %q", result.Target, tt.wantTarget)
			}
		})
	}
}

func TestLookupToken(t *testing.T) {
	cat := newFrictionCatalog("testcli")
	err := cat.Update(CatalogData{
		Version: "1.0.0",
		Tokens: []TokenMapping{
			{Pattern: "depliy", Target: "deploy", Kind: FailureUnknownCommand, Count: 10, Confidence: 0.95},
			{Pattern: "satuts", Target: "status", Kind: FailureUnknownCommand, Count: 7, Confidence: 0.85},
			{Pattern: "--verbos", Target: "--verbose", Kind: FailureUnknownFlag, Count: 5, Confidence: 0.8},
			{Pattern: "--hlep", Target: "--help", Kind: FailureUnknownFlag, Count: 3, Confidence: 0.75},
		},
	})
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	tests := []struct {
		name       string
		token      string
		kind       FailureKind
		wantNil    bool
		wantTarget string
	}{
		{name: "command typo match", token: "depliy", kind: FailureUnknownCommand, wantTarget: "deploy"},
		{name: "another command typo", token: "satuts", kind: FailureUnknownCommand, wantTarget: "status"},
		{name: "flag typo match", token: "--verbos", kind: FailureUnknownFlag, wantTarget: "--verbose"},
		{name: "case insensitive lookup", token: "DEPLIY", kind: FailureUnknownCommand, wantTarget: "deploy"},
		{name: "mixed case lookup", token: "DePLiy", kind: FailureUnknownCommand, wantTarget: "deploy"},
		{name: "wrong kind returns nil", token: "depliy", kind: FailureUnknownFlag, wantNil: true},
		{name: "nonexistent token", token: "foobar", kind: FailureUnknownCommand, wantNil: true},
		{name: "empty token", token: "", kind: FailureUnknownCommand, wantNil: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cat.LookupToken(tt.token, tt.kind)

			if tt.wantNil {
				if result != nil {
					t.Errorf("expected nil, got %+v", result)
				}
				return
			}

			if result == nil {
				t.Fatal("expected non-nil result")
			}

			if result.Target != tt.wantTarget {
				t.Errorf("target = %q, want %q", result.Target, tt.wantTarget)
			}
		})
	}
}

func TestVersion(t *testing.T) {
	cat := newFrictionCatalog("testcli")

	if v := cat.Version(); v != "" {
		t.Errorf("initial version = %q, want empty", v)
	}

	_ = cat.Update(CatalogData{Version: "3.14.159"})
	if v := cat.Version(); v != "3.14.159" {
		t.Errorf("version = %q, want %q", v, "3.14.159")
	}

	_ = cat.Update(CatalogData{Version: ""})
	if v := cat.Version(); v != "" {
		t.Errorf("version = %q, want empty", v)
	}
}

func TestTokenKey(t *testing.T) {
	tests := []struct {
		name  string
		token string
		kind  FailureKind
		want  string
	}{
		{name: "lowercase token", token: "deploy", kind: FailureUnknownCommand, want: "deploy:unknown-command"},
		{name: "uppercase gets lowercased", token: "DEPLOY", kind: FailureUnknownCommand, want: "deploy:unknown-command"},
		{name: "mixed case token", token: "DePlOy", kind: FailureUnknownFlag, want: "deploy:unknown-flag"},
		{name: "empty token", token: "", kind: FailureInvalidArg, want: ":invalid-arg"},
		{name: "flag with dashes", token: "--verbose", kind: FailureUnknownFlag, want: "--verbose:unknown-flag"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tokenKey(tt.token, tt.kind)
			if got != tt.want {
				t.Errorf("tokenKey(%q, %q) = %q, want %q", tt.token, tt.kind, got, tt.want)
			}
		})
	}
}

func TestConcurrentAccess(t *testing.T) {
	cat := newFrictionCatalog("testcli")

	_ = cat.Update(CatalogData{
		Version: "1.0.0",
		Commands: []CommandMapping{
			{Pattern: "test cmd", Target: "target", Count: 1, Confidence: 0.9},
		},
		Tokens: []TokenMapping{
			{Pattern: "typo", Target: "correct", Kind: FailureUnknownCommand, Count: 1, Confidence: 0.9},
		},
	})

	var wg sync.WaitGroup
	done := make(chan struct{})

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-done:
					return
				default:
					_ = cat.LookupCommand("test cmd")
					_ = cat.LookupToken("typo", FailureUnknownCommand)
					_ = cat.Version()
				}
			}
		}()
	}

	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(version int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				select {
				case <-done:
					return
				default:
					_ = cat.Update(CatalogData{
						Version: "1.0." + string(rune('0'+version)),
						Commands: []CommandMapping{
							{Pattern: "test cmd", Target: "target", Count: j, Confidence: 0.9},
						},
					})
				}
			}
		}(i)
	}

	go func() {
		wg.Wait()
	}()

	for i := 0; i < 300; i++ {
		_ = cat.Version()
	}
	close(done)

	wg.Wait()
}

func TestCatalogImplementsInterface(t *testing.T) {
	var _ catalog = (*frictionCatalog)(nil)
}

func TestRegexPatternMatching(t *testing.T) {
	tests := []struct {
		name       string
		pattern    string
		target     string
		input      string
		wantNil    bool
		wantTarget string
	}{
		{
			name:       "simple regex match with capture",
			pattern:    `agent close ([a-zA-Z0-9-]+)`,
			target:     "agent $1 session stop",
			input:      "agent close Oxa7b3",
			wantTarget: "agent Oxa7b3 session stop",
		},
		{
			name:       "multiple capture groups",
			pattern:    `agent ([a-zA-Z0-9-]+) move ([a-zA-Z0-9-]+)`,
			target:     "agent $1 session move --dest=$2",
			input:      "agent Oxa7b3 move archive",
			wantTarget: "agent Oxa7b3 session move --dest=archive",
		},
		{
			name:    "no match",
			pattern: `agent close ([a-zA-Z0-9-]+)`,
			target:  "agent $1 session stop",
			input:   "agent list",
			wantNil: true,
		},
		{
			name:       "regex without capture groups",
			pattern:    `daemons (list|show) --every`,
			target:     "daemons show --all",
			input:      "daemons list --every",
			wantTarget: "daemons show --all",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cat := newFrictionCatalog("testcli")
			err := cat.Update(CatalogData{
				Version: "1.0.0",
				Commands: []CommandMapping{
					{
						Pattern:     tt.pattern,
						Target:      tt.target,
						HasRegex:    true,
						AutoExecute: true,
						Confidence:  0.95,
					},
				},
			})
			if err != nil {
				t.Fatalf("Update failed: %v", err)
			}

			result := cat.LookupCommand(tt.input)

			if tt.wantNil {
				if result != nil {
					t.Errorf("expected nil, got %+v", result)
				}
				return
			}

			if result == nil {
				t.Fatal("expected non-nil result")
			}

			corrected, ok := result.ApplyMapping(tt.input)
			if !ok {
				t.Fatal("ApplyMapping returned false")
			}

			if corrected != tt.wantTarget {
				t.Errorf("corrected = %q, want %q", corrected, tt.wantTarget)
			}
		})
	}
}

func TestApplyMapping(t *testing.T) {
	tests := []struct {
		name     string
		mapping  CommandMapping
		input    string
		wantCorr string
		wantOk   bool
	}{
		{
			name:     "literal pattern returns target as-is",
			mapping:  CommandMapping{Pattern: "agent prine", Target: "agent prime", HasRegex: false},
			input:    "agent prine",
			wantCorr: "agent prime",
			wantOk:   true,
		},
		{
			name:     "regex with single capture",
			mapping:  CommandMapping{Pattern: `agent close ([a-zA-Z0-9-]+)`, Target: "agent $1 session stop", HasRegex: true},
			input:    "agent close Oxa7b3",
			wantCorr: "agent Oxa7b3 session stop",
			wantOk:   true,
		},
		{
			name:     "regex no match returns false",
			mapping:  CommandMapping{Pattern: `agent close ([a-zA-Z0-9-]+)`, Target: "agent $1 session stop", HasRegex: true},
			input:    "agent list",
			wantCorr: "",
			wantOk:   false,
		},
		{
			name:     "invalid regex returns false",
			mapping:  CommandMapping{Pattern: `agent close ([`, Target: "agent $1 session stop", HasRegex: true},
			input:    "agent close test",
			wantCorr: "",
			wantOk:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			corrected, ok := tt.mapping.ApplyMapping(tt.input)

			if ok != tt.wantOk {
				t.Errorf("ok = %v, want %v", ok, tt.wantOk)
			}
			if corrected != tt.wantCorr {
				t.Errorf("corrected = %q, want %q", corrected, tt.wantCorr)
			}
		})
	}
}

package frictionax

import (
	"testing"
)

// mockCatalog implements catalog for testing.
type mockCatalog struct {
	cliName         string
	commandMappings map[string]*CommandMapping
	tokenMappings   map[string]*TokenMapping
	version         string
}

func newMockCatalog(cliName string) *mockCatalog {
	return &mockCatalog{
		cliName:         cliName,
		commandMappings: make(map[string]*CommandMapping),
		tokenMappings:   make(map[string]*TokenMapping),
		version:         "test-v1",
	}
}

func (m *mockCatalog) LookupCommand(input string) *CommandMapping {
	fc := &frictionCatalog{cliName: m.cliName}
	return m.commandMappings[fc.normalizeCommand(input)]
}

func (m *mockCatalog) LookupToken(token string, kind FailureKind) *TokenMapping {
	key := tokenKey(token, kind)
	return m.tokenMappings[key]
}

func (m *mockCatalog) Update(data CatalogData) error {
	return nil
}

func (m *mockCatalog) Version() string {
	return m.version
}

func (m *mockCatalog) addCommand(pattern, target string, confidence float64, desc string) {
	mapping := &CommandMapping{
		Pattern:     pattern,
		Target:      target,
		Confidence:  confidence,
		Description: desc,
	}
	fc := &frictionCatalog{cliName: m.cliName}
	m.commandMappings[fc.normalizeCommand(pattern)] = mapping
}

func (m *mockCatalog) addToken(pattern, target string, kind FailureKind, confidence float64) {
	mapping := &TokenMapping{
		Pattern:    pattern,
		Target:     target,
		Kind:       kind,
		Confidence: confidence,
	}
	key := tokenKey(pattern, kind)
	m.tokenMappings[key] = mapping
}

func TestNewSuggestionEngine(t *testing.T) {
	tests := []struct {
		name    string
		catalog catalog
	}{
		{name: "creates engine with catalog", catalog: newMockCatalog("testcli")},
		{name: "creates engine with nil catalog", catalog: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			engine := newSuggestionEngine(tt.catalog)
			if engine == nil {
				t.Fatal("engine should not be nil")
			}
			if engine.levenshtein == nil {
				t.Fatal("levenshtein should not be nil")
			}
		})
	}
}

func TestSuggestForCommand_CatalogCommandRemap(t *testing.T) {
	tests := []struct {
		name     string
		fullCmd  string
		ctx      suggestContext
		setupCat func(*mockCatalog)
		wantType SuggestionType
		wantOrig string
		wantCorr string
		wantConf float64
		wantDesc string
		wantNil  bool
	}{
		{
			name:    "returns catalog command remap when found",
			fullCmd: "testcli agent lsit",
			ctx:     suggestContext{},
			setupCat: func(c *mockCatalog) {
				c.addCommand("agent lsit", "agent list", 0.95, "typo correction")
			},
			wantType: SuggestionCommandRemap,
			wantOrig: "testcli agent lsit",
			wantCorr: "agent list",
			wantConf: 0.95,
			wantDesc: "typo correction",
		},
		{
			name:    "returns catalog command remap with flags",
			fullCmd: "testcli daemons show --every",
			ctx:     suggestContext{},
			setupCat: func(c *mockCatalog) {
				c.addCommand("daemons show --every", "daemons show --all", 0.9, "flag remap")
			},
			wantType: SuggestionCommandRemap,
			wantOrig: "testcli daemons show --every",
			wantCorr: "daemons show --all",
			wantConf: 0.9,
			wantDesc: "flag remap",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cat := newMockCatalog("testcli")
			tt.setupCat(cat)
			engine := newSuggestionEngine(cat)

			suggestion := engine.suggestForCommand(tt.fullCmd, tt.ctx)

			if tt.wantNil {
				if suggestion != nil {
					t.Fatalf("expected nil, got %+v", suggestion)
				}
				return
			}

			if suggestion == nil {
				t.Fatal("expected non-nil suggestion")
			}
			if suggestion.Type != tt.wantType {
				t.Errorf("type = %q, want %q", suggestion.Type, tt.wantType)
			}
			if suggestion.Original != tt.wantOrig {
				t.Errorf("original = %q, want %q", suggestion.Original, tt.wantOrig)
			}
			if suggestion.Corrected != tt.wantCorr {
				t.Errorf("corrected = %q, want %q", suggestion.Corrected, tt.wantCorr)
			}
			if suggestion.Confidence != tt.wantConf {
				t.Errorf("confidence = %f, want %f", suggestion.Confidence, tt.wantConf)
			}
			if suggestion.Description != tt.wantDesc {
				t.Errorf("description = %q, want %q", suggestion.Description, tt.wantDesc)
			}
		})
	}
}

func TestSuggestForCommand_TokenLookup(t *testing.T) {
	tests := []struct {
		name     string
		fullCmd  string
		ctx      suggestContext
		setupCat func(*mockCatalog)
		wantType SuggestionType
		wantOrig string
		wantCorr string
		wantConf float64
	}{
		{
			name:    "falls back to token lookup when command not found",
			fullCmd: "testcli agent stattus",
			ctx: suggestContext{
				Kind:     FailureUnknownCommand,
				BadToken: "stattus",
			},
			setupCat: func(c *mockCatalog) {
				c.addToken("stattus", "status", FailureUnknownCommand, 0.88)
			},
			wantType: SuggestionTokenFix,
			wantOrig: "stattus",
			wantCorr: "status",
			wantConf: 0.88,
		},
		{
			name:    "token lookup respects failure kind",
			fullCmd: "testcli agent list --verboes",
			ctx: suggestContext{
				Kind:     FailureUnknownFlag,
				BadToken: "verboes",
			},
			setupCat: func(c *mockCatalog) {
				c.addToken("verboes", "verbose", FailureUnknownFlag, 0.92)
			},
			wantType: SuggestionTokenFix,
			wantOrig: "verboes",
			wantCorr: "verbose",
			wantConf: 0.92,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cat := newMockCatalog("testcli")
			tt.setupCat(cat)
			engine := newSuggestionEngine(cat)

			suggestion := engine.suggestForCommand(tt.fullCmd, tt.ctx)

			if suggestion == nil {
				t.Fatal("expected non-nil suggestion")
			}
			if suggestion.Type != tt.wantType {
				t.Errorf("type = %q, want %q", suggestion.Type, tt.wantType)
			}
			if suggestion.Original != tt.wantOrig {
				t.Errorf("original = %q, want %q", suggestion.Original, tt.wantOrig)
			}
			if suggestion.Corrected != tt.wantCorr {
				t.Errorf("corrected = %q, want %q", suggestion.Corrected, tt.wantCorr)
			}
			if suggestion.Confidence != tt.wantConf {
				t.Errorf("confidence = %f, want %f", suggestion.Confidence, tt.wantConf)
			}
		})
	}
}

func TestSuggestForCommand_LevenshteinFallback(t *testing.T) {
	tests := []struct {
		name     string
		fullCmd  string
		ctx      suggestContext
		wantOrig string
		wantCorr string
		wantNil  bool
	}{
		{
			name:    "falls back to levenshtein when catalog has no match",
			fullCmd: "testcli agent statis",
			ctx: suggestContext{
				Kind:         FailureUnknownCommand,
				BadToken:     "statis",
				ValidOptions: []string{"status", "start", "stop", "list"},
			},
			wantOrig: "statis",
			wantCorr: "status",
		},
		{
			name:    "levenshtein finds closest match",
			fullCmd: "testcli agent lst",
			ctx: suggestContext{
				Kind:         FailureUnknownCommand,
				BadToken:     "lst",
				ValidOptions: []string{"list", "status", "stop"},
			},
			wantOrig: "lst",
			wantCorr: "list",
		},
		{
			name:    "levenshtein returns nil when distance too large",
			fullCmd: "testcli agent xyz",
			ctx: suggestContext{
				Kind:         FailureUnknownCommand,
				BadToken:     "xyz",
				ValidOptions: []string{"list", "status", "stop"},
			},
			wantNil: true,
		},
		{
			name:    "levenshtein returns nil when no valid options",
			fullCmd: "testcli agent statis",
			ctx: suggestContext{
				Kind:         FailureUnknownCommand,
				BadToken:     "statis",
				ValidOptions: []string{},
			},
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cat := newMockCatalog("testcli")
			engine := newSuggestionEngine(cat)

			suggestion := engine.suggestForCommand(tt.fullCmd, tt.ctx)

			if tt.wantNil {
				if suggestion != nil {
					t.Fatalf("expected nil, got %+v", suggestion)
				}
				return
			}

			if suggestion == nil {
				t.Fatal("expected non-nil suggestion")
			}
			if suggestion.Type != SuggestionLevenshtein {
				t.Errorf("type = %q, want %q", suggestion.Type, SuggestionLevenshtein)
			}
			if suggestion.Original != tt.wantOrig {
				t.Errorf("original = %q, want %q", suggestion.Original, tt.wantOrig)
			}
			if suggestion.Corrected != tt.wantCorr {
				t.Errorf("corrected = %q, want %q", suggestion.Corrected, tt.wantCorr)
			}
			if suggestion.Confidence <= 0.0 {
				t.Errorf("confidence = %f, should be > 0", suggestion.Confidence)
			}
		})
	}
}

func TestSuggestForCommand_ChainPriority(t *testing.T) {
	tests := []struct {
		name     string
		fullCmd  string
		ctx      suggestContext
		setupCat func(*mockCatalog)
		wantType SuggestionType
		wantCorr string
	}{
		{
			name:    "command remap takes priority over token lookup",
			fullCmd: "testcli agent stattus",
			ctx: suggestContext{
				Kind:         FailureUnknownCommand,
				BadToken:     "stattus",
				ValidOptions: []string{"status", "list"},
			},
			setupCat: func(c *mockCatalog) {
				c.addCommand("agent stattus", "agent status --verbose", 0.95, "common pattern")
				c.addToken("stattus", "status", FailureUnknownCommand, 0.88)
			},
			wantType: SuggestionCommandRemap,
			wantCorr: "agent status --verbose",
		},
		{
			name:    "token lookup takes priority over levenshtein",
			fullCmd: "testcli agent stattus",
			ctx: suggestContext{
				Kind:         FailureUnknownCommand,
				BadToken:     "stattus",
				ValidOptions: []string{"status", "list"},
			},
			setupCat: func(c *mockCatalog) {
				c.addToken("stattus", "state", FailureUnknownCommand, 0.88)
			},
			wantType: SuggestionTokenFix,
			wantCorr: "state",
		},
		{
			name:    "levenshtein used when no catalog matches",
			fullCmd: "testcli agent stattus",
			ctx: suggestContext{
				Kind:         FailureUnknownCommand,
				BadToken:     "stattus",
				ValidOptions: []string{"status", "list"},
			},
			setupCat: func(c *mockCatalog) {},
			wantType: SuggestionLevenshtein,
			wantCorr: "status",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cat := newMockCatalog("testcli")
			tt.setupCat(cat)
			engine := newSuggestionEngine(cat)

			suggestion := engine.suggestForCommand(tt.fullCmd, tt.ctx)

			if suggestion == nil {
				t.Fatal("expected non-nil suggestion")
			}
			if suggestion.Type != tt.wantType {
				t.Errorf("type = %q, want %q", suggestion.Type, tt.wantType)
			}
			if suggestion.Corrected != tt.wantCorr {
				t.Errorf("corrected = %q, want %q", suggestion.Corrected, tt.wantCorr)
			}
		})
	}
}

func TestSuggestForCommand_NilCatalog(t *testing.T) {
	tests := []struct {
		name     string
		fullCmd  string
		ctx      suggestContext
		wantNil  bool
		wantType SuggestionType
		wantCorr string
	}{
		{
			name:    "nil catalog skips to levenshtein",
			fullCmd: "testcli agent statis",
			ctx: suggestContext{
				Kind:         FailureUnknownCommand,
				BadToken:     "statis",
				ValidOptions: []string{"status", "list"},
			},
			wantType: SuggestionLevenshtein,
			wantCorr: "status",
		},
		{
			name:    "nil catalog returns nil when no valid options",
			fullCmd: "testcli agent statis",
			ctx: suggestContext{
				Kind:         FailureUnknownCommand,
				BadToken:     "statis",
				ValidOptions: []string{},
			},
			wantNil: true,
		},
		{
			name:    "nil catalog returns nil when no bad token",
			fullCmd: "testcli agent statis",
			ctx: suggestContext{
				Kind:         FailureUnknownCommand,
				BadToken:     "",
				ValidOptions: []string{"status", "list"},
			},
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			engine := newSuggestionEngine(nil)
			suggestion := engine.suggestForCommand(tt.fullCmd, tt.ctx)

			if tt.wantNil {
				if suggestion != nil {
					t.Fatalf("expected nil, got %+v", suggestion)
				}
				return
			}

			if suggestion == nil {
				t.Fatal("expected non-nil suggestion")
			}
			if suggestion.Type != tt.wantType {
				t.Errorf("type = %q, want %q", suggestion.Type, tt.wantType)
			}
			if suggestion.Corrected != tt.wantCorr {
				t.Errorf("corrected = %q, want %q", suggestion.Corrected, tt.wantCorr)
			}
		})
	}
}

func TestSuggestForCommand_EdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		fullCmd string
		ctx     suggestContext
	}{
		{name: "empty command", fullCmd: "", ctx: suggestContext{}},
		{name: "whitespace only command", fullCmd: "   ", ctx: suggestContext{}},
		{name: "just cli prefix", fullCmd: "testcli", ctx: suggestContext{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cat := newMockCatalog("testcli")
			engine := newSuggestionEngine(cat)

			suggestion := engine.suggestForCommand(tt.fullCmd, tt.ctx)
			if suggestion != nil {
				t.Errorf("expected nil for edge case, got %+v", suggestion)
			}
		})
	}
}

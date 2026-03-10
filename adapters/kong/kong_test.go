package kong

import (
	"errors"
	"testing"

	"github.com/alecthomas/kong"
	"github.com/sageox/frictionax"
)

// testGrammar is a struct-based CLI definition for kong.
type testGrammar struct {
	Verbose bool   `short:"v" help:"Enable verbose output."`
	Output  string `short:"o" help:"Output file."`

	Deploy deployCmd `cmd:"" help:"Deploy the application."`
	Config configCmd `cmd:"" help:"Manage configuration."`
}

type deployCmd struct {
	Env   string `short:"e" help:"Target environment."`
	Force bool   `help:"Force deployment."`

	Rollback rollbackCmd `cmd:"" help:"Rollback deployment."`
}

type rollbackCmd struct {
	Steps int `help:"Number of steps to rollback."`
}

type configCmd struct{}

func newTestKong(t *testing.T) *kong.Kong {
	t.Helper()
	var grammar testGrammar
	parser, err := kong.New(&grammar,
		kong.Name("testcli"),
		kong.Exit(func(int) {}), // prevent os.Exit in tests
	)
	if err != nil {
		t.Fatalf("failed to create kong parser: %v", err)
	}
	return parser
}

func TestCommandNames(t *testing.T) {
	adapter := NewKongAdapter(newTestKong(t))
	names := adapter.CommandNames()

	want := map[string]bool{
		"deploy":          true,
		"deploy rollback": true,
		"config":          true,
	}

	got := make(map[string]bool)
	for _, n := range names {
		got[n] = true
	}

	for name := range want {
		if !got[name] {
			t.Errorf("missing expected command name %q; got %v", name, names)
		}
	}
}

func TestFlagNames(t *testing.T) {
	adapter := NewKongAdapter(newTestKong(t))

	tests := []struct {
		name    string
		command string
		want    []string
	}{
		{
			name:    "global flags",
			command: "",
			want:    []string{"--verbose", "-v", "--output", "-o"},
		},
		{
			name:    "command flags",
			command: "deploy",
			want:    []string{"--env", "-e", "--force"},
		},
		{
			name:    "nested command flags",
			command: "deploy rollback",
			want:    []string{"--steps"},
		},
		{
			name:    "unknown command returns empty",
			command: "nonexistent",
			want:    nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := adapter.FlagNames(tt.command)

			if tt.want == nil {
				if len(got) != 0 {
					t.Errorf("expected empty flags, got %v", got)
				}
				return
			}

			wantSet := make(map[string]bool)
			for _, w := range tt.want {
				wantSet[w] = true
			}
			gotSet := make(map[string]bool)
			for _, g := range got {
				gotSet[g] = true
			}

			for w := range wantSet {
				if !gotSet[w] {
					t.Errorf("missing flag %q in result %v", w, got)
				}
			}
		})
	}
}

func TestParseError(t *testing.T) {
	adapter := NewKongAdapter(newTestKong(t))

	tests := []struct {
		name     string
		err      error
		wantKind frictionax.FailureKind
		wantBad  string
		wantNil  bool
	}{
		{
			name:    "nil error",
			err:     nil,
			wantNil: true,
		},
		{
			name:     "unknown flag long",
			err:      errors.New("unknown flag --foo"),
			wantKind: frictionax.FailureUnknownFlag,
			wantBad:  "--foo",
		},
		{
			name:     "unknown flag short",
			err:      errors.New("unknown flag -x"),
			wantKind: frictionax.FailureUnknownFlag,
			wantBad:  "-x",
		},
		{
			name:     "unknown command quoted",
			err:      errors.New(`unknown command "depoly"`),
			wantKind: frictionax.FailureUnknownCommand,
			wantBad:  "depoly",
		},
		{
			name:     "unexpected argument",
			err:      errors.New("unexpected argument foo"),
			wantKind: frictionax.FailureUnknownCommand,
			wantBad:  "foo",
		},
		{
			name:     "missing flags",
			err:      errors.New("missing flags: --env"),
			wantKind: frictionax.FailureMissingRequired,
			wantBad:  "--env",
		},
		{
			name:     "arg is required",
			err:      errors.New(`"name" is required`),
			wantKind: frictionax.FailureMissingRequired,
			wantBad:  "name",
		},
		{
			name:     "invalid value",
			err:      errors.New(`invalid value "abc" for flag --count`),
			wantKind: frictionax.FailureInvalidArg,
			wantBad:  "abc",
		},
		{
			name:     "strconv type error",
			err:      errors.New(`strconv.ParseInt: parsing "abc": invalid syntax`),
			wantKind: frictionax.FailureInvalidArg,
			wantBad:  "abc",
		},
		{
			name:    "unrecognized error",
			err:     errors.New("some other error"),
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := adapter.ParseError(tt.err)

			if tt.wantNil {
				if got != nil {
					t.Errorf("expected nil, got %+v", got)
				}
				return
			}

			if got == nil {
				t.Fatal("expected non-nil ParsedError")
			}
			if got.Kind != tt.wantKind {
				t.Errorf("kind = %q, want %q", got.Kind, tt.wantKind)
			}
			if got.BadToken != tt.wantBad {
				t.Errorf("bad token = %q, want %q", got.BadToken, tt.wantBad)
			}
		})
	}
}

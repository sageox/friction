package urfavecli

import (
	"errors"
	"testing"

	"github.com/sageox/frictionax"
	"github.com/urfave/cli/v2"
)

func newTestApp() *cli.App {
	return &cli.App{
		Name: "testcli",
		Flags: []cli.Flag{
			&cli.BoolFlag{Name: "verbose", Aliases: []string{"v"}},
			&cli.StringFlag{Name: "output", Aliases: []string{"o"}},
		},
		Commands: []*cli.Command{
			{
				Name:    "deploy",
				Aliases: []string{"d"},
				Flags: []cli.Flag{
					&cli.StringFlag{Name: "env", Aliases: []string{"e"}},
					&cli.BoolFlag{Name: "force"},
				},
				Subcommands: []*cli.Command{
					{
						Name: "rollback",
						Flags: []cli.Flag{
							&cli.IntFlag{Name: "steps"},
						},
					},
				},
			},
			{
				Name:   "status",
				Hidden: true,
			},
			{
				Name: "config",
			},
		},
	}
}

func TestCommandNames(t *testing.T) {
	adapter := NewUrfaveAdapter(newTestApp())
	names := adapter.CommandNames()

	want := map[string]bool{
		"deploy":          true,
		"d":               true, // alias
		"deploy rollback": true,
		"config":          true,
	}

	got := make(map[string]bool)
	for _, n := range names {
		got[n] = true
	}

	for name := range want {
		if !got[name] {
			t.Errorf("missing expected command name %q", name)
		}
	}

	// hidden commands should be excluded
	if got["status"] {
		t.Error("hidden command 'status' should not be included")
	}
}

func TestFlagNames(t *testing.T) {
	adapter := NewUrfaveAdapter(newTestApp())

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
	adapter := NewUrfaveAdapter(newTestApp())

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
			err:      errors.New("flag provided but not defined: --foo"),
			wantKind: frictionax.FailureUnknownFlag,
			wantBad:  "--foo",
		},
		{
			name:     "unknown flag short",
			err:      errors.New("flag provided but not defined: -x"),
			wantKind: frictionax.FailureUnknownFlag,
			wantBad:  "-x",
		},
		{
			name:     "unknown command",
			err:      errors.New(`command "depoly" not found`),
			wantKind: frictionax.FailureUnknownCommand,
			wantBad:  "depoly",
		},
		{
			name:     "required flag not set",
			err:      errors.New(`Required flag "env" not set`),
			wantKind: frictionax.FailureMissingRequired,
			wantBad:  "env",
		},
		{
			name:     "invalid value",
			err:      errors.New(`invalid value "abc" for flag --count`),
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

func TestFindCommandByAlias(t *testing.T) {
	adapter := NewUrfaveAdapter(newTestApp())
	flags := adapter.FlagNames("d") // alias for deploy
	if len(flags) == 0 {
		t.Error("expected flags for alias 'd' (deploy), got none")
	}
}

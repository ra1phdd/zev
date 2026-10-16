package main

import (
	"testing"

	"github.com/ra1phdd/zev-go/internal/core/config"
	"github.com/spf13/cobra"
)

func TestParseArgsFlags(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		args          []string
		wantArgs      []string
		wantVerbosity int
		wantExecute   bool
		wantInject    bool
		wantHelp      bool
	}{
		{
			name:          "no flags",
			args:          []string{"list", "files"},
			wantArgs:      []string{"list", "files"},
			wantVerbosity: 0,
		},
		{
			name:          "single v",
			args:          []string{"-v", "list"},
			wantArgs:      []string{"list"},
			wantVerbosity: 1,
		},
		{
			name:          "double vv",
			args:          []string{"-vv", "list"},
			wantArgs:      []string{"list"},
			wantVerbosity: 2,
		},
		{
			name:          "triple vvv",
			args:          []string{"-vvv", "list"},
			wantArgs:      []string{"list"},
			wantVerbosity: 3,
		},
		{
			name:          "quadruple vvvv",
			args:          []string{"-vvvv", "list"},
			wantArgs:      []string{"list"},
			wantVerbosity: 4,
		},
		{
			name:          "long verbose with value",
			args:          []string{"--verbose=2", "list"},
			wantArgs:      []string{"list"},
			wantVerbosity: 2,
		},
		{
			name:          "execute flag",
			args:          []string{"-e", "list"},
			wantArgs:      []string{"list"},
			wantVerbosity: 0,
			wantExecute:   true,
		},
		{
			name:          "inject long flag",
			args:          []string{"--inject", "list"},
			wantArgs:      []string{"list"},
			wantVerbosity: 0,
			wantInject:    true,
		},
		{
			name:          "alias with flags passed through",
			args:          []string{"dps", "-a", "--format", "json"},
			wantArgs:      []string{"dps", "-a", "--format", "json"},
			wantVerbosity: 0,
		},
		{
			name:          "zev flag mixed with alias args",
			args:          []string{"-vvv", "dps", "-a"},
			wantArgs:      []string{"dps", "-a"},
			wantVerbosity: 3,
		},
		{
			name:          "help flag",
			args:          []string{"-h"},
			wantArgs:      nil,
			wantVerbosity: 0,
			wantHelp:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cmd := &cobra.Command{Use: "query", RunE: func(cmd *cobra.Command, args []string) error {
				return nil
			}}
			cmd.Flags().BoolVar(&executeFlag, "execute", false, "")
			cmd.Flags().BoolVar(&injectFlag, "inject", false, "")

			verbosityLevel = 0
			executeFlag = false
			injectFlag = false

			gotArgs, gotHelp := parseArgsFlags(tt.args)

			if tt.wantHelp != gotHelp {
				t.Errorf("parseArgsFlags() help = %v, want %v", gotHelp, tt.wantHelp)
			}

			if tt.wantVerbosity != verbosityLevel {
				t.Errorf("verbosityLevel = %d, want %d", verbosityLevel, tt.wantVerbosity)
			}

			if tt.wantExecute != executeFlag {
				t.Errorf("executeFlag = %v, want %v", executeFlag, tt.wantExecute)
			}

			if tt.wantInject != injectFlag {
				t.Errorf("injectFlag = %v, want %v", injectFlag, tt.wantInject)
			}

			if len(gotArgs) != len(tt.wantArgs) {
				t.Fatalf("parseArgsFlags() args = %v, want %v", gotArgs, tt.wantArgs)
			}
			for i := range gotArgs {
				if gotArgs[i] != tt.wantArgs[i] {
					t.Errorf("parseArgsFlags() args = %v, want %v", gotArgs, tt.wantArgs)
				}
			}
		})
	}
}

func TestParseArgsFlagsLeavesVerbosityUnchanged(t *testing.T) {
	t.Parallel()

	verbosityLevel = 0
	executeFlag = false
	injectFlag = false

	args, _ := parseArgsFlags([]string{"-e"})

	if len(args) != 0 {
		t.Errorf("parseArgsFlags() args = %v, want empty", args)
	}
	if verbosityLevel != 0 {
		t.Errorf("verbosityLevel = %d, want 0", verbosityLevel)
	}
}

func TestResolveExecutionMode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		cfg         *config.Config
		executeFlag bool
		injectFlag  bool
		want        string
	}{
		{
			name:        "default config, no flags",
			cfg:         &config.Config{CommandExecutionMode: "inject"},
			executeFlag: false,
			injectFlag:  false,
			want:        "inject",
		},
		{
			name:        "config set to execute, no flags",
			cfg:         &config.Config{CommandExecutionMode: "execute"},
			executeFlag: false,
			injectFlag:  false,
			want:        "execute",
		},
		{
			name:        "config inject, execute flag overrides",
			cfg:         &config.Config{CommandExecutionMode: "inject"},
			executeFlag: true,
			injectFlag:  false,
			want:        "execute",
		},
		{
			name:        "config execute, inject flag overrides",
			cfg:         &config.Config{CommandExecutionMode: "execute"},
			executeFlag: false,
			injectFlag:  true,
			want:        "inject",
		},
		{
			name:        "empty config mode defaults to inject",
			cfg:         &config.Config{CommandExecutionMode: ""},
			executeFlag: false,
			injectFlag:  false,
			want:        "inject",
		},
		{
			name:        "execute flag with empty config",
			cfg:         &config.Config{CommandExecutionMode: ""},
			executeFlag: true,
			injectFlag:  false,
			want:        "execute",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := resolveExecutionMode(tt.cfg, tt.executeFlag, tt.injectFlag)
			if got != tt.want {
				t.Errorf("resolveExecutionMode() = %v, want %v", got, tt.want)
			}
		})
	}
}

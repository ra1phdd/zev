package main

import (
	"testing"

	"github.com/ra1phdd/zev/internal/core/config"
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

			got := parseArgsFlags(tt.args)

			if tt.wantHelp != got.Help {
				t.Errorf("parseArgsFlags() help = %v, want %v", got.Help, tt.wantHelp)
			}

			if tt.wantVerbosity != got.Verbosity {
				t.Errorf("parseArgsFlags() verbosity = %d, want %d", got.Verbosity, tt.wantVerbosity)
			}

			if tt.wantExecute != got.Execute {
				t.Errorf("parseArgsFlags() execute = %v, want %v", got.Execute, tt.wantExecute)
			}

			if tt.wantInject != got.Inject {
				t.Errorf("parseArgsFlags() inject = %v, want %v", got.Inject, tt.wantInject)
			}

			if len(got.Args) != len(tt.wantArgs) {
				t.Fatalf("parseArgsFlags() args = %v, want %v", got.Args, tt.wantArgs)
			}
			for i := range got.Args {
				if got.Args[i] != tt.wantArgs[i] {
					t.Errorf("parseArgsFlags() args = %v, want %v", got.Args, tt.wantArgs)
				}
			}
		})
	}
}

func TestParseArgsFlagsLeavesVerbosityUnchanged(t *testing.T) {
	t.Parallel()

	got := parseArgsFlags([]string{"-e"})

	if len(got.Args) != 0 {
		t.Errorf("parseArgsFlags() args = %v, want empty", got.Args)
	}
	if got.Verbosity != 0 {
		t.Errorf("parseArgsFlags() verbosity = %d, want 0", got.Verbosity)
	}
	if !got.Execute {
		t.Error("parseArgsFlags() execute = false, want true")
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

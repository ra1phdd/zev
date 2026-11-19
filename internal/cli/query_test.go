package cli

import (
	"fmt"
	"testing"

	"github.com/ra1phdd/zev/internal/core/config"
)

func TestParseArgsFlags(t *testing.T) {
	t.Parallel()

	const (
		listArgument  = "list"
		aliasArgument = "dps"
	)

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
			args:          []string{listArgument, "files"},
			wantArgs:      []string{listArgument, "files"},
			wantVerbosity: 0,
		},
		{
			name:          "single v",
			args:          []string{"-v", listArgument},
			wantArgs:      []string{listArgument},
			wantVerbosity: 1,
		},
		{
			name:          "double vv",
			args:          []string{"-vv", listArgument},
			wantArgs:      []string{listArgument},
			wantVerbosity: 2,
		},
		{
			name:          "triple vvv",
			args:          []string{"-vvv", listArgument},
			wantArgs:      []string{listArgument},
			wantVerbosity: 3,
		},
		{
			name:          "quadruple vvvv",
			args:          []string{"-vvvv", listArgument},
			wantArgs:      []string{listArgument},
			wantVerbosity: 4,
		},
		{
			name:          "long verbose with value",
			args:          []string{"--verbose=2", listArgument},
			wantArgs:      []string{listArgument},
			wantVerbosity: 2,
		},
		{
			name:          "execute flag",
			args:          []string{"-e", listArgument},
			wantArgs:      []string{listArgument},
			wantVerbosity: 0,
			wantExecute:   true,
		},
		{
			name:          "inject long flag",
			args:          []string{"--inject", listArgument},
			wantArgs:      []string{listArgument},
			wantVerbosity: 0,
			wantInject:    true,
		},
		{
			name:          "alias with flags passed through",
			args:          []string{aliasArgument, "-a", "--format", historyExportJSON},
			wantArgs:      []string{aliasArgument, "-a", "--format", historyExportJSON},
			wantVerbosity: 0,
		},
		{
			name:          "zev flag mixed with alias args",
			args:          []string{"-vvv", aliasArgument, "-a"},
			wantArgs:      []string{aliasArgument, "-a"},
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

func TestParseArgsFlagsSupportsRepeatedContextFiles(t *testing.T) {
	t.Parallel()
	got := parseArgsFlags([]string{historyValidationQuery, "--context-file", "a.txt", "-cf", "b.txt", "--context-file=c.txt"})
	want := []string{"a.txt", "b.txt", "c.txt"}
	if fmt.Sprint(got.ContextFiles) != fmt.Sprint(want) {
		t.Fatalf("context files = %v, want %v", got.ContextFiles, want)
	}
	if got.Args[0] != "query" {
		t.Fatalf("args = %v, want query preserved", got.Args)
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

func TestParseArgsFlagsProxy(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name  string
		args  []string
		proxy string
		err   string
	}{
		{name: "separate value", args: []string{cliProxyFlag, "socks5://localhost:1080", queryFlowFixturePrompt}, proxy: "socks5://localhost:1080"},
		{name: "equals value", args: []string{"--proxy=http://proxy.example:8080", queryFlowFixturePrompt}, proxy: "http://proxy.example:8080"},
		{name: "missing value", args: []string{cliProxyFlag}, err: errProxyRequiresURL},
		{name: "empty separate value", args: []string{cliProxyFlag, "", queryFlowFixturePrompt}, err: errProxyRequiresURL},
		{name: "next option is not a value", args: []string{cliProxyFlag, "--continue", queryFlowFixturePrompt}, err: errProxyRequiresURL},
		{name: "empty equals value", args: []string{"--proxy="}, err: errProxyRequiresURL},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := parseArgsFlags(tc.args)
			if got.Proxy != tc.proxy || got.ParseError != tc.err {
				t.Fatalf("parseArgsFlags() proxy=%q error=%q, want proxy=%q error=%q", got.Proxy, got.ParseError, tc.proxy, tc.err)
			}
			if len(got.Args) != 1 || got.Args[0] != "show files" {
				if tc.err != "" {
					return
				}
				t.Fatalf("query arguments = %v, want [show files]", got.Args)
			}
		})
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
			cfg:         &config.Config{CommandExecutionMode: executionModeInject},
			executeFlag: false,
			injectFlag:  false,
			want:        executionModeInject,
		},
		{
			name:        "config set to execute, no flags",
			cfg:         &config.Config{CommandExecutionMode: executionModeExecute},
			executeFlag: false,
			injectFlag:  false,
			want:        executionModeExecute,
		},
		{
			name:        "config inject, execute flag overrides",
			cfg:         &config.Config{CommandExecutionMode: executionModeInject},
			executeFlag: true,
			injectFlag:  false,
			want:        executionModeExecute,
		},
		{
			name:        "config execute, inject flag overrides",
			cfg:         &config.Config{CommandExecutionMode: executionModeExecute},
			executeFlag: false,
			injectFlag:  true,
			want:        executionModeInject,
		},
		{
			name:        "empty config mode defaults to inject",
			cfg:         &config.Config{CommandExecutionMode: ""},
			executeFlag: false,
			injectFlag:  false,
			want:        executionModeInject,
		},
		{
			name:        "execute flag with empty config",
			cfg:         &config.Config{CommandExecutionMode: ""},
			executeFlag: true,
			injectFlag:  false,
			want:        executionModeExecute,
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

package main

import (
	"testing"

	"github.com/ra1phdd/zev-go/internal/core/config"
)

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

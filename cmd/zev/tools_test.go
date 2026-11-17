package main

import (
	"strings"
	"testing"

	"github.com/ra1phdd/zev/internal/core/config"
)

const (
	toolTestPrompt   = "inspect"
	toolEnableFlag   = "--tools"
	toolDisableFlag  = "--no-tools"
	toolApprovalFlag = "--tool-approval"
)

func TestToolFlagParsingAndMachineOptIn(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		args    []string
		enabled bool
		mode    string
		invalid bool
	}{
		{name: "interactive default", args: []string{toolTestPrompt}, enabled: true, mode: toolModeManual},
		{name: "machine default", args: []string{machinePrintFlag, toolTestPrompt}, mode: toolModeManual},
		{name: "machine mode flag insufficient", args: []string{"--json", toolApprovalFlag, toolModeBypass, toolTestPrompt}, mode: toolModeBypass},
		{name: "machine explicit", args: []string{machinePrintFlag, toolEnableFlag, "--tool-approval=auto", toolTestPrompt}, enabled: true, mode: toolModeAuto},
		{name: "disabled", args: []string{toolDisableFlag, toolTestPrompt}, mode: toolModeManual},
		{name: "invalid approval mode", args: []string{"--tool-approval=oops", toolTestPrompt}, invalid: true},
		{name: "missing approval value", args: []string{toolApprovalFlag}, invalid: true},
		{name: "conflict", args: []string{toolEnableFlag, toolDisableFlag, toolTestPrompt}, invalid: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			flags := parseArgsFlags(tc.args)
			err := validateMachineFlags(flags)
			if (err != nil) != tc.invalid {
				t.Fatalf("error %v flags %+v", err, flags)
			}
			if tc.invalid {
				return
			}
			enabled, mode := resolveToolSettings(config.DefaultConfig(), flags)
			if enabled != tc.enabled || mode != tc.mode {
				t.Fatalf("enabled=%t mode=%s", enabled, mode)
			}
			if strings.Join(flags.Args, " ") != toolTestPrompt {
				t.Fatalf("query=%q", flags.Args)
			}
		})
	}
}

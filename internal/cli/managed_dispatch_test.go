package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ra1phdd/zev/internal/core/config"
	"github.com/ra1phdd/zev/internal/core/domain"
	aliasrepo "github.com/ra1phdd/zev/internal/entities/alias/repository"
	aliasservice "github.com/ra1phdd/zev/internal/entities/alias/service"
	"github.com/spf13/cobra"
)

func TestManagedDispatchOriginalArgv(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	writeCLIConfig(t, home, invalidProviderFixture())
	dir, err := config.GetZevDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := aliasservice.New(aliasrepo.New(dir)).AddAlias(domain.Alias{Name: managedCaptureFixture, Template: "printf {{.args}}", Description: "Exact argv"}); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		tail []string
		want string
	}{
		{[]string{"a b", "", historyJSONFlag, "--", localizationHelpFlag}, "printf 'a b' '' '--json' '--help'"},
		{[]string{"--", localizationHelpFlag, "--"}, "printf '--help' '--'"},
		{[]string{"--context-file", managedMissingFixture}, "printf '--context-file' 'missing'"},
	} {
		root := NewRootCommand("test")
		var out bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&out)
		root.SetArgs(append([]string{machinePrintFlag, managedCaptureFixture}, tt.tail...))
		if err := root.ExecuteContext(t.Context()); err != nil {
			t.Fatal(err)
		}
		if strings.TrimSpace(out.String()) != tt.want {
			t.Errorf("got %q want %q", out.String(), tt.want)
		}
	}
	marker := filepath.Join(home, "marker")
	root := NewRootCommand("test")
	root.SetArgs([]string{machinePrintFlag, managedCaptureFixture, "$(touch " + marker + ")"})
	root.SetOut(&bytes.Buffer{})
	if err := root.ExecuteContext(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("machine alias executed")
	}
}

func TestManagedExecutionUsesCommandDeliveryPolicy(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mode   string
		wantOK bool
	}{
		{name: "normal requires review", mode: config.SecurityModeNormal},
		{name: "bypass is scoped peer mode", mode: config.SecurityModeBypass, wantOK: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)
			cfg := config.DefaultConfig()
			cfg.Security.Mode = tc.mode
			writeCLIConfig(t, home, cfg)
			cmd := &cobra.Command{}
			cmd.SetContext(t.Context())
			cmd.SetIn(bytes.NewBuffer(nil))
			cmd.SetErr(&bytes.Buffer{})
			err := authorizeManagedCommand(cmd, "project-reader --opaque")
			if (err == nil) != tc.wantOK {
				t.Fatalf("authorizeManagedCommand() error = %v, wantOK=%t", err, tc.wantOK)
			}
		})
	}
}

func TestManagedDispatchKeepsExplicitQueryAndRootHelp(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	dir, err := config.GetZevDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := aliasservice.New(aliasrepo.New(dir)).AddAlias(domain.Alias{Name: managedNamedFixture, Template: managedEchoFixture}); err != nil {
		t.Fatal(err)
	}
	query := newQueryCmd()
	if handled, err := dispatchOriginalArgs(query, []string{"named"}); handled || err != nil {
		t.Fatalf("explicit query dispatch handled=%v err=%v", handled, err)
	}
	root := NewRootCommand("test")
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{entityHelpFlag, "named"})
	if err := root.ExecuteContext(t.Context()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Zev") && !strings.Contains(out.String(), "Usage:") {
		t.Fatalf("root help not shown: %q", out.String())
	}
	for _, args := range [][]string{{"--verbose", "--help", managedNamedFixture}, {machinePrintFlag, "--help", managedNamedFixture}} {
		root = NewRootCommand("test")
		out.Reset()
		root.SetOut(&out)
		root.SetErr(&out)
		root.SetArgs(args)
		if err := root.ExecuteContext(t.Context()); err != nil {
			t.Fatalf("root help args %v: %v", args, err)
		}
		if !strings.Contains(out.String(), "Usage:") && !strings.Contains(out.String(), "Zev") {
			t.Fatalf("root help not shown for %v: %q", args, out.String())
		}
	}
}

func TestExplicitQueryPrintUsesQueryProviderPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	writeCLIConfig(t, home, invalidProviderFixture())
	dir, err := config.GetZevDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := aliasservice.New(aliasrepo.New(dir)).AddAlias(domain.Alias{Name: managedNamedFixture, Template: managedEchoFixture}); err != nil {
		t.Fatal(err)
	}
	root := NewRootCommand("test")
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"query", machinePrintFlag, managedNamedFixture})
	if err := root.ExecuteContext(t.Context()); err != nil {
		t.Fatalf("explicit query failed: %v", err)
	}
	if got := strings.TrimSpace(out.String()); got != managedEchoFixture {
		t.Fatalf("explicit query output = %q, want %q", got, managedEchoFixture)
	}
}

func TestManagedEntityHelpBeforeProvider(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	writeCLIConfig(t, home, map[string]any{cliConfigVersionKey: 1, "tool_approval_mode": cliFixtureInvalidValue})
	dir, err := config.GetZevDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := aliasservice.New(aliasrepo.New(dir)).AddAlias(domain.Alias{Name: toolTestPrompt, Template: managedTemplateFixture, Description: "Inspect service", Arguments: []domain.Argument{{Name: "service", Required: true}}}); err != nil {
		t.Fatal(err)
	}
	root := NewRootCommand("test")
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetArgs([]string{toolTestPrompt, localizationHelpFlag})
	if err := root.ExecuteContext(t.Context()); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Inspect service", "service", "argv-safe", managedTemplateFixture, "zev inspect"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("help missing %q: %s", want, out.String())
		}
	}
	root = NewRootCommand("test")
	root.SetArgs([]string{toolTestPrompt, "value", localizationHelpFlag})
	root.SetOut(&bytes.Buffer{})
	err = root.ExecuteContext(t.Context())
	if e, ok := errors.AsType[*machineExitError](err); !ok || e.code != exitArguments {
		t.Fatalf("want usage error got %v", err)
	}
}

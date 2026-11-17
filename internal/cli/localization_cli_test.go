package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ra1phdd/zev/internal/core/domain"
	"github.com/ra1phdd/zev/internal/entities/history/service"
	"github.com/spf13/cobra"
)

const localizationHelpFlag = "--help"

func TestCLILocalizationAndMachineStability(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	dir := filepath.Join(home, ".zev")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "aliases"), []byte("ll: ls -la\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Run("Russian alias help and list", func(t *testing.T) {
		t.Setenv("ZEV_LANG", "ru")
		var help bytes.Buffer
		helpCmd := newAliasCmd()
		helpCmd.SetOut(&help)
		helpCmd.SetErr(io.Discard)
		helpCmd.SetArgs([]string{localizationHelpFlag})
		if err := helpCmd.Execute(); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(help.String(), "Создание, просмотр и удаление алиасов команд с подстановкой аргументов") {
			t.Fatalf("Russian alias help = %q", help.String())
		}

		var list bytes.Buffer
		listCmd := newAliasListCmd()
		listCmd.SetOut(&list)
		listCmd.SetErr(io.Discard)
		if err := listCmd.Execute(); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(list.String(), "ИМЯ") || !strings.Contains(list.String(), "ШАБЛОН") || !strings.Contains(list.String(), "ll") || !strings.Contains(list.String(), "ls -la") {
			t.Fatalf("Russian alias list = %q", list.String())
		}
	})

	t.Run("English alias list", func(t *testing.T) {
		t.Setenv("ZEV_LANG", "en")
		var out bytes.Buffer
		cmd := newAliasListCmd()
		cmd.SetOut(&out)
		cmd.SetErr(io.Discard)
		if err := cmd.Execute(); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out.String(), "NAME") || !strings.Contains(out.String(), "TEMPLATE") || !strings.Contains(out.String(), "ll") || !strings.Contains(out.String(), "ls -la") {
			t.Fatalf("English alias list = %q", out.String())
		}
	})

	t.Run("machine JSON remains stable", func(t *testing.T) {
		t.Setenv("ZEV_LANG", "ru")
		const commandText = "printf localization-fixture"
		response := &domain.OptionsResponse{IsValid: true, Commands: []domain.Command{{Command: commandText}}}
		var out bytes.Buffer
		cmd := newAliasListCmd()
		cmd.SetOut(&out)
		cmd.SetErr(io.Discard)
		if err := writeMachineResult(cmd, parsedFlags{JSON: true, Option: 1}, response); err != nil {
			t.Fatal(err)
		}
		var got machineOutput
		if err := json.Unmarshal(out.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if got.Status != "ok" || got.SelectedOption != 1 || len(got.Commands) != 1 || got.Commands[0].Command != commandText {
			t.Fatalf("machine result changed: %+v", got)
		}
	})
}

func TestCLIHelpLanguages(t *testing.T) {
	for _, tc := range []struct {
		name       string
		newCommand func() *cobra.Command
		english    string
		russian    string
	}{
		{"alias variables", newAliasAddCmd, "Template variables:", "Переменные шаблона:"},
		{"script creation", newScriptCreateCmd, "Create a new bash script", "Создать bash-скрипт"},
		{"history search", func() *cobra.Command { return newHistorySearchCmd(nil) }, "Search mode:", "Режим поиска:"},
		{"history index", func() *cobra.Command { return newHistoryIndexRebuildCmd(nil) }, "Rebuild semantic vectors", "Перестроить семантические векторы"},
		{"doctor flags", newDoctorCmd, "Output a stable JSON report", "Вывести отчёт JSON со стабильной схемой"},
		{"setup headers", newSetupHeadersCmd, "Headers are applied", "Заголовки применяются"},
		{"setup mode", newSetupModeCmd, "Insert command into terminal", "Вставить команду в терминал"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, language := range []string{"en", "ru"} {
				t.Setenv("ZEV_LANG", language)
				var out bytes.Buffer
				cmd := tc.newCommand()
				cmd.SetOut(&out)
				cmd.SetErr(io.Discard)
				cmd.SetArgs([]string{localizationHelpFlag})
				if err := cmd.Execute(); err != nil {
					t.Fatal(err)
				}
				want := tc.english
				if language == "ru" {
					want = tc.russian
				}
				if !strings.Contains(out.String(), want) {
					t.Errorf("%s help missing %q: %q", language, want, out.String())
				}
				if tc.name == "alias variables" && !strings.Contains(out.String(), "{{.arg1}}") {
					t.Errorf("template example was changed: %q", out.String())
				}
			}
		})
	}
}

func TestDoctorLocalizedPresentationPreservesJSON(t *testing.T) {
	t.Setenv("ZEV_LANG", "en")
	report := doctorReport{SchemaVersion: 1, Status: doctorStatusOK, Checks: []doctorCheck{{Name: "configuration", Status: doctorStatusOK, Message: "configuration loaded", ActionableMessage: doctorActionNone}}, ActionableMessage: "all checks passed"}
	var previous string
	for _, language := range []string{"en", "ru"} {
		t.Setenv("ZEV_LANG", language)
		var human, machine bytes.Buffer
		cmd := &cobra.Command{}
		cmd.SetOut(&human)
		if err := writeDoctorReport(cmd, report, doctorOptions{Print: true}); err != nil {
			t.Fatal(err)
		}
		want := "configuration loaded"
		if language == "ru" {
			want = "Настройки загружены"
		}
		if !strings.Contains(human.String(), want) || !strings.Contains(human.String(), "[ok] configuration:") {
			t.Fatalf("%s doctor report = %q", language, human.String())
		}
		cmd.SetOut(&machine)
		if err := writeDoctorReport(cmd, report, doctorOptions{JSON: true}); err != nil {
			t.Fatal(err)
		}
		if previous != "" && machine.String() != previous {
			t.Fatalf("doctor JSON changed with language: %q != %q", machine.String(), previous)
		}
		previous = machine.String()
	}
}

func TestLocalizedSearchWarningsPreserveJSON(t *testing.T) {
	const warning = "Semantic search unavailable; using lexical search."
	result := service.SearchResult{Warning: warning}
	t.Setenv("ZEV_LANG", "ru")
	var out, diagnostic bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&out)
	cmd.SetErr(&diagnostic)
	if err := writeHistorySearchResult(cmd, result, false); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(diagnostic.String(), "Семантический поиск недоступен") {
		t.Fatalf("Russian search warning = %q", diagnostic.String())
	}
	if err := writeHistorySearchResult(cmd, result, true); err != nil {
		t.Fatal(err)
	}
	var decoded service.SearchResult
	if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Warning != warning {
		t.Fatalf("machine search warning changed: %q", decoded.Warning)
	}
}

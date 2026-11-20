package locale

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"maps"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
)

func TestNamedMessageKeysHaveEnglishAndRussianCatalogEntries(t *testing.T) {
	t.Parallel()

	wantEnglish := map[Key]string{
		KeyUIConfirmHint:              "(y/Enter: confirm, n: decline, Esc/Ctrl+C: cancel)",
		KeyUIMultilineHint:            "(Enter: newline, Ctrl+D: submit or skip, Esc/Ctrl+C: cancel; 64 KiB / 10000 lines maximum)",
		KeyUIListNavigationHint:       "(↑/↓ select, Enter confirm, Esc cancel)",
		KeyCliHistoryCommandDelivery:  "     Outcome: %s\n",
		KeyCliHistoryCommandWarning:   "     Warning: %s\n",
		KeyCliHistoryDangerousCommand: "Dangerous command",
		KeyCliHistoryInvalidResponse:  "The provider returned an invalid response.",
		KeyCliHistoryMessage:          "Message %s\n",
		KeyCliHistoryRole:             "Role: %s\n",
		KeyCliHistoryRoleAssistant:    "ASSISTANT",
		KeyCliHistoryRoleUser:         "USER",
		KeyCliHelpCommandRemoved:      "use --help or -h",
		KeyUISetupCurrentDefault:      "%s (current: %s, press Enter to keep or type 'default' for %s):",
		KeyUISetupCurrent:             "%s (current: %s, press Enter to keep):",
		KeyUISetupDefault:             "%s (default: %s):",
		KeyUISetupProvider:            "Which LLM provider would you like to use?",
		KeyUISetupSaved:               "\n✓ Configuration saved successfully!",
		KeyUISetupModel:               "Enter the model to use",
		KeyUISetupModelLoadFailed:     "Could not load models from the provider; enter one manually (%v).",
		KeyUISetupSelectModel:         "Select a model (loaded from the provider)",
		KeyUISetupManualModel:         "Enter a model manually",
		KeyUISetupBaseURL:             "Enter custom base URL",
		KeyUISetupOllamaBaseURL:       "Enter Ollama base URL",
		KeyUISetupAPIKey:              "Enter your %s API key",
		KeyUISetupCurrentAPIKey:       " (current: %s, press Enter to keep)",
		KeyToolsApprovalPrompt:        "Approve with y or Enter; decline with n; Esc cancels.",
	}
	catalogs := loadTestCatalogs(t)
	for key, want := range wantEnglish {
		id := string(key)
		if !strings.Contains(id, ".") || strings.ContainsAny(id, " \t\r\n") {
			t.Errorf("message key %q is not a stable identifier", id)
		}
		for _, language := range []string{"en", "ru"} {
			message, ok := catalogs[language][id]
			if !ok {
				t.Errorf("missing %s catalog entry for %q", language, id)
				continue
			}
			if message["other"] == "" {
				t.Errorf("empty %s catalog entry for %q", language, id)
			}
			if language == "en" && message["other"] != want {
				t.Errorf("English value for %q = %q, want %q", id, message["other"], want)
			}
		}
	}
}

func TestAllCatalogEntriesUseNamedStableIDs(t *testing.T) {
	t.Parallel()

	paths, err := fs.Glob(files, "*.toml")
	if err != nil {
		t.Fatal(err)
	}
	catalogIDs := make(map[string]struct{})
	for _, path := range paths {
		data, readErr := fs.ReadFile(files, path)
		if readErr != nil {
			t.Fatal(readErr)
		}
		var messages map[string]map[string]string
		if err := toml.Unmarshal(data, &messages); err != nil {
			t.Fatal(err)
		}
		for id := range messages {
			if !strings.Contains(id, ".") || strings.ContainsAny(id, " \t\r\n") {
				t.Errorf("catalog %s has non-identifier message ID %q", path, id)
			}
			catalogIDs[id] = struct{}{}
		}
	}
	constantIDs := make(map[string]struct{})
	for _, keysPath := range []string{"keys.go", "provider_keys.go"} {
		keysFile, err := os.ReadFile(keysPath) //nolint:gosec // Test reads package-local source files.
		if err != nil {
			t.Fatal(err)
		}
		file, err := parser.ParseFile(token.NewFileSet(), keysPath, keysFile, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, declaration := range file.Decls {
			declaration, ok := declaration.(*ast.GenDecl)
			if !ok || declaration.Tok != token.CONST {
				continue
			}
			for _, name := range declaration.Specs {
				values, ok := name.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for valueIndex, constant := range values.Names {
					if !strings.HasPrefix(constant.Name, "Key") || valueIndex >= len(values.Values) {
						continue
					}
					literal, ok := values.Values[valueIndex].(*ast.BasicLit)
					if !ok || literal.Kind != token.STRING {
						t.Errorf("constant %s is not a string key", constant.Name)
						continue
					}
					id, err := strconv.Unquote(literal.Value)
					if err != nil {
						t.Errorf("constant %s has invalid string: %v", constant.Name, err)
						continue
					}
					constantIDs[id] = struct{}{}
					if _, ok := catalogIDs[id]; !ok {
						t.Errorf("constant %s has no catalog entry for %q", constant.Name, id)
					}
				}
			}
		}
	}
	if len(constantIDs) != len(catalogIDs) {
		t.Errorf("catalog IDs=%d, named constants=%d", len(catalogIDs), len(constantIDs))
	}
	for id := range catalogIDs {
		if _, ok := constantIDs[id]; !ok {
			t.Errorf("catalog ID %q has no named constant", id)
		}
	}
}

func loadTestCatalogs(t *testing.T) map[string]map[string]map[string]string {
	t.Helper()
	catalogs := map[string]map[string]map[string]string{"en": {}, "ru": {}}
	for _, path := range []string{"ui.en.toml", "ui.ru.toml", "tools.en.toml", "tools.ru.toml", "cli.en.toml", "cli.ru.toml"} {
		data, err := fs.ReadFile(files, path)
		if err != nil {
			t.Fatal(err)
		}
		var messages map[string]map[string]string
		if err := toml.Unmarshal(data, &messages); err != nil {
			t.Fatal(err)
		}
		language := "en"
		if strings.HasSuffix(path, ".ru.toml") {
			language = "ru"
		}
		maps.Copy(catalogs[language], messages)
	}
	return catalogs
}

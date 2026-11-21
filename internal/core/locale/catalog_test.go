package locale

import (
	"fmt"
	"io/fs"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
)

func TestCatalogsHaveMatchingMessagesAndFormatVerbs(t *testing.T) {
	t.Parallel()
	paths, err := fs.Glob(files, "*.toml")
	if err != nil {
		t.Fatal(err)
	}
	catalogs := map[string]map[string]map[string]string{"en": {}, "ru": {}}
	for _, path := range paths {
		data, readErr := fs.ReadFile(files, path)
		if readErr != nil {
			t.Fatal(readErr)
		}
		var messages map[string]map[string]string
		if err := toml.Unmarshal(data, &messages); err != nil {
			t.Fatal(err)
		}
		lang := "en"
		if strings.HasSuffix(path, "ru.toml") {
			lang = "ru"
		}
		for id, message := range messages {
			if old, exists := catalogs[lang][id]; exists && old["other"] != message["other"] {
				t.Errorf("conflicting %s translation for %q", lang, id)
			}
			catalogs[lang][id] = message
		}
	}
	for id, en := range catalogs["en"] {
		ru, ok := catalogs["ru"][id]
		if !ok {
			t.Errorf("missing Russian message %q", id)
			continue
		}
		if ru["other"] == "" {
			t.Errorf("empty Russian message %q", id)
		}
		if fmt.Sprint(formatVerbs(en["other"])) != fmt.Sprint(formatVerbs(ru["other"])) {
			t.Errorf("placeholder mismatch for %q", id)
		}
	}
	for id := range catalogs["ru"] {
		if _, ok := catalogs["en"][id]; !ok {
			t.Errorf("missing English message %q", id)
		}
	}
}

func TestCatalogEntriesKeepLayoutPunctuationOutOfOtherValues(t *testing.T) {
	t.Parallel()
	paths, err := fs.Glob(files, "*.toml")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		data, err := fs.ReadFile(files, path)
		if err != nil {
			t.Fatal(err)
		}
		var messages map[string]map[string]string
		if err := toml.Unmarshal(data, &messages); err != nil {
			t.Fatal(err)
		}
		for id, message := range messages {
			value := message["other"]
			if value != strings.TrimSpace(value) {
				t.Errorf("%s:%s keeps edge whitespace: %q", path, id, value)
			}
			if strings.HasSuffix(value, ":") {
				t.Errorf("%s:%s keeps a trailing colon: %q", path, id, value)
			}
		}
	}
}

func formatVerbs(s string) []string {
	var verbs []string
	for i := 0; i < len(s); i++ {
		if s[i] != '%' {
			continue
		}
		start := i
		i++
		for i < len(s) && strings.ContainsRune("# +-.0123456789[]", rune(s[i])) {
			i++
		}
		if i < len(s) {
			verbs = append(verbs, s[start:i+1])
		}
	}
	return verbs
}

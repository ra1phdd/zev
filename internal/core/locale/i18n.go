// Package locale embeds and resolves user-facing message catalogs.
package locale

import (
	"io/fs"
	"os"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"github.com/ra1phdd/zev"
	"golang.org/x/text/language"
)

func mustCatalogFiles() fs.FS {
	files, err := fs.Sub(zev.LocaleFiles, "locale")
	if err != nil {
		panic(err)
	}
	return files
}

var files = mustCatalogFiles()

var (
	bundle     = newBundle()
	sourceKeys = newSourceKeys()
)

func newBundle() *i18n.Bundle {
	b := i18n.NewBundle(language.English)
	b.RegisterUnmarshalFunc("toml", toml.Unmarshal)
	paths, err := fs.Glob(files, "*.toml")
	if err != nil {
		panic(err)
	}
	for _, path := range paths {
		if _, err := b.LoadMessageFileFS(files, path); err != nil {
			panic(err)
		}
	}
	return b
}

func newSourceKeys() map[string]Key {
	keys := make(map[string]Key)
	paths, err := fs.Glob(files, "*.toml")
	if err != nil {
		panic(err)
	}
	for _, path := range paths {
		if !strings.HasSuffix(path, ".en.toml") {
			continue
		}
		data, err := fs.ReadFile(files, path)
		if err != nil {
			panic(err)
		}
		var messages map[string]map[string]string
		if err := toml.Unmarshal(data, &messages); err != nil {
			panic(err)
		}
		for id, message := range messages {
			source := message["other"]
			if source == "" {
				continue
			}
			// Multiple stable keys may intentionally share one English rendering
			// (for example, the generic and tool approval hints). Reverse lookup
			// only needs one canonical key for that source value.
			if _, exists := keys[source]; !exists {
				keys[source] = Key(id)
			}
		}
	}
	return keys
}

// Language returns the process-wide UI language. Configured ru/en values take
// precedence over request and system locale. Unsupported system locales use English.
func Language() string {
	if selected := selectedLanguage.Load(); selected != nil {
		if selected.configured == "ru" || selected.configured == "en" {
			return selected.configured
		}
		if selected.language != "" {
			return selected.language
		}
		return environmentLanguage(os.Getenv("LC_ALL"), os.Getenv("LC_MESSAGES"), os.Getenv("LANG"))
	}
	return environmentLanguage(os.Getenv("LC_ALL"), os.Getenv("LC_MESSAGES"), os.Getenv("LANG"))
}

func environmentLanguage(values ...string) string {
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" {
			continue
		}
		value = strings.ReplaceAll(strings.SplitN(strings.SplitN(value, ".", 2)[0], "@", 2)[0], "_", "-")
		tag, err := language.Parse(value)
		if err == nil {
			base, _ := tag.Base()
			if base.String() == "ru" {
				return "ru"
			}
		}
		return "en"
	}
	return "en"
}

// Text translates a message using a stable message key.
// Missing translations intentionally return the key itself.
func Text(key Key) string {
	localizer := i18n.NewLocalizer(bundle, Language())
	value, err := localizer.Localize(&i18n.LocalizeConfig{MessageID: string(key), DefaultMessage: &i18n.Message{ID: string(key), Other: string(key)}})
	if err != nil || value == "" {
		return string(key)
	}
	return value
}

// TextSource translates a dynamic English catalog value at a localization boundary.
// Unknown dynamic values remain unchanged instead of becoming catalog IDs.
func TextSource(source string) string {
	key, ok := sourceKeys[source]
	if !ok {
		return source
	}
	return Text(key)
}

// Package locale embeds and resolves user-facing message catalogs.
package locale

import (
	"embed"
	"io/fs"
	"os"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"golang.org/x/text/language"
)

//go:embed *.toml
var files embed.FS

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
		if path != "en.toml" && !strings.HasSuffix(path, ".en.toml") {
			continue
		}
		data, err := files.ReadFile(path)
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
			if previous, exists := keys[source]; exists && previous != Key(id) {
				panic("duplicate English localization value: " + source)
			}
			keys[source] = Key(id)
		}
	}
	return keys
}

// Language returns the configured language. ZEV_LANG takes precedence over
// LC_ALL, LC_MESSAGES and LANG. Unsupported languages fall back to English.
func Language() string {
	for _, value := range []string{os.Getenv("ZEV_LANG"), os.Getenv("LC_ALL"), os.Getenv("LC_MESSAGES"), os.Getenv("LANG")} {
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

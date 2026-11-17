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

var bundle = newBundle()

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

// Text translates a message using the source string as its stable message ID.
// Missing translations intentionally return the source string.
func Text(source string) string {
	localizer := i18n.NewLocalizer(bundle, Language())
	value, err := localizer.Localize(&i18n.LocalizeConfig{MessageID: source, DefaultMessage: &i18n.Message{ID: source, Other: source}})
	if err != nil || value == "" {
		return source
	}
	return value
}

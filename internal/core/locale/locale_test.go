package locale

import (
	"testing"
)

const (
	localeLangEnv = "LANG"
	localeAllEnv  = "LC_ALL"
)

const (
	localeOverrideEnv  = "ZEV_LANG"
	localeRussianPOSIX = "ru_RU.UTF-8"
)

func TestLanguagePreference(t *testing.T) {
	for _, tc := range []struct {
		name string
		env  map[string]string
		want string
	}{
		{"default", nil, "en"},
		{"Russian POSIX", map[string]string{localeLangEnv: localeRussianPOSIX}, "ru"},
		{"LC_ALL wins", map[string]string{localeLangEnv: localeRussianPOSIX, localeAllEnv: "C"}, "en"},
		{"unknown fallback", map[string]string{localeLangEnv: "fr_FR.UTF-8"}, "en"},
		{"unknown LC_ALL wins", map[string]string{localeLangEnv: localeRussianPOSIX, localeAllEnv: "fr_FR.UTF-8"}, "en"},
		{"C UTF-8 wins", map[string]string{localeLangEnv: localeRussianPOSIX, localeAllEnv: "C.UTF-8"}, "en"},
		{"BCP47 regional Russian", map[string]string{localeLangEnv: "ru-BY"}, "ru"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, key := range []string{localeOverrideEnv, localeAllEnv, "LC_MESSAGES", localeLangEnv} {
				t.Setenv(key, tc.env[key])
			}
			if got := Language(); got != tc.want {
				t.Fatalf("language=%s, want %s", got, tc.want)
			}
		})
	}
}

//nolint:paralleltest // Changes the process-wide locale.
func TestTextCatalogAndFallback(t *testing.T) {
	restore := SetLanguage("ru", "")
	t.Cleanup(restore)
	if got := Text(KeyQueryFetchingSuggestions); got != "Получение вариантов команд...\n" {
		t.Fatalf("translation=%q", got)
	}
	if got := Text("unknown message %s"); got != "unknown message %s" {
		t.Fatalf("fallback=%q", got)
	}
	restoreEnglish := SetLanguage("en", "")
	defer restoreEnglish()
	if got := Text(KeyQueryFetchingSuggestions); got != "Fetching suggestions...\n" {
		t.Fatalf("English=%q", got)
	}
}

//nolint:paralleltest // Changes the process-wide locale.
func TestTextSourceUsesCatalogReverseLookup(t *testing.T) {
	restore := SetLanguage("ru", "")
	t.Cleanup(restore)
	if got := TextSource("Fetching suggestions...\n"); got != "Получение вариантов команд...\n" {
		t.Fatalf("dynamic translation=%q", got)
	}
	const unknown = "dynamic message from a provider"
	if got := TextSource(unknown); got != unknown {
		t.Fatalf("unknown dynamic value=%q, want unchanged", got)
	}
}

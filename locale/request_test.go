package locale

import (
	"os"
	"strings"
	"sync"
	"testing"
)

const (
	localeAutoLanguage        = "auto"
	localeEnglishRequest      = "Show all files"
	localeEnglishPOSIX        = "en_US.UTF-8"
	localeRussianRequest      = "Покажи файлы"
	localeShortEnglishRequest = "Show files"
	localeRussianSentence     = "Покажи файлы. "
)

func TestSetLanguageDetectsRequest(t *testing.T) {
	for _, tc := range []struct {
		name, configured, request, override, system, want string
	}{
		{"Russian short", localeAutoLanguage, localeRussianRequest, "", localeEnglishPOSIX, "ru"},
		{"Russian long", localeAutoLanguage, "Покажи все файлы в текущем каталоге, включая скрытые файлы, и выведи подробную информацию о каждом из них", "", localeEnglishPOSIX, "ru"},
		{"English", localeAutoLanguage, "Find all large files in the current directory", "", localeRussianPOSIX, "en"},
		{"Ukrainian", localeAutoLanguage, "Знайди всі великі файли в поточному каталозі та відсортуй їх за розміром", "", localeRussianPOSIX, "en"},
		{"Bulgarian", localeAutoLanguage, "Покажи всички файлове в текущата директория и ги сортирай по размер", "", localeRussianPOSIX, "en"},
		{"Serbian", localeAutoLanguage, "Прикажи све датотеке у тренутном директоријуму и сортирај их по величини", "", localeRussianPOSIX, "en"},
		{"French", localeAutoLanguage, "Afficher tous les fichiers du répertoire courant", "", localeRussianPOSIX, "en"},
		{"Japanese", localeAutoLanguage, "現在のディレクトリにあるファイルを表示してください", "", localeRussianPOSIX, "en"},
		{"unknown ASCII", localeAutoLanguage, "qzxj 12345", "", localeRussianPOSIX, "en"},
		{"punctuation", localeAutoLanguage, "!?! 1234", "", localeRussianPOSIX, "en"},
		{"fixed English", "en", localeRussianRequest, "", localeRussianPOSIX, "en"},
		{"fixed Russian", "ru", localeEnglishRequest, "", localeEnglishPOSIX, "ru"},
		{"normalized config", " RU ", localeEnglishRequest, "", localeEnglishPOSIX, "ru"},
		{"empty config detects", "", localeRussianRequest, "", localeEnglishPOSIX, "ru"},
		{"environment beats config", "ru", localeRussianRequest, "en", localeRussianPOSIX, "en"},
		{"Russian environment beats config", "en", localeEnglishRequest, " RU ", localeEnglishPOSIX, "ru"},
		{"auto environment detects", localeAutoLanguage, localeRussianRequest, localeAutoLanguage, localeEnglishPOSIX, "ru"},
		{"no request uses system", localeAutoLanguage, "", "", localeRussianPOSIX, "ru"},
		{"auto no request uses system", localeAutoLanguage, "", localeAutoLanguage, localeRussianPOSIX, "ru"},
		{"fixed no request", "en", "", "", localeRussianPOSIX, "en"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(localeOverrideEnv, tc.override)
			t.Setenv(localeAllEnv, tc.system)
			restore := SetLanguage(tc.configured, tc.request)
			t.Cleanup(restore)
			if got := Language(); got != tc.want {
				t.Fatalf("Language() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestUpdateRequestLanguageRetainsOverride(t *testing.T) {
	t.Setenv(localeOverrideEnv, "")
	t.Setenv(localeAllEnv, localeEnglishPOSIX)
	for _, configured := range []string{localeAutoLanguage, "en", "ru"} {
		t.Run(configured, func(t *testing.T) {
			restore := SetLanguage(configured, localeShortEnglishRequest)
			t.Cleanup(restore)
			for _, request := range []string{localeRussianRequest, localeShortEnglishRequest, ""} {
				UpdateRequestLanguage(request)
				want := configured
				if configured == localeAutoLanguage {
					want = "en"
					if request == localeRussianRequest {
						want = "ru"
					}
				}
				if got := Language(); got != want {
					t.Fatalf("after update %q: language=%q, want %q", request, got, want)
				}
			}
		})
	}
}

func TestSetLanguageRestoresPreviousSelection(t *testing.T) {
	t.Setenv(localeOverrideEnv, "")
	t.Setenv(localeAllEnv, localeRussianPOSIX)
	before := Language()
	restoreOuter := SetLanguage("en", "")
	restored := false
	t.Cleanup(func() {
		if !restored {
			restoreOuter()
		}
	})
	restoreInner := SetLanguage(localeAutoLanguage, localeRussianRequest)
	if got := Language(); got != "ru" {
		t.Fatalf("inner language=%q", got)
	}
	UpdateRequestLanguage(localeShortEnglishRequest)
	restoreInner()
	if got := Language(); got != "en" {
		t.Fatalf("restored outer language=%q", got)
	}
	restoreOuter()
	restored = true
	if got := Language(); got != before {
		t.Fatalf("restored environment language=%q, want %q", got, before)
	}
	if got := strings.TrimSpace(os.Getenv(localeOverrideEnv)); got != "" {
		t.Fatalf("selection mutated ZEV_LANG: %q", got)
	}
}

func TestRequestLanguageDetectionIsBounded(t *testing.T) {
	t.Setenv(localeOverrideEnv, "")
	for _, tc := range []struct {
		prefix, suffix, want string
	}{
		{strings.Repeat("あ", 4096), strings.Repeat(localeRussianSentence, 10000), "en"},
		{strings.Repeat("あ", 1000) + strings.Repeat(localeRussianSentence, 200), "", "ru"},
		{strings.Repeat(localeRussianSentence, 4096), strings.Repeat("Show files. ", 10000), "ru"},
	} {
		restore := SetLanguage(localeAutoLanguage, tc.prefix+tc.suffix)
		if got := Language(); got != tc.want {
			t.Errorf("bounded detection=%q, want %q", got, tc.want)
		}
		restore()
	}
}

func TestRequestLanguageConcurrentAccess(t *testing.T) {
	t.Setenv(localeOverrideEnv, "")
	restore := SetLanguage(localeAutoLanguage, localeShortEnglishRequest)
	t.Cleanup(restore)
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 20 {
				UpdateRequestLanguage(localeRussianRequest)
				_ = Language()
				_ = Text(KeyQueryFetchingSuggestions)
				UpdateRequestLanguage(localeShortEnglishRequest)
			}
		})
	}
	wg.Wait()
}

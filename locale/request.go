package locale

import (
	"strings"
	"sync/atomic"

	"github.com/abadojack/whatlanggo"
)

type languageSelection struct {
	configured string
	language   string
}

var selectedLanguage atomic.Pointer[languageSelection]

// SetLanguage selects the process-wide UI language for one CLI command and returns
// a function restoring the previous selection. configured accepts auto (or empty),
// ru, or en; other values behave as auto and must be rejected by config validation.
// Auto detects a nonempty request locally, mapping Russian to ru and all other
// results to en. Detection inspects at most the first 4096 runes and is statistical:
// short or similar-language texts can be misclassified. An empty request uses the
// system locale. Explicit ru/en ZEV_LANG takes precedence. No environment variables
// are changed.
// Commands with independent locales must not run concurrently in the same process;
// nested selections must be restored in reverse order, once per selection.
func SetLanguage(configured, request string) func() {
	previous := selectedLanguage.Swap(&languageSelection{
		configured: strings.ToLower(strings.TrimSpace(configured)),
		language:   detectRequestLanguage(request),
	})
	return func() { selectedLanguage.Store(previous) }
}

// UpdateRequestLanguage replaces the request language while retaining the current
// configured override. Without SetLanguage it starts an auto selection. It shares
// the same process scope and concurrency limitation as SetLanguage.
func UpdateRequestLanguage(request string) {
	language := detectRequestLanguage(request)
	for {
		previous := selectedLanguage.Load()
		next := &languageSelection{language: language}
		if previous != nil {
			next.configured = previous.configured
		}
		if selectedLanguage.CompareAndSwap(previous, next) {
			return
		}
	}
}

func detectRequestLanguage(request string) string {
	if request == "" {
		return ""
	}
	// Bound classifier work without allocating a rune slice for the full request.
	runes := 0
	for index := range request {
		if runes == 4096 {
			request = request[:index]
			break
		}
		runes++
	}
	if whatlanggo.DetectLang(request) == whatlanggo.Rus {
		return "ru"
	}
	return "en"
}

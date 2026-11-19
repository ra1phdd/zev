package security

import (
	"sort"
	"strings"
)

// SanitizedEnvironment returns a deterministic, non-secret environment for
// child processes. Explicitly supplied variables remain the caller's contract;
// this function is used for the default process boundary so provider keys,
// credential handles, and unrelated application secrets are not inherited.
func SanitizedEnvironment(environment []string) []string {
	values := make(map[string]string)
	for _, entry := range environment {
		key, value, ok := strings.Cut(entry, "=")
		if !ok || !allowedEnvironmentKey(key) {
			continue
		}
		values[key] = value
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]string, 0, len(keys))
	for _, key := range keys {
		result = append(result, key+"="+values[key])
	}
	return result
}

func allowedEnvironmentKey(key string) bool {
	switch key {
	case "PATH", "HOME", "USER", "LOGNAME", "SHELL", "TERM", "COLORTERM", "LANG", "TMPDIR", "TMP", "TEMP", "PWD":
		return true
	default:
		return strings.HasPrefix(key, "LC_")
	}
}

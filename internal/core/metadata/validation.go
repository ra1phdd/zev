package metadata

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/ra1phdd/zev/internal/core/domain"
)

const (
	maxTemplateBytes     = 16 << 10
	maxDescriptionBytes  = 4096
	maxArgumentNameBytes = 64
	maxArgumentDescBytes = 1024
	maxArguments         = 32
	maxScriptPathBytes   = 512
)

var identifierPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,63}$`)

var reservedNames = map[string]bool{
	"alias": true, fieldScript: true, "query": true, "history": true,
	"setup": true, "doctor": true, "version": true, "continue": true,
	"help": true,
}

// ValidateName validates a user-created alias or script name.
func ValidateName(name string) error {
	if !utf8.ValidString(name) {
		return fmt.Errorf("invalid name: %q is not valid UTF-8", name)
	}
	if len([]byte(name)) == 0 || len([]byte(name)) > maxArgumentNameBytes || !identifierPattern.MatchString(name) {
		return fmt.Errorf("invalid name %q: expected [a-zA-Z0-9][a-zA-Z0-9._-]{0,63}", name)
	}
	if reservedNames[name] {
		return fmt.Errorf("name %q is reserved", name)
	}
	return nil
}

// ValidateArguments validates shared ordered positional metadata.
func ValidateArguments(arguments []domain.Argument) error {
	if len(arguments) > maxArguments {
		return fmt.Errorf("too many arguments: %d, maximum is %d", len(arguments), maxArguments)
	}
	seen := make(map[string]struct{}, len(arguments))
	optionalSeen := false
	for i, argument := range arguments {
		if !utf8.ValidString(argument.Name) || len([]byte(argument.Name)) == 0 || len([]byte(argument.Name)) > maxArgumentNameBytes || !identifierPattern.MatchString(argument.Name) {
			return fmt.Errorf("invalid argument %d name %q", i+1, argument.Name)
		}
		if _, exists := seen[argument.Name]; exists {
			return fmt.Errorf("duplicate argument name %q", argument.Name)
		}
		seen[argument.Name] = struct{}{}
		if err := validateText("argument description", argument.Description, maxArgumentDescBytes); err != nil {
			return err
		}
		if !argument.Required {
			optionalSeen = true
		} else if optionalSeen {
			return fmt.Errorf("required argument %q follows an optional argument", argument.Name)
		}
	}
	return nil
}

// ValidateAlias validates the complete canonical alias model, including its
// closed template grammar and effective positional argument contract.
func ValidateAlias(alias domain.Alias) error {
	if err := ValidateName(alias.Name); err != nil {
		return err
	}
	if err := validateText(fieldDescription, alias.Description, maxDescriptionBytes); err != nil {
		return err
	}
	_, err := EffectiveArguments(alias)
	return err
}

// ValidateScript validates a canonical script model with its immutable record ID.
func ValidateScript(script domain.Script) error {
	if err := ValidateScriptFields(script); err != nil {
		return err
	}
	if script.RecordID == "" {
		return fmt.Errorf("recordID for %q is required", script.Name)
	}
	if _, err := uuid.Parse(script.RecordID); err != nil {
		return fmt.Errorf("invalid recordID for %q: %w", script.Name, err)
	}
	return nil
}

// ValidateScriptFields validates user-supplied fields independently of the repository-assigned ID.
func ValidateScriptFields(script domain.Script) error {
	if err := ValidateName(script.Name); err != nil {
		return err
	}
	if err := ValidateScriptPath(script.ScriptFile); err != nil {
		return err
	}
	if err := validateText(fieldDescription, script.Description, maxDescriptionBytes); err != nil {
		return err
	}
	return ValidateArguments(script.Arguments)
}

// ValidateScriptPath checks a storage-relative path without touching the filesystem.
func ValidateScriptPath(path string) error {
	if !utf8.ValidString(path) || path == "" || len([]byte(path)) > maxScriptPathBytes {
		return fmt.Errorf("invalid script path %q", path)
	}
	for _, r := range path {
		if unicode.IsControl(r) || r == '\\' {
			return fmt.Errorf("invalid script path %q", path)
		}
	}
	if filepath.IsAbs(path) || filepath.Clean(path) != path || path == "." || path == ".." ||
		strings.HasPrefix(path, "../") || strings.HasPrefix(path, "//") ||
		(len(path) >= 2 && ((path[0] >= 'a' && path[0] <= 'z') || (path[0] >= 'A' && path[0] <= 'Z')) && path[1] == ':') {
		return fmt.Errorf("script path %q must be relative and confined", path)
	}
	return nil
}

func validateText(field, value string, maxBytes int) error {
	if !utf8.ValidString(value) {
		return fmt.Errorf("%s is not valid UTF-8", field)
	}
	if len([]byte(value)) > maxBytes {
		return fmt.Errorf("%s is too large: maximum is %d bytes", field, maxBytes)
	}
	for _, r := range value {
		if unicode.IsControl(r) || (r >= 0x7f && r <= 0x9f) {
			return fmt.Errorf("%s contains a control character", field)
		}
	}
	return nil
}

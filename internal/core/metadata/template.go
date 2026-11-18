package metadata

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/ra1phdd/zev/internal/core/domain"
)

type templateWord struct {
	literal  string
	position int
	all      bool
}

type parsedTemplate struct {
	words       []templateWord
	maxPosition int
	hasArgs     bool
}

var restrictedShellWords = map[string]bool{
	"!": true, "case": true, "coproc": true, "do": true, "done": true, "elif": true,
	"else": true, "esac": true, "fi": true, "for": true, "function": true, "if": true,
	"in": true, "select": true, "then": true, "time": true, "until": true,
	"while": true, "[[": true, "]]": true, "eval": true,
}

// ValidateAliasTemplate validates the closed argv-safe template dialect.
func ValidateAliasTemplate(template string, arguments []domain.Argument) error {
	parsed, err := parseTemplate(template)
	if err != nil {
		return err
	}
	_, err = effectiveArguments(parsed, arguments)
	return err
}

// EffectiveArguments returns the positional contract implied by a template and
// its metadata. Missing metadata entries are required by default.
func EffectiveArguments(alias domain.Alias) ([]domain.Argument, error) {
	parsed, err := parseTemplate(alias.Template)
	if err != nil {
		return nil, err
	}
	return effectiveArguments(parsed, alias.Arguments)
}

func parseTemplate(template string) (parsedTemplate, error) {
	var parsed parsedTemplate
	if err := validateTemplateText(template); err != nil {
		return parsed, err
	}
	wordPosition := 0
	for word := range strings.SplitSeq(template, " ") {
		if word == "" {
			continue
		}
		wordPosition++
		parsedWord, err := parseTemplateWord(word, wordPosition)
		if err != nil {
			return parsed, err
		}
		parsed.words = append(parsed.words, parsedWord)
		if parsedWord.all {
			parsed.hasArgs = true
		} else if parsedWord.position > parsed.maxPosition {
			parsed.maxPosition = parsedWord.position
		}
	}
	if len(parsed.words) == 0 {
		return parsed, errors.New("template cannot be empty")
	}
	for position := 1; position <= parsed.maxPosition; position++ {
		if !hasNumberedPosition(parsed.words, position) {
			return parsed, fmt.Errorf("numbered placeholders must be contiguous: missing arg%d", position)
		}
	}
	if err := rejectShellReevaluation(parsed.words); err != nil {
		return parsed, err
	}
	return parsed, nil
}

var shellInterpreters = map[string]bool{
	"ash": true, "bash": true, "dash": true, "ksh": true, "sh": true, "zsh": true,
}

func rejectShellReevaluation(words []templateWord) error {
	for i, word := range words {
		if !shellInterpreters[word.literal] {
			continue
		}
		for _, following := range words[i+1:] {
			if following.literal == "-c" {
				return errors.New("template cannot invoke a shell for re-evaluation")
			}
		}
	}
	return nil
}

func validateTemplateText(template string) error {
	if !utf8.ValidString(template) {
		return errors.New("template is not valid UTF-8")
	}
	if len([]byte(template)) > maxTemplateBytes {
		return fmt.Errorf("template is too large: maximum is %d bytes", maxTemplateBytes)
	}
	if strings.Trim(template, " ") == "" {
		return errors.New("template cannot be empty")
	}
	for _, r := range template {
		if r == '\n' || r == '\r' || r == '	' || (unicode.IsSpace(r) && r != ' ') {
			return errors.New("template may contain only spaces between words")
		}
		if unicode.IsControl(r) || (r >= 0x7f && r <= 0x9f) {
			return errors.New("template contains a control character")
		}
		if strings.ContainsRune("|&;<>()$`'\"\\#", r) {
			return fmt.Errorf("template contains unsupported shell syntax %q", r)
		}
	}
	return nil
}

func parseTemplateWord(word string, position int) (templateWord, error) {
	if strings.ContainsAny(word, "*?[]") {
		return templateWord{}, fmt.Errorf("template word %d uses shell glob syntax", position)
	}
	if strings.HasPrefix(word, "~") {
		return templateWord{}, fmt.Errorf("template word %d uses tilde expansion", position)
	}
	if isAssignmentWord(word) {
		return templateWord{}, fmt.Errorf("template word %d is a shell assignment", position)
	}
	if restrictedShellWords[word] {
		return templateWord{}, fmt.Errorf("template word %d is a restricted shell word", position)
	}
	placeholderPosition, all, ok := ParsePlaceholder(word)
	if ok {
		if !all && placeholderPosition > maxArguments {
			return templateWord{}, fmt.Errorf("template word %d placeholder exceeds the maximum argument position", position)
		}
		return templateWord{position: placeholderPosition, all: all}, nil
	}
	if strings.ContainsAny(word, "{}") {
		return templateWord{}, fmt.Errorf("template word %d uses a non-standalone placeholder", position)
	}
	return templateWord{literal: word}, nil
}

func hasNumberedPosition(words []templateWord, position int) bool {
	for _, word := range words {
		if word.position == position && !word.all {
			return true
		}
	}
	return false
}

func isAssignmentWord(word string) bool {
	if word == "" || !isAssignmentStart(word[0]) {
		return false
	}
	for i := 1; i < len(word); i++ {
		if word[i] == '=' {
			return true
		}
		if !isAssignmentPart(word[i]) {
			return false
		}
	}
	return false
}

func isAssignmentStart(value byte) bool {
	return value == '_' || (value >= 'a' && value <= 'z') || (value >= 'A' && value <= 'Z')
}

func isAssignmentPart(value byte) bool {
	return isAssignmentStart(value) || (value >= '0' && value <= '9')
}

func effectiveArguments(parsed parsedTemplate, supplied []domain.Argument) ([]domain.Argument, error) {
	if parsed.maxPosition == 0 {
		if len(supplied) != 0 {
			return nil, errors.New("arguments metadata requires numbered placeholders")
		}
		return nil, nil
	}
	if len(supplied) > parsed.maxPosition {
		return nil, errors.New("arguments metadata exceeds the highest numbered placeholder")
	}
	result := make([]domain.Argument, parsed.maxPosition)
	copy(result, supplied)
	for i := len(supplied); i < parsed.maxPosition; i++ {
		result[i] = domain.Argument{Name: "arg" + strconv.Itoa(i+1), Required: true}
	}
	if err := ValidateArguments(result); err != nil {
		return nil, err
	}
	return result, nil
}

// ParsePlaceholder recognizes only the two standalone placeholders in the
// argv-safe dialect. The returned position is one-based for numbered values.
func ParsePlaceholder(word string) (position int, all bool, ok bool) {
	if word == "{{.args}}" {
		return 0, true, true
	}
	if !strings.HasPrefix(word, "{{.arg") || !strings.HasSuffix(word, "}}") {
		return 0, false, false
	}
	digits := strings.TrimSuffix(strings.TrimPrefix(word, "{{.arg"), "}}")
	if digits == "" || (len(digits) > 1 && digits[0] == '0') {
		return 0, false, false
	}
	for _, r := range digits {
		if r < '0' || r > '9' {
			return 0, false, false
		}
	}
	position, err := strconv.Atoi(digits)
	if err != nil || position < 1 {
		return 0, false, false
	}
	return position, false, true
}

// RenderAlias validates an alias and renders its exact argv values as
// POSIX-single-quoted shell words. It rejects missing required and extra
// positional values before returning any command text.
func RenderAlias(alias domain.Alias, argv []string) (string, error) {
	if err := ValidateAlias(alias); err != nil {
		return "", err
	}
	parsed, err := parseTemplate(alias.Template)
	if err != nil {
		return "", err
	}
	effective, err := effectiveArguments(parsed, alias.Arguments)
	if err != nil {
		return "", err
	}
	for i, argument := range effective {
		if i >= len(argv) {
			if argument.Required {
				return "", fmt.Errorf("alias %q requires argument %d: %s — %s", alias.Name, i+1, argument.Name, argument.Description)
			}
			continue
		}
	}
	if !parsed.hasArgs && len(argv) > parsed.maxPosition {
		return "", fmt.Errorf("alias %q accepts at most %d argument(s)", alias.Name, parsed.maxPosition)
	}

	words := make([]string, 0, len(parsed.words)+len(argv))
	for _, word := range parsed.words {
		switch {
		case word.all:
			for _, value := range argv {
				words = append(words, quoteShellWord(value))
			}
		case word.position > 0:
			if word.position <= len(argv) {
				words = append(words, quoteShellWord(argv[word.position-1]))
			}
		default:
			words = append(words, word.literal)
		}
	}
	return strings.Join(words, " "), nil
}

func quoteShellWord(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

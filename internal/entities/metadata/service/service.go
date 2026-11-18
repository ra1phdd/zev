// Package service defines the consumer-side metadata generation contract and
// validation shared by provider adapters.
package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"unicode"
	"unicode/utf8"

	"github.com/ra1phdd/zev/internal/core/domain"
	"github.com/ra1phdd/zev/internal/core/metadata"
)

const (
	// MaxGenerationBytes is the maximum UTF-8 size of a generation request or response.
	MaxGenerationBytes = 65536
	// MaxArguments is the maximum number of generated positional arguments.
	MaxArguments        = 32
	maxDescriptionBytes = 4096
	maxEntityNameBytes  = 64
	entityKindAlias     = "alias"
	entityKindScript    = "script"
	maxJSONDepth        = 32
)

var (
	// ErrInvalidInput indicates that a generation input violates the metadata contract.
	ErrInvalidInput = errors.New("invalid metadata generation input")
	// ErrInvalidResponse indicates that a provider response is not the exact metadata schema.
	ErrInvalidResponse = errors.New("invalid metadata generation response")
	// ErrUnavailable indicates a provider transport or protocol failure.
	ErrUnavailable = errors.New("metadata generation unavailable")
)

// MetadataGenerator generates one metadata field without query history or runtime context.
type MetadataGenerator interface {
	GenerateDescription(ctx context.Context, input DescriptionInput) (string, error)
	GenerateArguments(ctx context.Context, input ArgumentsInput) ([]domain.Argument, error)
}

// DescriptionInput contains only the bounded entity source and already supplied arguments.
type DescriptionInput struct {
	EntityName string
	EntityKind string
	Template   string
	Body       string
	Arguments  []domain.Argument
}

// ArgumentsInput contains only the bounded entity source and an already supplied description.
type ArgumentsInput struct {
	EntityName  string
	EntityKind  string
	Template    string
	Body        string
	Description string
}

// ValidateDescriptionInput validates the source and supplied metadata for description generation.
func ValidateDescriptionInput(input DescriptionInput) error {
	if err := validateSource(input.EntityName, input.EntityKind, input.Template, input.Body); err != nil {
		return err
	}
	if err := metadata.ValidateArguments(input.Arguments); err != nil {
		return fmt.Errorf("%w: supplied arguments", ErrInvalidInput)
	}
	if input.EntityKind == entityKindAlias {
		if err := metadata.ValidateAliasTemplate(input.Template, input.Arguments); err != nil {
			return fmt.Errorf("%w: supplied arguments", ErrInvalidInput)
		}
	}
	return nil
}

// ValidateArgumentsInput validates the source and supplied metadata for argument generation.
func ValidateArgumentsInput(input ArgumentsInput) error {
	if err := validateSource(input.EntityName, input.EntityKind, input.Template, input.Body); err != nil {
		return err
	}
	if err := validateText(input.Description, maxDescriptionBytes, true); err != nil {
		return fmt.Errorf("%w: supplied description", ErrInvalidInput)
	}
	return nil
}

func validateSource(name, kind, template, body string) error {
	if err := metadata.ValidateName(name); err != nil || len([]byte(name)) > maxEntityNameBytes {
		return fmt.Errorf("%w: entity name", ErrInvalidInput)
	}
	if kind != entityKindAlias && kind != entityKindScript {
		return fmt.Errorf("%w: entity kind", ErrInvalidInput)
	}
	if kind == entityKindAlias {
		if body != "" {
			return fmt.Errorf("%w: alias body is not allowed", ErrInvalidInput)
		}
		if err := metadata.ValidateAliasTemplate(template, nil); err != nil {
			return fmt.Errorf("%w: alias template", ErrInvalidInput)
		}
		return nil
	}
	if template != "" {
		return fmt.Errorf("%w: script template is not allowed", ErrInvalidInput)
	}
	if err := validateText(body, MaxGenerationBytes, false); err != nil || body == "" {
		return fmt.Errorf("%w: script body", ErrInvalidInput)
	}
	return nil
}

func validateText(value string, maxBytes int, rejectNewlines bool) error {
	if !utf8.ValidString(value) || len([]byte(value)) > maxBytes {
		return ErrInvalidInput
	}
	for _, r := range value {
		if r == 0x1b || (r >= 0x7f && r <= 0x9f) || unicode.IsControl(r) && (rejectNewlines || r != '\n' && r != '\r' && r != '\t') {
			return ErrInvalidInput
		}
	}
	return nil
}

// DescriptionPrompt builds the dedicated, context-free prompt for description generation.
func DescriptionPrompt(input DescriptionInput) (string, error) {
	if err := ValidateDescriptionInput(input); err != nil {
		return "", err
	}
	payload := descriptionPayload{EntityName: input.EntityName, EntityKind: input.EntityKind, Template: input.Template, Body: input.Body, Arguments: encodeArguments(input.Arguments)}
	return prompt("description", payload)
}

// ArgumentsPrompt builds the dedicated, context-free prompt for argument generation.
func ArgumentsPrompt(input ArgumentsInput) (string, error) {
	if err := ValidateArgumentsInput(input); err != nil {
		return "", err
	}
	payload := argumentsPayload(input)
	return prompt("arguments", payload)
}

type descriptionPayload struct {
	EntityName string           `json:"entity_name"` //nolint:tagliatelle // Metadata prompts use a stable snake_case wire shape.
	EntityKind string           `json:"entity_kind"` //nolint:tagliatelle // Metadata prompts use a stable snake_case wire shape.
	Template   string           `json:"template,omitempty"`
	Body       string           `json:"body,omitempty"`
	Arguments  []promptArgument `json:"arguments"`
}

type promptArgument struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
}

func encodeArguments(arguments []domain.Argument) []promptArgument {
	result := make([]promptArgument, len(arguments))
	for i, argument := range arguments {
		result[i] = promptArgument{Name: argument.Name, Description: argument.Description, Required: argument.Required}
	}
	return result
}

type argumentsPayload struct {
	EntityName  string `json:"entity_name"` //nolint:tagliatelle // Metadata prompts use a stable snake_case wire shape.
	EntityKind  string `json:"entity_kind"` //nolint:tagliatelle // Metadata prompts use a stable snake_case wire shape.
	Template    string `json:"template,omitempty"`
	Body        string `json:"body,omitempty"`
	Description string `json:"description"`
}

func prompt(field string, payload any) (string, error) {
	data, err := json.Marshal(payload)
	if err != nil || len(data) > MaxGenerationBytes {
		return "", fmt.Errorf("%w: generation request is too large", ErrInvalidInput)
	}
	instruction := "Return exactly one JSON object with no unknown fields, nulls, prose, or trailing data."
	if field == "description" {
		instruction += " The object must be {\"description\": string} with no more than 4096 UTF-8 bytes."
	} else {
		instruction += " The object must be {\"arguments\": array} with no more than 32 items; each item has name string, optional description string, and optional required boolean."
	}
	return instruction + "\nfield=" + field + "\ninput=" + string(data), nil
}

// DecodeDescriptionResponse strictly decodes one generated description object.
func DecodeDescriptionResponse(body []byte) (string, error) {
	object, err := decodeObject(body, "description")
	if err != nil {
		return "", err
	}
	raw, ok := object["description"]
	if !ok || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return "", ErrInvalidResponse
	}
	var description string
	if err := json.Unmarshal(raw, &description); err != nil || !utf8.ValidString(description) || validateText(description, maxDescriptionBytes, true) != nil {
		return "", ErrInvalidResponse
	}
	return description, nil
}

// DecodeArgumentsResponse strictly decodes one generated arguments object.
func DecodeArgumentsResponse(body []byte) ([]domain.Argument, error) {
	object, err := decodeObject(body, "arguments")
	if err != nil {
		return nil, err
	}
	raw, ok := object["arguments"]
	if !ok || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, ErrInvalidResponse
	}
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil || len(items) > MaxArguments {
		return nil, ErrInvalidResponse
	}
	result := make([]domain.Argument, len(items))
	for i, item := range items {
		argument, err := decodeArgument(item)
		if err != nil {
			return nil, err
		}
		result[i] = argument
	}
	if err := metadata.ValidateArguments(result); err != nil {
		return nil, ErrInvalidResponse
	}
	return result, nil
}

func decodeArgument(body []byte) (domain.Argument, error) {
	object, err := decodeRawObject(body)
	if err != nil || len(object) == 0 {
		return domain.Argument{}, ErrInvalidResponse
	}
	for field := range object {
		if field != "name" && field != "description" && field != "required" {
			return domain.Argument{}, ErrInvalidResponse
		}
	}
	argument := domain.Argument{Required: true}
	if raw, ok := object["name"]; !ok || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) || json.Unmarshal(raw, &argument.Name) != nil {
		return domain.Argument{}, ErrInvalidResponse
	}
	if raw, ok := object["description"]; ok {
		if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) || json.Unmarshal(raw, &argument.Description) != nil {
			return domain.Argument{}, ErrInvalidResponse
		}
	}
	if raw, ok := object["required"]; ok {
		if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) || json.Unmarshal(raw, &argument.Required) != nil {
			return domain.Argument{}, ErrInvalidResponse
		}
	}
	return argument, nil
}

// ValidateResponseDocument validates the shared JSON envelope constraints before
// an adapter decodes provider-specific fields. Provider extension fields remain
// allowed; generated metadata fields are separately checked by the field decoders.
func ValidateResponseDocument(body []byte) error {
	if len(body) == 0 || len(body) > MaxGenerationBytes || !utf8.Valid(body) || !validJSONDocument(body) {
		return ErrInvalidResponse
	}
	return nil
}

func decodeObject(body []byte, requiredField string) (map[string]json.RawMessage, error) {
	if err := ValidateResponseDocument(body); err != nil {
		return nil, err
	}
	object, err := decodeRawObject(body)
	if err != nil || len(object) != 1 {
		return nil, ErrInvalidResponse
	}
	if _, ok := object[requiredField]; !ok {
		return nil, ErrInvalidResponse
	}
	return object, nil
}

func decodeRawObject(body []byte) (map[string]json.RawMessage, error) {
	var object map[string]json.RawMessage
	decoder := json.NewDecoder(bytes.NewReader(body))
	if err := decoder.Decode(&object); err != nil || object == nil {
		return nil, ErrInvalidResponse
	}
	return object, nil
}

func validJSONDocument(body []byte) bool {
	if !validUnicodeEscapes(body) {
		return false
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	if !scanJSONValue(decoder, 0) {
		return false
	}
	_, err := decoder.Token()
	return errors.Is(err, io.EOF)
}

func validUnicodeEscapes(body []byte) bool {
	inString := false
	for i := 0; i < len(body); i++ {
		if body[i] == '"' {
			inString = !inString
			continue
		}
		if inString && body[i] == '\\' {
			end, ok := validEscape(body, i)
			if !ok {
				return false
			}
			i = end
			continue
		}
	}
	return !inString
}

func validEscape(body []byte, index int) (end int, ok bool) {
	if index+1 >= len(body) {
		return index, false
	}
	if body[index+1] != 'u' {
		return index + 1, true
	}
	if index+5 >= len(body) {
		return index, false
	}
	value, ok := hexQuad(body[index+2 : index+6])
	if !ok {
		return index, false
	}
	if value >= 0xdc00 && value <= 0xdfff {
		return index, false
	}
	if value < 0xd800 || value > 0xdbff {
		return index + 5, true
	}
	if index+11 >= len(body) || body[index+6] != '\\' || body[index+7] != 'u' {
		return index, false
	}
	low, ok := hexQuad(body[index+8 : index+12])
	if !ok || low < 0xdc00 || low > 0xdfff {
		return index, false
	}
	return index + 11, true
}

func hexQuad(value []byte) (int, bool) {
	if len(value) != 4 {
		return 0, false
	}
	result := 0
	for _, digit := range value {
		part := hexValue(digit)
		if part < 0 {
			return 0, false
		}
		result = result<<4 | part
	}
	return result, true
}

func hexValue(value byte) int {
	switch {
	case value >= '0' && value <= '9':
		return int(value - '0')
	case value >= 'a' && value <= 'f':
		return int(value-'a') + 10
	case value >= 'A' && value <= 'F':
		return int(value-'A') + 10
	default:
		return -1
	}
}

func scanJSONValue(decoder *json.Decoder, depth int) bool {
	if depth > maxJSONDepth {
		return false
	}
	token, err := decoder.Token()
	if err != nil {
		return false
	}
	value, ok := token.(json.Delim)
	if !ok {
		return true
	}
	if value == '{' {
		return scanJSONObject(decoder, depth+1)
	}
	if value == '[' {
		return scanJSONArray(decoder, depth+1)
	}
	return false
}

func scanJSONObject(decoder *json.Decoder, depth int) bool {
	seen := make(map[string]struct{})
	for decoder.More() {
		key, ok := mustStringToken(decoder.Token())
		if !ok {
			return false
		}
		if _, exists := seen[key]; exists || !scanJSONValue(decoder, depth) {
			return false
		}
		seen[key] = struct{}{}
	}
	end, err := decoder.Token()
	return err == nil && end == json.Delim('}')
}

func scanJSONArray(decoder *json.Decoder, depth int) bool {
	for decoder.More() {
		if !scanJSONValue(decoder, depth) {
			return false
		}
	}
	end, err := decoder.Token()
	return err == nil && end == json.Delim(']')
}

func mustStringToken(token json.Token, err error) (string, bool) {
	if err != nil {
		return "", false
	}
	value, ok := token.(string)
	return value, ok
}

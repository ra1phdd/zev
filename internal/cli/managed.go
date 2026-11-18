package cli

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/ra1phdd/zev/internal/core/domain"
	"github.com/ra1phdd/zev/internal/core/filestore"
	"github.com/ra1phdd/zev/internal/core/locale"
	"github.com/ra1phdd/zev/internal/core/metadata"
	"github.com/spf13/cobra"
)

type managedFields struct {
	description                            *string
	arguments                              *[]domain.Argument
	template                               *string
	json, nonInteractive, noMetadata       bool
	generateDescription, generateArguments bool
	bodyFile                               string
	bodyStdin, editBody                    bool
}

func addManagedFlags(cmd *cobra.Command, kind, operation string) {
	cmd.SetFlagErrorFunc(func(_ *cobra.Command, err error) error { return machineError(exitArguments, err) })
	f := cmd.Flags()
	f.Bool("json", false, locale.Text(locale.KeyManagedJSONFlag))
	f.Bool("non-interactive", false, locale.Text(locale.KeyManagedNonInteractiveFlag))
	if operation == managedOpRemove {
		f.Bool("yes", false, locale.Text(locale.KeyManagedYesFlag))
		return
	}
	if operation == managedOpList {
		return
	}
	f.String("description", "", locale.Text(locale.KeyManagedDescriptionFlag))
	f.StringArray("argument", nil, locale.Text(locale.KeyManagedArgumentFlag))
	f.String("arguments-json", "", locale.Text(locale.KeyManagedArgumentsJSONFlag))
	f.Bool("clear-description", false, locale.Text(locale.KeyManagedClearDescriptionFlag))
	f.Bool("clear-arguments", false, locale.Text(locale.KeyManagedClearArgumentsFlag))
	f.Bool("generate-description", false, locale.Text(locale.KeyManagedGenerateDescriptionFlag))
	f.Bool("generate-arguments", false, locale.Text(locale.KeyManagedGenerateArgumentsFlag))
	if kind == managedKindAlias {
		f.String("template", "", locale.Text(locale.KeyManagedTemplateFlag))
		f.Bool("no-metadata", false, locale.Text(locale.KeyManagedNoMetadataFlag))
	} else {
		f.String("body-file", "", locale.Text(locale.KeyManagedBodyFileFlag))
		f.Bool("body-stdin", false, locale.Text(locale.KeyManagedBodyStdinFlag))
		f.Bool("edit-body", false, locale.Text(locale.KeyManagedEditBodyFlag))
	}
}

func usageError(key locale.Key, args ...any) error {
	return machineError(exitArguments, fmt.Errorf(locale.Text(key), args...))
}

//nolint:gocognit // The field conflict matrix is intentionally checked in one place.
func readManagedFields(cmd *cobra.Command, positionals []string, kind, operation string) (managedFields, error) {
	var p managedFields
	f := cmd.Flags()
	str := func(name string) string {
		flag := f.Lookup(name)
		if flag == nil {
			return ""
		}
		return flag.Value.String()
	}
	boolean := func(name string) bool { return str(name) == "true" }
	p.json = boolean("json")
	p.nonInteractive = boolean("non-interactive") || p.json
	p.noMetadata = boolean("no-metadata")
	p.generateDescription = boolean("generate-description")
	p.generateArguments = boolean("generate-arguments")
	p.bodyFile = str("body-file")
	p.bodyStdin = boolean("body-stdin")
	p.editBody = boolean("edit-body")
	p.nonInteractive = p.nonInteractive || p.bodyStdin
	changed := f.Changed
	if p.bodyStdin && changed("body-file") || p.editBody && (p.nonInteractive || changed("body-file") || p.bodyStdin) {
		return p, usageError(locale.KeyManagedConflictingFlags)
	}
	if p.editBody && !managedTerminal(cmd) {
		return p, usageError(locale.KeyManagedTTYRequired)
	}
	if operation == managedOpEdit && changed("no-metadata") {
		return p, usageError(locale.KeyManagedNoMetadataAddOnly)
	}
	if changed("template") {
		v := str("template")
		p.template = &v
	}
	offset := 1
	if kind == managedKindAlias && operation == managedOpAdd {
		if len(positionals) > 1 {
			if p.template != nil {
				return p, usageError(locale.KeyManagedFieldConflict, "template")
			}
			v := positionals[1]
			p.template = &v
		}
		offset = 2
	}
	descPos := len(positionals) > offset
	argPos := len(positionals) > offset+1
	if descPos && changed("description") || argPos && (changed("argument") || changed("arguments-json")) {
		return p, usageError(locale.KeyManagedConflictingFlags)
	}
	if changed("description") {
		v := str("description")
		p.description = &v
	}
	if descPos {
		v := positionals[offset]
		p.description = &v
	}
	descCount := 0
	if p.description != nil {
		descCount++
	}
	if boolean("clear-description") {
		descCount++
		v := ""
		p.description = &v
	}
	if p.generateDescription {
		descCount++
	}
	argCount := 0
	if changed("argument") {
		argCount++
		values, err := f.GetStringArray("argument")
		if err != nil {
			return p, fmt.Errorf("read argument flags: %w", err)
		}
		args := make([]domain.Argument, 0, len(values))
		for _, value := range values {
			name, desc, _ := strings.Cut(value, "=")
			args = append(args, domain.Argument{Name: name, Description: desc, Required: true})
		}
		p.arguments = &args
	}
	if changed("arguments-json") || argPos {
		argCount++
		text := str("arguments-json")
		if argPos {
			text = positionals[offset+1]
		}
		args, err := decodeManagedArguments(text)
		if err != nil {
			return p, machineError(exitArguments, err)
		}
		p.arguments = &args
	}
	if boolean("clear-arguments") {
		argCount++
		args := []domain.Argument{}
		p.arguments = &args
	}
	if p.generateArguments {
		argCount++
	}
	if descCount > 1 || argCount > 1 {
		return p, usageError(locale.KeyManagedConflictingFlags)
	}
	if p.noMetadata && (descCount > 0 || argCount > 0) {
		return p, usageError(locale.KeyManagedConflictingFlags)
	}
	if p.arguments != nil {
		if err := metadata.ValidateArguments(*p.arguments); err != nil {
			return p, machineError(exitArguments, err)
		}
	}
	if p.description != nil {
		if err := metadata.ValidateScriptFields(domain.Script{Name: "validation", ScriptFile: "validation.sh", Description: *p.description}); err != nil {
			return p, machineError(exitArguments, err)
		}
	}
	return p, nil
}

//nolint:gocognit // The bounded streaming parser rejects duplicate keys and wrong types.
func decodeManagedArguments(text string) ([]domain.Argument, error) {
	if len(text) > 65536 || !utf8.ValidString(text) {
		return nil, errors.New(locale.Text(locale.KeyManagedInvalidArgumentsJSON))
	}
	dec := json.NewDecoder(strings.NewReader(text))
	token, err := dec.Token()
	if err != nil || token != json.Delim('[') {
		return nil, errors.New(locale.Text(locale.KeyManagedInvalidArgumentsJSON))
	}
	result := []domain.Argument{}
	for dec.More() {
		token, err = dec.Token()
		if err != nil || token != json.Delim('{') || len(result) >= 32 {
			return nil, errors.New(locale.Text(locale.KeyManagedInvalidArgumentsJSON))
		}
		arg := domain.Argument{Required: true}
		seen := map[string]bool{}
		for dec.More() {
			keyToken, e := dec.Token()
			if e != nil {
				return nil, fmt.Errorf("decode metadata: %w", e)
			}
			key, ok := keyToken.(string)
			if !ok || seen[key] {
				return nil, errors.New(locale.Text(locale.KeyManagedInvalidArgumentsJSON))
			}
			seen[key] = true
			var raw json.RawMessage
			if err := dec.Decode(&raw); err != nil {
				return nil, fmt.Errorf("validate metadata: %w", err)
			}
			if bytes.Equal(raw, []byte("null")) {
				return nil, errors.New(locale.Text(locale.KeyManagedInvalidArgumentsJSON))
			}
			switch key {
			case "name":
				err = json.Unmarshal(raw, &arg.Name)
			case "description":
				err = json.Unmarshal(raw, &arg.Description)
			case "required":
				err = json.Unmarshal(raw, &arg.Required)
			default:
				return nil, errors.New(locale.Text(locale.KeyManagedInvalidArgumentsJSON))
			}
			if err != nil {
				return nil, fmt.Errorf("validate metadata: %w", err)
			}
		}
		if _, err := dec.Token(); err != nil {
			return nil, fmt.Errorf("validate metadata: %w", err)
		}
		result = append(result, arg)
	}
	if _, err := dec.Token(); err != nil {
		return nil, fmt.Errorf("validate metadata: %w", err)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, errors.New(locale.Text(locale.KeyManagedInvalidArgumentsJSON))
	}
	if err := metadata.ValidateArguments(result); err != nil {
		return nil, fmt.Errorf("validate metadata: %w", err)
	}
	return result, nil
}

var managedTerminal = realManagedTTY

func requireManagedInteractive(cmd *cobra.Command, p managedFields) error {
	if p.nonInteractive || !managedTerminal(cmd) {
		return usageError(locale.KeyManagedTTYRequired)
	}
	return nil
}

type managedForm struct {
	cmd    *cobra.Command
	reader *bufio.Reader
}

func newManagedForm(cmd *cobra.Command) *managedForm {
	return &managedForm{cmd: cmd, reader: bufio.NewReader(cmd.InOrStdin())}
}

func (f *managedForm) field(key locale.Key, current string) (string, error) {
	if err := f.cmd.Context().Err(); err != nil {
		return "", fmt.Errorf("read managed form: %w", err)
	}
	if realManagedTTY(f.cmd) {
		return readManagedTTY(f.cmd, locale.Text(key), current)
	}
	if _, err := fmt.Fprintf(f.cmd.ErrOrStderr(), locale.Text(locale.KeyManagedFieldPrompt), locale.Text(key), current); err != nil {
		return "", fmt.Errorf("read form: %w", err)
	}
	text, err := f.reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("%s: %w", locale.Text(locale.KeyManagedCancelled), err)
	}
	text = strings.TrimSuffix(strings.TrimSuffix(text, "\n"), "\r")
	if strings.ContainsAny(text, "\x03\x1b") {
		return "", errors.New(locale.Text(locale.KeyManagedCancelled))
	}
	if text == "" {
		return current, nil
	}
	return text, nil
}

func (f *managedForm) selectName(names []string) (string, error) {
	if len(names) == 0 {
		return "", usageError(locale.KeyManagedNoRecords)
	}
	for i, name := range names {
		if _, err := fmt.Fprintf(f.cmd.ErrOrStderr(), "%d\t%s\n", i+1, name); err != nil {
			return "", fmt.Errorf("read form: %w", err)
		}
	}
	value, err := f.field(locale.KeyManagedSelect, "")
	if err != nil {
		return "", fmt.Errorf("read form: %w", err)
	}
	n, err := strconv.Atoi(value)
	if err != nil || n < 1 || n > len(names) {
		return "", usageError(locale.KeyManagedInvalidSelection)
	}
	return names[n-1], nil
}

func (f *managedForm) confirm(name string) (bool, error) {
	if _, err := fmt.Fprintln(f.cmd.ErrOrStderr(), name); err != nil {
		return false, fmt.Errorf("write removal selection: %w", err)
	}
	value, err := f.field(locale.KeyManagedConfirmRemove, "")
	return value == "y" || value == "yes", err
}

func (f *managedForm) metadata(description string, arguments []domain.Argument) (string, []domain.Argument, error) {
	desc, err := f.field(locale.KeyManagedDescription, description)
	if err != nil {
		return "", nil, err
	}
	encoded, err := json.Marshal(managedArguments(arguments))
	if err != nil {
		return "", nil, fmt.Errorf("encode argument metadata: %w", err)
	}
	text, err := f.field(locale.KeyManagedArgumentsJSON, string(encoded))
	if err != nil {
		return "", nil, err
	}
	args, err := decodeManagedArguments(text)
	return desc, args, err
}

type managedArgument struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
}
type managedRecord struct {
	usage       string
	Name        string            `json:"name"`
	Template    string            `json:"template,omitempty"`
	Script      string            `json:"script,omitempty"`
	RecordID    string            `json:"recordID,omitempty"` //nolint:tagliatelle // Management JSON preserves the canonical immutable recordID spelling.
	Description string            `json:"description"`
	Arguments   []managedArgument `json:"arguments"`
}

func managedArguments(args []domain.Argument) []managedArgument {
	result := make([]managedArgument, len(args))
	for i, a := range args {
		result[i] = managedArgument{a.Name, a.Description, a.Required}
	}
	return result
}

func aliasRecord(a domain.Alias) managedRecord {
	return managedRecord{Name: a.Name, Template: a.Template, Description: a.Description, Arguments: managedArguments(a.Arguments)}
}

func scriptRecord(s domain.Script) managedRecord {
	return managedRecord{Name: s.Name, Script: s.ScriptFile, RecordID: s.RecordID, Description: s.Description, Arguments: managedArguments(s.Arguments)}
}

func writeManagedJSON(cmd *cobra.Command, kind string, item *managedRecord, items []managedRecord) error {
	result := map[string]any{"schemaVersion": 1, "status": "ok", "type": kind}
	if item != nil {
		result["item"] = item
	} else {
		if items == nil {
			items = []managedRecord{}
		}
		result["items"] = items
	}
	if err := json.NewEncoder(cmd.OutOrStdout()).Encode(result); err != nil {
		return fmt.Errorf("write management JSON: %w", err)
	}
	return nil
}

func managedStorageError(err error) error {
	if errors.Is(err, filestore.ErrCommitted) {
		return machineError(6, fmt.Errorf("%s: %w", locale.Text(locale.KeyManagedCommittedUnknown), err))
	}
	return err
}

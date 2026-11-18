package cli

import (
	"encoding/json"
	"fmt"

	"github.com/ra1phdd/zev/internal/core/domain"
	"github.com/ra1phdd/zev/internal/core/locale"
)

func (f *managedForm) creationMetadata(fields managedFields) (string, []domain.Argument, error) {
	if fields.noMetadata {
		return "", nil, nil
	}
	var description string
	var arguments []domain.Argument
	if fields.description != nil {
		description = *fields.description
	} else if !fields.generateDescription {
		value, err := f.field(locale.KeyManagedDescription, "")
		if err != nil {
			return "", nil, err
		}
		description = value
	}
	if fields.arguments != nil {
		arguments = *fields.arguments
	} else if !fields.generateArguments {
		encoded, err := json.Marshal(managedArguments(nil))
		if err != nil {
			return "", nil, fmt.Errorf("encode metadata form: %w", err)
		}
		text, err := f.field(locale.KeyManagedArgumentsJSON, string(encoded))
		if err != nil {
			return "", nil, err
		}
		arguments, err = decodeManagedArguments(text)
		if err != nil {
			return "", nil, err
		}
	}
	return description, arguments, nil
}

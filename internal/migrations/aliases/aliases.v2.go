package aliases

import (
	"context"
	"fmt"
	"strings"

	"github.com/ra1phdd/zev/internal/core/metadata/yamldoc"

	"github.com/ra1phdd/zev/internal/core/domain"
	"github.com/ra1phdd/zev/internal/core/metadata"
	"gopkg.in/yaml.v3"
)

func toV2(_ context.Context, source []byte) ([]byte, error) {
	entries, err := yamldoc.DecodeMapping(source)
	if err != nil {
		return nil, fmt.Errorf("decode legacy aliases: %w", err)
	}
	records := make(map[string]domain.Alias, len(entries))
	for name, node := range entries {
		if node.Kind != yaml.ScalarNode || node.Tag != "!!str" {
			return nil, fmt.Errorf("alias %q must be a string", name)
		}
		tmpl := node.Value
		// A valid placeholder is always a whole word. Check the input before
		// adding implicit arguments so an empty command cannot become a template.
		if err := metadata.ValidateAliasTemplate(tmpl, nil); err != nil {
			return nil, fmt.Errorf("alias %q: %w", name, err)
		}
		if !strings.Contains(tmpl, "{{") {
			tmpl += " {{.args}}"
		}
		records[name] = domain.Alias{Name: name, Template: tmpl}
	}
	data, err := metadata.EncodeAliasesV2(records)
	if err != nil {
		return nil, fmt.Errorf("encode v2 aliases: %w", err)
	}
	return data, nil
}

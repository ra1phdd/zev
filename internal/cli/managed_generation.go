package cli

import (
	"errors"
	"fmt"
	"log/slog"

	"github.com/ra1phdd/zev/internal/core/config"
	"github.com/ra1phdd/zev/internal/core/domain"
	"github.com/ra1phdd/zev/internal/core/httpclient"
	"github.com/ra1phdd/zev/internal/core/locale"
	"github.com/ra1phdd/zev/internal/core/metadata"
	metadataclient "github.com/ra1phdd/zev/internal/entities/metadata/client"
	metadataservice "github.com/ra1phdd/zev/internal/entities/metadata/service"
	scriptrepo "github.com/ra1phdd/zev/internal/entities/script/repository"
	"github.com/spf13/cobra"
)

type managedGeneration struct {
	Alias       *domain.Alias
	Script      *domain.Script
	Source      *scriptrepo.BodySnapshot
	Description bool
	Arguments   bool
	Body        string
	HasBody     bool
	Interactive bool
	Form        *managedForm
}

func managedRangeArgs(minimum, maximum int) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		return machineError(exitArguments, cobra.RangeArgs(minimum, maximum)(cmd, args))
	}
}

func generationInputs(cmd *cobra.Command, request managedGeneration) (description metadataservice.DescriptionInput, arguments metadataservice.ArgumentsInput, err error) {
	if request.Alias != nil {
		a := request.Alias
		description = metadataservice.DescriptionInput{EntityName: a.Name, EntityKind: managedKindAlias, Template: a.Template, Arguments: a.Arguments}
		arguments = metadataservice.ArgumentsInput{EntityName: a.Name, EntityKind: managedKindAlias, Template: a.Template, Description: a.Description}
		return description, arguments, nil
	}
	script := request.Script
	if script == nil {
		return description, arguments, metadataservice.ErrInvalidInput
	}
	body := request.Body
	if !request.HasBody {
		dir, e := config.GetZevDir()
		if e != nil {
			return description, arguments, fmt.Errorf("resolve script generation source: %w", e)
		}
		if request.Source == nil {
			return description, arguments, metadataservice.ErrInvalidInput
		}
		data, e := scriptrepo.New(dir).ReadBody(cmd.Context(), scriptrepo.ReadBodyParams{Snapshot: *request.Source, MaxBytes: metadataservice.MaxGenerationBytes})
		if e != nil {
			return description, arguments, fmt.Errorf("read pinned generation source: %w", e)
		}
		body = string(data)
	}
	description = metadataservice.DescriptionInput{EntityName: script.Name, EntityKind: managedKindScript, Body: body, Arguments: script.Arguments}
	arguments = metadataservice.ArgumentsInput{EntityName: script.Name, EntityKind: managedKindScript, Body: body, Description: script.Description}
	return description, arguments, nil
}

// generateManagedMetadata is the only CRUD network boundary. Explicit flags
// authorize command mode; interactive mode separately discloses the full inputs.
//
//nolint:gocognit // Generation coordinates validation, consent, provider calls and per-field fallback.
func generateManagedMetadata(cmd *cobra.Command, request managedGeneration) error {
	if !request.Description && !request.Arguments {
		return nil
	}
	description, arguments, err := generationInputs(cmd, request)
	if err != nil {
		return warnManagedGeneration(cmd, request, err)
	}
	cfg, err := config.Load()
	if err != nil {
		return warnManagedGeneration(cmd, request, err)
	}
	logger := slog.New(slog.NewTextHandler(cmd.ErrOrStderr(), &slog.HandlerOptions{Level: slog.LevelWarn}))
	generator, err := metadataclient.NewConfigured(cfg, httpclient.New(cfg.HTTPHeaders, logger))
	if err != nil {
		return warnManagedGeneration(cmd, request, err)
	}
	if request.Description && request.Interactive {
		accepted, err := confirmManagedGeneration(cmd, cfg, managedGeneration{Description: true, Form: request.Form}, description, metadataservice.ArgumentsInput{})
		if err != nil {
			if errors.Is(err, metadataservice.ErrInvalidInput) {
				return warnManagedGeneration(cmd, request, err)
			}
			return err
		}
		if !accepted {
			return nil
		}
	}
	if request.Description {
		candidate, err := generator.GenerateDescription(cmd.Context(), description)
		if err == nil {
			err = applyGeneratedDescription(request, candidate)
		}
		if err != nil {
			if err := generationWarning(cmd, locale.KeyManagedDescription, err); err != nil {
				return err
			}
		}
	}
	if request.Alias != nil {
		arguments.Description = request.Alias.Description
	} else {
		arguments.Description = request.Script.Description
	}
	if request.Arguments && request.Interactive {
		accepted, err := confirmManagedGeneration(cmd, cfg, managedGeneration{Arguments: true, Form: request.Form}, metadataservice.DescriptionInput{}, arguments)
		if err != nil {
			if errors.Is(err, metadataservice.ErrInvalidInput) {
				return warnManagedGeneration(cmd, request, err)
			}
			return err
		}
		if !accepted {
			return nil
		}
	}
	if request.Arguments {
		candidate, err := generator.GenerateArguments(cmd.Context(), arguments)
		if err == nil {
			err = applyGeneratedArguments(request, candidate)
		}
		if err != nil {
			if err := generationWarning(cmd, locale.KeyManagedArgumentsJSON, err); err != nil {
				return err
			}
		}
	}
	if err := cmd.Context().Err(); err != nil {
		return fmt.Errorf("generate managed metadata: %w", err)
	}
	return nil
}

func applyGeneratedDescription(request managedGeneration, value string) error {
	if request.Alias != nil {
		candidate := *request.Alias
		candidate.Description = value
		if err := metadata.ValidateAlias(candidate); err != nil {
			return fmt.Errorf("validate generated alias description: %w", err)
		}
		request.Alias.Description = value
		return nil
	}
	candidate := *request.Script
	candidate.Description = value
	if err := metadata.ValidateLegacyScript(candidate); err != nil {
		return fmt.Errorf("validate generated script description: %w", err)
	}
	request.Script.Description = value
	return nil
}

func applyGeneratedArguments(request managedGeneration, value []domain.Argument) error {
	if request.Alias != nil {
		candidate := *request.Alias
		candidate.Arguments = value
		if err := metadata.ValidateAlias(candidate); err != nil {
			return fmt.Errorf("validate generated alias arguments: %w", err)
		}
		request.Alias.Arguments = value
		return nil
	}
	candidate := *request.Script
	candidate.Arguments = value
	if err := metadata.ValidateLegacyScript(candidate); err != nil {
		return fmt.Errorf("validate generated script arguments: %w", err)
	}
	request.Script.Arguments = value
	return nil
}

func generationWarning(cmd *cobra.Command, field locale.Key, cause error) error {
	if err := cmd.Context().Err(); err != nil {
		return fmt.Errorf("generate managed metadata: %w", err)
	}
	return writeQueryOutput(cmd.ErrOrStderr(), locale.Text(locale.KeyManagedGenerationWarning), locale.Text(field), cause)
}

func warnManagedGeneration(cmd *cobra.Command, request managedGeneration, cause error) error {
	if request.Description {
		if err := generationWarning(cmd, locale.KeyManagedDescription, cause); err != nil {
			return err
		}
	}
	if request.Arguments {
		return generationWarning(cmd, locale.KeyManagedArgumentsJSON, cause)
	}
	return nil
}

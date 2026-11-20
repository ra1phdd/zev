package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"

	"github.com/ra1phdd/zev/internal/core/config"
	"github.com/ra1phdd/zev/internal/core/domain"
	"github.com/ra1phdd/zev/internal/core/locale"
	"github.com/ra1phdd/zev/internal/core/security"
	toolrepo "github.com/ra1phdd/zev/internal/entities/tool/repository"
	toolservice "github.com/ra1phdd/zev/internal/entities/tool/service"
	"github.com/ra1phdd/zev/pkg/tui/prompt"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

const (
	toolModeAuto   = "auto"
	toolModeManual = "manual"
	toolModeBypass = "bypass"
)

func registerToolFlags(cmd *cobra.Command) {
	cmd.Flags().Bool("tools", false, locale.Text(locale.KeyToolsEnableModelContextToolsRequiredExplicitlyPrint))
	cmd.Flags().Bool("no-tools", false, locale.Text(locale.KeyToolsDisableModelContextTools))
	cmd.Flags().String("tool-approval", "", locale.Text(locale.KeyToolsToolApprovalPolicyManualAutoBypassOverrides))
}

func validateToolFlags(flags parsedFlags) error {
	if flags.Tools && flags.NoTools {
		return errors.New(locale.Text(locale.KeyToolsToolsNoToolsCannotUsedTogether))
	}
	if flags.ToolApprovalSet && flags.ToolApproval != toolModeManual && flags.ToolApproval != toolModeAuto && flags.ToolApproval != toolModeBypass {
		return errors.New(locale.Text(locale.KeyToolsToolApprovalMustManualAutoBypass))
	}
	return nil
}

func resolveToolSettings(cfg *config.Config, flags parsedFlags) (bool, string) {
	mode := cfg.ToolApprovalMode
	if flags.ToolApprovalSet {
		mode = flags.ToolApproval
	}
	enabled := cfg.ToolsEnabled
	if flags.Print || flags.JSON || flags.NoContext || !cfg.ContextEnabled {
		enabled = false
	}
	if flags.Tools {
		enabled = true
	}
	if flags.NoTools {
		enabled = false
	}
	return enabled, mode
}

type toolProviderParams struct {
	Config   *config.Config
	Flags    parsedFlags
	Provider domain.InferenceProvider
}

func configureQueryTools(cmd *cobra.Command, params toolProviderParams) (domain.InferenceProvider, error) {
	cfg, flags, provider := params.Config, params.Flags, params.Provider
	enabled, mode := resolveToolSettings(cfg, flags)
	if !enabled {
		return provider, nil
	}
	root, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("get tool working directory: %w", err)
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return nil, fmt.Errorf("canonicalize tool working directory: %w", err)
	}
	executor, err := toolrepo.New(root)
	if err != nil {
		return nil, fmt.Errorf("initialize tools: %w", err)
	}
	settings := toolservice.Settings{Mode: mode, Root: root, Executor: executor, Policy: cfg.Security.Policy().WithContext(root, homeDirectory()), WebFetch: cfg.Security.WebFetchEnabled, Fetcher: security.Fetcher{Proxy: flags.Proxy}}
	// Machine output never opens a TUI, even with --tools. A manual tool call is
	// returned to the model as denied instead of silently approving or blocking.
	if !flags.Print && !flags.JSON && interactiveToolInput(cmd) {
		settings.Confirm = func(ctx context.Context, request domain.ExecRequest) (bool, error) {
			encoded, encodeErr := json.Marshal(request)
			if encodeErr != nil {
				return false, fmt.Errorf("encode tool approval: %w", encodeErr)
			}
			title := fmt.Sprintf(locale.Text(locale.KeyToolsModelRequestsLocalCommandWorkingDirectoryQ), root, strconv.QuoteToASCII(string(encoded)))
			return prompt.ConfirmContext(ctx, prompt.ApprovalOptions{Prompt: title, Output: cmd.ErrOrStderr(), Input: cmd.InOrStdin()})
		}
	}
	if mode == toolModeBypass {
		if _, err := fmt.Fprintln(cmd.ErrOrStderr(), locale.Text(locale.KeyToolsWarningToolBypassPermitsArbitraryCommandsUser)); err != nil {
			return nil, fmt.Errorf("write tool warning: %w", errors.Join(err, executor.Close()))
		}
	}
	wrapped, err := toolservice.New(provider, settings)
	if err != nil {
		return nil, fmt.Errorf("configure model tools: %w", errors.Join(err, executor.Close()))
	}
	return wrapped, nil
}

func closeQueryTools(provider domain.InferenceProvider) error {
	if closer, ok := provider.(io.Closer); ok {
		if err := closer.Close(); err != nil {
			return fmt.Errorf("close model tools: %w", err)
		}
	}
	return nil
}

func interactiveToolInput(cmd *cobra.Command) bool {
	file, ok := cmd.InOrStdin().(*os.File)
	return ok && term.IsTerminal(int(file.Fd()))
}

package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/ra1phdd/zev/internal/core/config"
	"github.com/ra1phdd/zev/internal/core/locale"
	"github.com/ra1phdd/zev/pkg/tui/prompt"
	"github.com/spf13/cobra"
)

func newSetupHeadersCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "headers",
		Short: locale.Text(locale.KeyCliConfigureCustomHTTPHeaders),
		Long:  locale.Text(locale.KeyCliConfigureCustomHTTPHeadersAllRequestsHeaders),
		RunE:  runSetupHeaders,
	}
}

func runSetupHeaders(cmd *cobra.Command, _ []string) error {
	cfg, err := loadConfig(cmd.Context())
	if err != nil {
		return fmt.Errorf(locale.Text(locale.KeyCliFailedLoadConfigError), err)
	}

	out := cmd.OutOrStdout()
	if err := showHTTPHeaders(out, locale.Text(locale.KeyCliCurrentHTTPHeaders), cfg.HTTPHeaders); err != nil {
		return err
	}

	if _, err := fmt.Fprint(out, locale.Text(locale.KeyCliEnterHeadersOnePerLineFormatHeader)); err != nil {
		return fmt.Errorf("write header instructions: %w", err)
	}

	newHeaders, err := readHTTPHeaders(cmd.Context(), out)
	if err != nil {
		return err
	}
	if len(newHeaders) == 0 {
		if _, err := fmt.Fprintln(out, locale.Text(locale.KeyCliNoHeadersEnteredConfigurationUnchanged)); err != nil {
			return fmt.Errorf("write unchanged header confirmation: %w", err)
		}
		return nil
	}
	cfg.HTTPHeaders = newHeaders

	if err := config.Save(cfg); err != nil {
		return fmt.Errorf(locale.Text(locale.KeyCliFailedSaveConfigError), err)
	}

	if _, err := fmt.Fprintln(out, locale.Text(locale.KeyCliHTTPHeadersUpdatedSuccessfully)); err != nil {
		return fmt.Errorf("write header confirmation: %w", err)
	}
	return showHTTPHeaders(out, locale.Text(locale.KeyCliNewHeaders), cfg.HTTPHeaders)
}

func showHTTPHeaders(out io.Writer, title string, headers map[string]string) error {
	if _, err := fmt.Fprintln(out, title); err != nil {
		return fmt.Errorf("write header list title: %w", err)
	}
	if len(headers) == 0 {
		if _, err := fmt.Fprintln(out, locale.Text(locale.KeyCliNoneSet)); err != nil {
			return fmt.Errorf("write empty header list: %w", err)
		}
		return nil
	}
	for key := range headers {
		if _, err := fmt.Fprintf(out, locale.Text(locale.KeyCliSRedacted), key); err != nil {
			return fmt.Errorf("write HTTP header %q: %w", key, err)
		}
	}
	return nil
}

func readHTTPHeaders(ctx context.Context, out io.Writer) (map[string]string, error) {
	headers := make(map[string]string)
	for {
		line, err := prompt.SecretInputContext(ctx, locale.Text(locale.KeyCliHeaderEmptyFinish))
		if err != nil {
			return nil, fmt.Errorf(locale.Text(locale.KeyCliReadHTTPHeaderError), err)
		}

		line = strings.TrimSpace(line)
		if line == "" {
			return headers, nil
		}

		name, value, ok := strings.Cut(line, ":")
		if !ok {
			if _, err := fmt.Fprintln(out, locale.Text(locale.KeyCliInvalidFormatHeaderNameHeaderValue)); err != nil {
				return nil, fmt.Errorf("write invalid header warning: %w", err)
			}
			continue
		}
		name = strings.TrimSpace(name)
		value = strings.TrimSpace(value)
		if name == "" {
			if _, err := fmt.Fprintln(out, locale.Text(locale.KeyCliHeaderNameCannotEmpty)); err != nil {
				return nil, fmt.Errorf("write empty header warning: %w", err)
			}
			continue
		}

		headers[name] = value
		if _, err := fmt.Fprintf(out, locale.Text(locale.KeyCliAddedSRedacted), name); err != nil {
			return nil, fmt.Errorf("write added header confirmation: %w", err)
		}
	}
}

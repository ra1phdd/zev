package main

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/ra1phdd/zev/internal/core/config"
	"github.com/ra1phdd/zev/pkg/tui/prompt"
	"github.com/spf13/cobra"
)

func newSetupHeadersCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "headers",
		Short: "Configure custom HTTP headers",
		Long: `Configure custom HTTP headers for all requests.

Headers are applied to all HTTP requests to LLM providers.
Common use cases:
  - Custom User-Agent
  - Authorization headers
  - API version headers
  - Custom tracking headers

Format: "Header-Name:Header-Value" (one per line, empty line to finish)`,
		RunE: runSetupHeaders,
	}
}

func runSetupHeaders(cmd *cobra.Command, _ []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	out := cmd.OutOrStdout()
	if err := showHTTPHeaders(out, "\n📋 Current HTTP headers:", cfg.HTTPHeaders); err != nil {
		return err
	}

	if _, err := fmt.Fprint(out, `
✏️  Enter headers one per line in format 'Header-Name:Header-Value'
   Press Enter on empty line to finish
   Common headers:
     User-Agent:zev/1.0
     X-Custom-Header:custom-value
     Authorization:Bearer ***

`); err != nil {
		return fmt.Errorf("write header instructions: %w", err)
	}

	newHeaders, err := readHTTPHeaders(cmd.Context(), out)
	if err != nil {
		return err
	}
	if len(newHeaders) == 0 {
		if _, err := fmt.Fprintln(out, "\n⚠️  No headers entered. Configuration unchanged."); err != nil {
			return fmt.Errorf("write unchanged header confirmation: %w", err)
		}
		return nil
	}
	cfg.HTTPHeaders = newHeaders

	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	if _, err := fmt.Fprintln(out, "\n✅ HTTP headers updated successfully!"); err != nil {
		return fmt.Errorf("write header confirmation: %w", err)
	}
	return showHTTPHeaders(out, "\n📋 New headers:", cfg.HTTPHeaders)
}

func showHTTPHeaders(out io.Writer, title string, headers map[string]string) error {
	if _, err := fmt.Fprintln(out, title); err != nil {
		return fmt.Errorf("write header list title: %w", err)
	}
	if len(headers) == 0 {
		if _, err := fmt.Fprintln(out, "  (none set)"); err != nil {
			return fmt.Errorf("write empty header list: %w", err)
		}
		return nil
	}
	for key := range headers {
		if _, err := fmt.Fprintf(out, "  %s: [redacted]\n", key); err != nil {
			return fmt.Errorf("write HTTP header %q: %w", key, err)
		}
	}
	return nil
}

func readHTTPHeaders(ctx context.Context, out io.Writer) (map[string]string, error) {
	headers := make(map[string]string)
	for {
		line, err := prompt.SecretInputContext(ctx, "Header (empty to finish):")
		if err != nil {
			return nil, fmt.Errorf("read HTTP header: %w", err)
		}

		line = strings.TrimSpace(line)
		if line == "" {
			return headers, nil
		}

		name, value, ok := strings.Cut(line, ":")
		if !ok {
			if _, err := fmt.Fprintln(out, "⚠️  Invalid format. Use 'Header-Name:Header-Value'"); err != nil {
				return nil, fmt.Errorf("write invalid header warning: %w", err)
			}
			continue
		}
		name = strings.TrimSpace(name)
		value = strings.TrimSpace(value)
		if name == "" {
			if _, err := fmt.Fprintln(out, "⚠️  Header name cannot be empty"); err != nil {
				return nil, fmt.Errorf("write empty header warning: %w", err)
			}
			continue
		}

		headers[name] = value
		if _, err := fmt.Fprintf(out, "✅ Added: %s: [redacted]\n", name); err != nil {
			return nil, fmt.Errorf("write added header confirmation: %w", err)
		}
	}
}

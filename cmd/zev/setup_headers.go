package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/ra1phdd/zev-go/internal/core/config"
	"github.com/ra1phdd/zev-go/pkg/tui/prompt"
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

func runSetupHeaders(_ *cobra.Command, _ []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	fmt.Fprintln(os.Stdout, "\n📋 Current HTTP headers:")
	if len(cfg.HTTPHeaders) == 0 {
		fmt.Fprintln(os.Stdout, "  (none set)")
	} else {
		for key, value := range cfg.HTTPHeaders {
			fmt.Fprintf(os.Stdout, "  %s: %s\n", key, value)
		}
	}

	fmt.Fprintln(os.Stdout, "\n✏️  Enter headers one per line in format 'Header-Name:Header-Value'")
	fmt.Fprintln(os.Stdout, "   Press Enter on empty line to finish")
	fmt.Fprintln(os.Stdout, "   Common headers:")
	fmt.Fprintln(os.Stdout, "     User-Agent:zev/1.0")
	fmt.Fprintln(os.Stdout, "     X-Custom-Header:custom-value")
	fmt.Fprintln(os.Stdout, "     Authorization:Bearer token123")
	fmt.Fprintln(os.Stdout, "")

	newHeaders := make(map[string]string)
	for {
		line, err := prompt.Input("Header (empty to finish):")
		if err != nil {
			return err
		}

		line = strings.TrimSpace(line)
		if line == "" {
			break
		}

		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			fmt.Fprintln(os.Stdout, "⚠️  Invalid format. Use 'Header-Name:Header-Value'")
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		if key == "" {
			fmt.Fprintln(os.Stdout, "⚠️  Header name cannot be empty")
			continue
		}

		newHeaders[key] = value
		fmt.Fprintf(os.Stdout, "✅ Added: %s: %s\n", key, value)
	}

	if len(newHeaders) == 0 {
		fmt.Fprintln(os.Stdout, "\n⚠️  No headers entered. Configuration unchanged.")
		return nil
	}
	cfg.HTTPHeaders = newHeaders

	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Fprintln(os.Stdout, "\n✅ HTTP headers updated successfully!")
	fmt.Fprintln(os.Stdout, "\n📋 New headers:")
	for key, value := range cfg.HTTPHeaders {
		fmt.Fprintf(os.Stdout, "  %s: %s\n", key, value)
	}

	return nil
}

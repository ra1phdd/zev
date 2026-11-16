package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/ra1phdd/zev/internal/core/config"
	"github.com/ra1phdd/zev/internal/core/httpclient"
	historyclient "github.com/ra1phdd/zev/internal/entities/history/client"
	"github.com/ra1phdd/zev/internal/entities/history/service"
	"github.com/spf13/cobra"
)

var errHistorySemanticDisabled = errors.New("history semantic search disabled")

type historyEmbeddingConfig struct {
	Provider string
	Model    string
	Enabled  bool
}

type historyIndexOperations interface {
	IndexStatus(context.Context, string) (service.IndexStatus, error)
	RebuildFTS(context.Context) error
	RebuildVectors(context.Context, bool, int) (service.RebuildResult, error)
}

type historyEmbeddingConfigProvider interface {
	EmbeddingConfig() historyEmbeddingConfig
}

type historyEmbeddingProfileProvider interface {
	EmbeddingProfile() string
}

func newHistoryEmbedder(cfg *config.Config) (service.Embedder, error) {
	if cfg == nil || !cfg.HistorySemanticEnabled {
		return nil, errHistorySemanticDisabled
	}
	client, err := historyclient.New(historyclient.Config{
		Provider:   cfg.HistoryEmbeddingProvider,
		BaseURL:    cfg.HistoryEmbeddingBaseURL,
		Model:      cfg.HistoryEmbeddingModel,
		APIKey:     cfg.HistoryEmbeddingAPIKey,
		Dimensions: cfg.HistoryEmbeddingDimensions,
		MaxBytes:   cfg.HistoryEmbeddingMaxBytes,
	}, httpclient.New(nil))
	if err != nil {
		return nil, fmt.Errorf("create embedding client: %w", err)
	}
	return client, nil
}

func newHistoryIndexCmd(open historyOpener) *cobra.Command {
	index := &cobra.Command{Use: "index", Short: "Inspect and maintain history indexes", Args: cobra.NoArgs}
	index.AddCommand(newHistoryIndexStatusCmd(open), newHistoryIndexRebuildCmd(open))
	return index
}

func newHistoryIndexStatusCmd(open historyOpener) *cobra.Command {
	var jsonOutput bool
	status := &cobra.Command{Use: "status", Short: "Show history index status", Args: cobra.NoArgs}
	status.Flags().BoolVar(&jsonOutput, "json", false, "Output JSON")
	status.RunE = func(cmd *cobra.Command, _ []string) error {
		return withHistory(cmd, open, func(raw historyOperations) error {
			index, ok := raw.(historyIndexOperations)
			if !ok {
				return errors.New("history index maintenance is unavailable")
			}
			status, err := index.IndexStatus(cmd.Context(), historyEmbeddingProfile(raw))
			if err != nil {
				return fmt.Errorf("read history index status: %w", err)
			}
			if jsonOutput {
				if err := json.NewEncoder(cmd.OutOrStdout()).Encode(status); err != nil {
					return fmt.Errorf("write history index status: %w", err)
				}
				return nil
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "documents=%d lexical_ready=%t eligible=%d ready=%d failed=%d stale=%d\n", status.Documents, status.LexicalReady, status.EmbeddingCoverage.Eligible, status.EmbeddingCoverage.Ready, status.EmbeddingCoverage.Failed, status.EmbeddingCoverage.Stale); err != nil {
				return fmt.Errorf("write history index status: %w", err)
			}
			return nil
		})
	}
	return status
}

func newHistoryIndexRebuildCmd(open historyOpener) *cobra.Command {
	var fts, vectors, retryFailed, yes bool
	rebuild := &cobra.Command{Use: "rebuild", Short: "Rebuild history indexes", Args: cobra.NoArgs}
	flags := rebuild.Flags()
	flags.BoolVar(&fts, "fts", false, "Rebuild lexical indexes")
	flags.BoolVar(&vectors, "vectors", false, "Rebuild semantic vectors")
	flags.BoolVar(&retryFailed, "retry-failed", false, "Retry previously failed vector messages")
	flags.BoolVar(&yes, "yes", false, "Confirm vector transmission")
	rebuild.RunE = func(cmd *cobra.Command, _ []string) error {
		if err := validateHistoryRebuildFlags(fts, vectors, retryFailed); err != nil {
			return err
		}
		return withHistory(cmd, open, func(raw historyOperations) error {
			index, ok := raw.(historyIndexOperations)
			if !ok {
				return errors.New("history index maintenance is unavailable")
			}
			if fts {
				if err := index.RebuildFTS(cmd.Context()); err != nil {
					return fmt.Errorf("rebuild FTS indexes: %w", err)
				}
				if _, err := fmt.Fprintln(cmd.OutOrStdout(), "FTS indexes rebuilt"); err != nil {
					return fmt.Errorf("write FTS result: %w", err)
				}
				return nil
			}
			return rebuildHistoryVectors(cmd, raw, index, retryFailed, yes)
		})
	}
	return rebuild
}

func validateHistoryRebuildFlags(fts, vectors, retryFailed bool) error {
	if !fts && !vectors {
		return errors.New("choose --fts or --vectors")
	}
	if fts && vectors {
		return errors.New("--fts and --vectors are mutually exclusive")
	}
	if retryFailed && !vectors {
		return errors.New("--retry-failed requires --vectors")
	}
	return nil
}

func rebuildHistoryVectors(cmd *cobra.Command, raw historyOperations, index historyIndexOperations, retryFailed, yes bool) error {
	status, err := index.IndexStatus(cmd.Context(), historyEmbeddingProfile(raw))
	if err != nil {
		return fmt.Errorf("read vector rebuild status: %w", err)
	}
	cfg := historyEmbeddingConfig{}
	if provider, ok := raw.(historyEmbeddingConfigProvider); ok {
		cfg = provider.EmbeddingConfig()
	}
	if _, err := fmt.Fprintf(cmd.ErrOrStderr(), "embedding provider=%s model=%s eligible=%d\n", cfg.Provider, cfg.Model, status.EmbeddingCoverage.Eligible); err != nil {
		return fmt.Errorf("write vector rebuild summary: %w", err)
	}
	if !yes {
		return errors.New("vector rebuild requires explicit --yes consent")
	}
	result, err := index.RebuildVectors(cmd.Context(), retryFailed, 0)
	if err != nil {
		return fmt.Errorf("rebuild vectors: %w", err)
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "vectors rebuilt: ready=%d failed=%d\n", result.Ready, result.Failed); err != nil {
		return fmt.Errorf("write vector rebuild result: %w", err)
	}
	return nil
}

func historyEmbeddingProfile(raw historyOperations) string {
	if provider, ok := raw.(historyEmbeddingProfileProvider); ok {
		return provider.EmbeddingProfile()
	}
	return ""
}

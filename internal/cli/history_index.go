package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/ra1phdd/zev/internal/core/config"
	"github.com/ra1phdd/zev/internal/core/httpclient"
	"github.com/ra1phdd/zev/internal/core/locale"
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
	index := &cobra.Command{Use: "index", Short: locale.Text(locale.KeyCliInspectMaintainHistoryIndexes), Args: cobra.NoArgs}
	index.AddCommand(newHistoryIndexStatusCmd(open), newHistoryIndexRebuildCmd(open))
	return index
}

func newHistoryIndexStatusCmd(open historyOpener) *cobra.Command {
	var jsonOutput bool
	status := &cobra.Command{Use: "status", Short: locale.Text(locale.KeyCliShowHistoryIndexStatus), Args: cobra.NoArgs}
	status.Flags().BoolVar(&jsonOutput, "json", false, locale.Text(locale.KeyCliOutputJSON))
	status.RunE = func(cmd *cobra.Command, _ []string) error {
		return withHistory(cmd, open, func(raw historyOperations) error {
			index, ok := raw.(historyIndexOperations)
			if !ok {
				return errors.New(locale.Text(locale.KeyCliHistoryIndexMaintenanceUnavailable))
			}
			status, err := index.IndexStatus(cmd.Context(), historyEmbeddingProfile(raw))
			if err != nil {
				return fmt.Errorf(locale.Text(locale.KeyCliReadHistoryIndexStatusError), err)
			}
			if jsonOutput {
				if err := json.NewEncoder(cmd.OutOrStdout()).Encode(status); err != nil {
					return fmt.Errorf("write history index status: %w", err)
				}
				return nil
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), locale.Text(locale.KeyCliIndexStatusLine), status.Documents, status.LexicalReady, status.EmbeddingCoverage.Eligible, status.EmbeddingCoverage.Ready, status.EmbeddingCoverage.Failed, status.EmbeddingCoverage.Stale); err != nil {
				return fmt.Errorf("write history index status: %w", err)
			}
			return nil
		})
	}
	return status
}

func newHistoryIndexRebuildCmd(open historyOpener) *cobra.Command {
	var fts, vectors, retryFailed, yes bool
	rebuild := &cobra.Command{Use: "rebuild", Short: locale.Text(locale.KeyCliRebuildHistoryIndexes), Args: cobra.NoArgs}
	flags := rebuild.Flags()
	flags.BoolVar(&fts, "fts", false, locale.Text(locale.KeyCliRebuildLexicalIndexes))
	flags.BoolVar(&vectors, "vectors", false, locale.Text(locale.KeyCliRebuildSemanticVectors))
	flags.BoolVar(&retryFailed, "retry-failed", false, locale.Text(locale.KeyCliRetryPreviouslyFailedVectorMessages))
	flags.BoolVar(&yes, "yes", false, locale.Text(locale.KeyCliConfirmVectorTransmission))
	rebuild.RunE = func(cmd *cobra.Command, _ []string) error {
		if err := validateHistoryRebuildFlags(fts, vectors, retryFailed); err != nil {
			return err
		}
		return withHistory(cmd, open, func(raw historyOperations) error {
			index, ok := raw.(historyIndexOperations)
			if !ok {
				return errors.New(locale.Text(locale.KeyCliHistoryIndexMaintenanceUnavailable))
			}
			if fts {
				if err := index.RebuildFTS(cmd.Context()); err != nil {
					return fmt.Errorf(locale.Text(locale.KeyCliRebuildFtsIndexesError), err)
				}
				if _, err := fmt.Fprintln(cmd.OutOrStdout(), locale.Text(locale.KeyCliFtsIndexesRebuilt)); err != nil {
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
		return errors.New(locale.Text(locale.KeyCliChooseFtsVectors))
	}
	if fts && vectors {
		return errors.New(locale.Text(locale.KeyCliFtsVectorsMutuallyExclusive))
	}
	if retryFailed && !vectors {
		return errors.New(locale.Text(locale.KeyCliRetryFailedRequiresVectors))
	}
	return nil
}

func rebuildHistoryVectors(cmd *cobra.Command, raw historyOperations, index historyIndexOperations, retryFailed, yes bool) error {
	status, err := index.IndexStatus(cmd.Context(), historyEmbeddingProfile(raw))
	if err != nil {
		return fmt.Errorf(locale.Text(locale.KeyCliReadVectorRebuildStatusError), err)
	}
	cfg := historyEmbeddingConfig{}
	if provider, ok := raw.(historyEmbeddingConfigProvider); ok {
		cfg = provider.EmbeddingConfig()
	}
	if _, err := fmt.Fprintf(cmd.ErrOrStderr(), locale.Text(locale.KeyCliEmbeddingStatusLine), cfg.Provider, cfg.Model, status.EmbeddingCoverage.Eligible); err != nil {
		return fmt.Errorf("write vector rebuild summary: %w", err)
	}
	if !yes {
		return errors.New(locale.Text(locale.KeyCliVectorRebuildRequiresExplicitYesConsent))
	}
	result, err := index.RebuildVectors(cmd.Context(), retryFailed, 0)
	if err != nil {
		return fmt.Errorf(locale.Text(locale.KeyCliRebuildVectorsError), err)
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), locale.Text(locale.KeyCliVectorsRebuiltReadyDFailedSummary), result.Ready, result.Failed); err != nil {
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

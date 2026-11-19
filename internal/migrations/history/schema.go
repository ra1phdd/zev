package history

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/ra1phdd/zev/internal/entities/history/service"
)

// Version is the released history schema version.
const Version = 1

const (
	columnCreatedAt      = "created_at"
	columnConversationID = "conversation_id"
	columnMessageID      = "message_id"
	columnKind           = "kind"
	columnMetadataJSON   = "metadata_json"
	columnModel          = "model"
	columnStatus         = "status"
)

var requiredTables = map[string][]string{
	"history_schema":         {"id", "format"},
	"conversations":          {"id", columnCreatedAt, "updated_at", columnStatus, columnKind, "provider", columnModel, "selected_command_id", columnMetadataJSON},
	"messages":               {"id", columnConversationID, "sequence", "role", columnKind, "content", columnCreatedAt, columnMetadataJSON, "parts_json"},
	"responses":              {"id", columnConversationID, columnMessageID, "provider", columnModel, "payload_json", "is_valid", columnCreatedAt},
	"commands":               {"id", "response_id", columnConversationID, "ordinal", "command", "short_explanation", "is_dangerous", "dangerous_explanation", "selected_at", "executed_at", "execution_exit_code", "execution_error", "delivery_outcome"},
	"message_embeddings":     {columnMessageID, "profile_id", columnModel, "dimensions", "embedding", "content_hash", columnCreatedAt, "status", "error"},
	"history_documents":      {"id", columnConversationID, columnMessageID, "command_id", columnKind, "content", "document_type", columnCreatedAt, "profile_id", "normalized", "morphology"},
	"history_fts":            nil,
	"history_fts_surface":    nil,
	"history_fts_morphology": nil,
	"history_fts_trigram":    nil,
	"history_tokens":         {"document_id", "token"},
	"history_deletions":      {"token", "deletion_key"},
	"history_revision":       {"id", "revision"},
	"history_writes":         {columnConversationID, "initial_message_id", "fingerprint"},
	"migration_runs":         {"id", "source_fingerprint", "status", "started_at", "completed_at", columnMetadataJSON},
	"migration_lines":        {"run_id", "line_number", columnConversationID},
}

var requiredIndexes = []string{
	"conversations_created_idx",
	"messages_conversation_idx",
	"history_deletions_key",
}

// ValidateLedger checks the released migration ledger without changing it.
func ValidateLedger(ctx context.Context, db *sql.DB) error {
	var columns int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM pragma_table_info('schema_migrations') WHERE name IN ('version','dirty')`).Scan(&columns); err != nil {
		return fmt.Errorf("inspect history migration ledger: %w", err)
	}
	if columns != 2 {
		return fmt.Errorf("%w: history migration ledger is not the release ledger", service.ErrUnsupportedSchema)
	}
	var rows int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations`).Scan(&rows); err != nil {
		return fmt.Errorf("inspect history migration ledger rows: %w", err)
	}
	if rows != 1 {
		return fmt.Errorf("%w: history migration ledger has %d rows", service.ErrUnsupportedSchema, rows)
	}
	var version int
	var dirty bool
	if err := db.QueryRowContext(ctx, `SELECT version,dirty FROM schema_migrations`).Scan(&version, &dirty); err != nil {
		return fmt.Errorf("%w: history migration ledger is invalid: %w", service.ErrUnsupportedSchema, err)
	}
	if version != Version || dirty {
		return fmt.Errorf("%w: history migration ledger version=%d dirty=%t", service.ErrUnsupportedSchema, version, dirty)
	}
	return nil
}

// Validate checks that db has the released history schema. It never changes db.
func Validate(ctx context.Context, db *sql.DB) error {
	if err := ValidateLedger(ctx, db); err != nil {
		return err
	}
	if err := validateTables(ctx, db); err != nil {
		return err
	}
	if err := validateIdentity(ctx, db); err != nil {
		return err
	}
	if err := validateIndexes(ctx, db); err != nil {
		return err
	}
	var revision int64
	if err := db.QueryRowContext(ctx, `SELECT revision FROM history_revision WHERE id=1`).Scan(&revision); err != nil {
		return fmt.Errorf("%w: inspect history revision: %w", service.ErrUnsupportedSchema, err)
	}
	return nil
}

func validateTables(ctx context.Context, db *sql.DB) error {
	for table, columns := range requiredTables {
		var kind string
		if err := db.QueryRowContext(ctx, `SELECT type FROM sqlite_schema WHERE name=?`, table).Scan(&kind); err != nil {
			return fmt.Errorf("%w: missing history object %s: %w", service.ErrUnsupportedSchema, table, err)
		}
		for _, column := range columns {
			var count int
			if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM pragma_table_info(?) WHERE name=?`, table, column).Scan(&count); err != nil {
				return fmt.Errorf("%w: inspect history table %s: %w", service.ErrUnsupportedSchema, table, err)
			}
			if count != 1 {
				return fmt.Errorf("%w: history table %s is missing column %s", service.ErrUnsupportedSchema, table, column)
			}
		}
	}
	return nil
}

func validateIdentity(ctx context.Context, db *sql.DB) error {
	var format string
	if err := db.QueryRowContext(ctx, `SELECT format FROM history_schema WHERE id=1`).Scan(&format); err != nil {
		return fmt.Errorf("%w: history schema identity is not the release baseline: %w", service.ErrUnsupportedSchema, err)
	}
	if format != "zev-history-release-v1" {
		return fmt.Errorf("%w: history schema identity is not the release baseline", service.ErrUnsupportedSchema)
	}
	return nil
}

func validateIndexes(ctx context.Context, db *sql.DB) error {
	for _, index := range requiredIndexes {
		var count int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_schema WHERE type='index' AND name=?`, index).Scan(&count); err != nil {
			return fmt.Errorf("%w: inspect history index %s: %w", service.ErrUnsupportedSchema, index, err)
		}
		if count != 1 {
			return fmt.Errorf("%w: missing history index %s", service.ErrUnsupportedSchema, index)
		}
	}
	return nil
}

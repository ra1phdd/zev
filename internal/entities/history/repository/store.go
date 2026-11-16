package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	"github.com/ra1phdd/zev/internal/core/filestore"
	"github.com/ra1phdd/zev/internal/entities/history/service"
	_ "modernc.org/sqlite" // Register the SQLite database/sql driver.
)

// Store persists history in a SQLite database.
type Store struct{ db *sql.DB }

// Open opens or creates a private SQLite history database and applies migrations.
// The caller owns the returned store and must close it. Schema setup and legacy
// migration share one lock so another opener cannot observe an unfinished import.
func Open(ctx context.Context, path string) (*Store, error) {
	if path == "" {
		return nil, fmt.Errorf("%w: empty database path", service.ErrInvalid)
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("open history: %w", err)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve history path: %w", err)
	}
	dir := filepath.Dir(abs)
	if err := filestore.ValidateDirectory(dir); err != nil {
		return nil, fmt.Errorf("validate history directory: %w", err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create history directory: %w", err)
	}
	if err := secureHistoryPath(dir, true); err != nil {
		return nil, fmt.Errorf("secure history directory: %w", err)
	}
	if err := filestore.ValidateDirectory(dir); err != nil {
		return nil, fmt.Errorf("validate history directory: %w", err)
	}
	var store *Store
	source := filepath.Join(dir, "history")
	err = filestore.WithLockContext(ctx, source, func() error {
		var err error
		store, err = openLocked(ctx, abs)
		return err
	})
	if err != nil {
		if store != nil {
			err = errors.Join(err, store.Close())
		}
		return nil, fmt.Errorf("initialize history: %w", err)
	}
	return store, nil
}

func secureHistorySidecars(abs string) error {
	for _, sidecar := range []string{abs + "-wal", abs + "-shm"} {
		info, err := os.Lstat(sidecar)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return fmt.Errorf("inspect database sidecar: %w", err)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("%w: database sidecar is not a regular file", service.ErrInvalid)
		}
		if err := secureHistoryPath(sidecar, false); err != nil {
			return fmt.Errorf("secure database sidecar: %w", err)
		}
	}
	return nil
}

func openLocked(ctx context.Context, abs string) (*Store, error) {
	if err := secureHistorySidecars(abs); err != nil {
		return nil, err
	}
	info, err := os.Lstat(abs)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("inspect history database: %w", err)
	}
	if err == nil && !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%w: database is not a regular file", service.ErrInvalid)
	}
	if err == nil {
		if err := secureHistoryPath(abs, false); err != nil {
			return nil, fmt.Errorf("secure history database: %w", err)
		}
	}
	f, err := os.OpenFile(abs, os.O_CREATE|os.O_RDWR, 0o600) //nolint:gosec // The caller-selected database leaf and trusted application directory were validated.
	if err != nil {
		return nil, fmt.Errorf("open history database: %w", err)
	}
	if err = secureHistoryPath(abs, false); err != nil {
		return nil, errors.Join(fmt.Errorf("secure history database: %w", err), f.Close())
	}
	if err = f.Close(); err != nil {
		return nil, fmt.Errorf("close history database: %w", err)
	}
	u := &url.URL{Scheme: "file", Path: filepath.ToSlash(abs)}
	q := u.Query()
	q.Add("_pragma", "foreign_keys(1)")
	q.Add("_pragma", "busy_timeout(5000)")
	// Reserve the writer before read-modify-write transactions so concurrent
	// processes wait rather than fail while upgrading a stale read snapshot.
	q.Set("_txlock", "immediate")
	u.RawQuery = q.Encode()
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	if _, err = db.ExecContext(ctx, "PRAGMA journal_mode=WAL"); err != nil {
		return nil, errors.Join(fmt.Errorf("enable WAL: %w", err), db.Close())
	}
	s := &Store{db: db}
	if err = s.migrate(ctx); err != nil {
		return nil, errors.Join(err, db.Close())
	}
	if err = s.importLegacyLocked(ctx, filepath.Join(filepath.Dir(abs), "history")); err != nil {
		return nil, errors.Join(err, db.Close())
	}
	return s, nil
}

// Close releases the database connection.
func (s *Store) Close() error {
	if err := s.db.Close(); err != nil {
		return fmt.Errorf("close history database: %w", err)
	}
	return nil
}

func (s *Store) migrate(ctx context.Context) (err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin migration: %w", err)
	}
	defer rollbackTransaction(&err, tx)
	if _, err = tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL)`); err != nil {
		return fmt.Errorf("create migration ledger: %w", err)
	}
	var version int
	if err = tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(version),0) FROM schema_migrations`).Scan(&version); err != nil {
		return fmt.Errorf("read migration version: %w", err)
	}
	migrations := []string{schema, schema2, schema3}
	if version > len(migrations) {
		return fmt.Errorf("%w: version %d", service.ErrUnsupportedSchema, version)
	}
	for i := version; i < len(migrations); i++ {
		if _, err = tx.ExecContext(ctx, migrations[i]); err != nil {
			return fmt.Errorf("apply migration %d: %w", i+1, err)
		}
		if i == 1 {
			if err = rebuildFTSTx(ctx, tx); err != nil {
				return fmt.Errorf("build migration index: %w", err)
			}
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO schema_migrations VALUES (?, strftime('%Y-%m-%dT%H:%M:%fZ','now'))`, i+1); err != nil {
			return fmt.Errorf("record migration %d: %w", i+1, err)
		}
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit migration: %w", err)
	}
	return nil
}

const schema = `
CREATE TABLE conversations (id TEXT PRIMARY KEY, created_at TEXT NOT NULL, updated_at TEXT NOT NULL, status TEXT NOT NULL CHECK(status IN ('started','completed','cancelled','failed')), kind TEXT NOT NULL, provider TEXT NOT NULL DEFAULT '', model TEXT NOT NULL DEFAULT '', selected_command_id TEXT REFERENCES commands(id) ON DELETE SET NULL, metadata_json TEXT CHECK(metadata_json IS NULL OR json_valid(metadata_json)));
CREATE TABLE messages (id TEXT PRIMARY KEY, conversation_id TEXT NOT NULL REFERENCES conversations(id) ON DELETE CASCADE, sequence INTEGER NOT NULL CHECK(sequence >= 0), role TEXT NOT NULL CHECK(role IN ('user','assistant','system')), kind TEXT NOT NULL CHECK(kind IN ('initial_prompt','refinement','response','system_prompt','error')), content TEXT NOT NULL, created_at TEXT NOT NULL, metadata_json TEXT CHECK(metadata_json IS NULL OR json_valid(metadata_json)), parts_json TEXT CHECK(parts_json IS NULL OR json_valid(parts_json)), UNIQUE(conversation_id,sequence), UNIQUE(id,conversation_id));
CREATE TABLE responses (id TEXT PRIMARY KEY, conversation_id TEXT NOT NULL REFERENCES conversations(id) ON DELETE CASCADE, message_id TEXT NOT NULL UNIQUE, provider TEXT NOT NULL, model TEXT NOT NULL, payload_json TEXT NOT NULL CHECK(json_valid(payload_json)), is_valid INTEGER NOT NULL CHECK(is_valid IN (0,1)), created_at TEXT NOT NULL, FOREIGN KEY(message_id,conversation_id) REFERENCES messages(id,conversation_id) ON DELETE CASCADE, UNIQUE(id,conversation_id));
CREATE TABLE commands (id TEXT PRIMARY KEY, response_id TEXT NOT NULL, conversation_id TEXT NOT NULL REFERENCES conversations(id) ON DELETE CASCADE, ordinal INTEGER NOT NULL CHECK(ordinal >= 0), command TEXT NOT NULL, short_explanation TEXT NOT NULL, is_dangerous INTEGER NOT NULL CHECK(is_dangerous IN (0,1)), dangerous_explanation TEXT NOT NULL, selected_at TEXT, executed_at TEXT, execution_exit_code INTEGER, execution_error TEXT NOT NULL DEFAULT '', FOREIGN KEY(response_id,conversation_id) REFERENCES responses(id,conversation_id) ON DELETE CASCADE, UNIQUE(response_id,ordinal));
CREATE TABLE message_embeddings (message_id TEXT NOT NULL REFERENCES messages(id) ON DELETE CASCADE, model TEXT NOT NULL, dimensions INTEGER NOT NULL CHECK(dimensions > 0), embedding BLOB NOT NULL, PRIMARY KEY(message_id,model));
CREATE TABLE history_documents (id INTEGER PRIMARY KEY, conversation_id TEXT NOT NULL REFERENCES conversations(id) ON DELETE CASCADE, message_id TEXT REFERENCES messages(id) ON DELETE CASCADE, command_id TEXT REFERENCES commands(id) ON DELETE CASCADE, kind TEXT NOT NULL, content TEXT NOT NULL, CHECK((message_id IS NOT NULL) != (command_id IS NOT NULL)));
CREATE VIRTUAL TABLE history_fts USING fts5(content, content='history_documents', content_rowid='id');
CREATE TRIGGER history_documents_ai AFTER INSERT ON history_documents BEGIN INSERT INTO history_fts(rowid,content) VALUES(new.id,new.content); END;
CREATE TRIGGER history_documents_ad AFTER DELETE ON history_documents BEGIN INSERT INTO history_fts(history_fts,rowid,content) VALUES('delete',old.id,old.content); END;
CREATE TRIGGER history_documents_au AFTER UPDATE ON history_documents BEGIN INSERT INTO history_fts(history_fts,rowid,content) VALUES('delete',old.id,old.content); INSERT INTO history_fts(rowid,content) VALUES(new.id,new.content); END;
CREATE TABLE migration_runs (id TEXT PRIMARY KEY, source_fingerprint TEXT NOT NULL, status TEXT NOT NULL, started_at TEXT NOT NULL, completed_at TEXT, metadata_json TEXT CHECK(metadata_json IS NULL OR json_valid(metadata_json)));
CREATE TABLE migration_lines (run_id TEXT NOT NULL REFERENCES migration_runs(id) ON DELETE CASCADE, line_number INTEGER NOT NULL CHECK(line_number > 0), conversation_id TEXT NOT NULL REFERENCES conversations(id) ON DELETE CASCADE, PRIMARY KEY(run_id,line_number));
CREATE INDEX conversations_created_idx ON conversations(created_at DESC,id DESC);
CREATE INDEX messages_conversation_idx ON messages(conversation_id,sequence);
`

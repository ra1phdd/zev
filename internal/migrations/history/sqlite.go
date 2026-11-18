package history

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/ra1phdd/zev/internal/entities/history/current"
	"github.com/ra1phdd/zev/internal/entities/history/service"
)

//go:embed 1_history.up.sql
var migrationFiles embed.FS

// migrateSQLite creates the released history schema in an empty database. Existing
// databases are accepted only when they have the current schema. The caller
// uses immediate write transactions to serialize concurrent initializers.
func migrateSQLite(ctx context.Context, db *sql.DB) (err error) {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("migrate history: %w", err)
	}
	state, err := inspect(ctx, db)
	if err != nil {
		return err
	}
	if state == currentState {
		return nil
	}
	if state != emptyState {
		return fmt.Errorf("%w: history database is not an empty release database", service.ErrUnsupportedSchema)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin history baseline: %w", err)
	}
	defer func() {
		if rollbackErr := tx.Rollback(); rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
			err = errors.Join(err, rollbackErr)
		}
	}()
	// Another initializer may have committed while we waited for the writer lock.
	var empty bool
	if err := tx.QueryRowContext(ctx, `SELECT NOT EXISTS(SELECT 1 FROM sqlite_schema WHERE name NOT LIKE 'sqlite_%')`).Scan(&empty); err != nil {
		return fmt.Errorf("check history database under writer lock: %w", err)
	}
	if !empty {
		if err := tx.Rollback(); err != nil {
			return fmt.Errorf("release history bootstrap transaction: %w", err)
		}
		if err := current.Validate(ctx, db); err != nil {
			return fmt.Errorf("validate concurrently initialized history: %w", err)
		}
		return nil
	}
	if err := applyBaseline(ctx, tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit history baseline: %w", err)
	}
	if err := current.Validate(ctx, db); err != nil {
		return fmt.Errorf("validate history baseline: %w", err)
	}
	return nil
}

func applyBaseline(ctx context.Context, tx *sql.Tx) (err error) {
	if _, err := tx.ExecContext(ctx, `CREATE TABLE schema_migrations (version INTEGER NOT NULL, dirty INTEGER NOT NULL); CREATE UNIQUE INDEX version_unique ON schema_migrations(version);`); err != nil {
		return fmt.Errorf("create history migration ledger: %w", err)
	}
	source, err := iofs.New(migrationFiles, ".")
	if err != nil {
		return fmt.Errorf("open history migration source: %w", err)
	}
	driver := &transactionDriver{ctx: ctx, tx: tx}
	migrator, err := migrate.NewWithInstance("history", source, "sqlite3", driver)
	if err != nil {
		return errors.Join(fmt.Errorf("initialize history migration: %w", err), source.Close())
	}
	defer func() {
		sourceErr, driverErr := migrator.Close()
		err = errors.Join(err, sourceErr, driverErr)
	}()
	if err := migrator.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("apply history baseline: %w", err)
	}
	return nil
}

type databaseState uint8

const (
	emptyState databaseState = iota
	currentState
	unknownState
)

func inspect(ctx context.Context, db *sql.DB) (databaseState, error) {
	var ledger, empty bool
	if err := db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM sqlite_schema WHERE type='table' AND name='schema_migrations'), NOT EXISTS(SELECT 1 FROM sqlite_schema WHERE name NOT LIKE 'sqlite_%')`).Scan(&ledger, &empty); err != nil {
		return unknownState, fmt.Errorf("inspect history migration ledger: %w", err)
	}
	if !ledger {
		if empty {
			return emptyState, nil
		}
		return unknownState, nil
	}
	if err := current.Validate(ctx, db); err != nil {
		return unknownState, fmt.Errorf("validate current history schema: %w", err)
	}
	return currentState, nil
}

type transactionDriver struct {
	ctx context.Context
	tx  *sql.Tx
}

func (*transactionDriver) Open(string) (database.Driver, error) {
	return nil, errors.New("history migration cannot reopen")
}
func (*transactionDriver) Close() error  { return nil }
func (*transactionDriver) Drop() error   { return errors.New("history migration cannot drop") }
func (*transactionDriver) Lock() error   { return nil }
func (*transactionDriver) Unlock() error { return nil }
func (d *transactionDriver) Version() (int, bool, error) {
	var version int
	var dirty bool
	err := d.tx.QueryRowContext(d.ctx, `SELECT version,dirty FROM schema_migrations LIMIT 1`).Scan(&version, &dirty)
	if errors.Is(err, sql.ErrNoRows) {
		return database.NilVersion, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("read history migration ledger: %w", err)
	}
	return version, dirty, nil
}

func (d *transactionDriver) SetVersion(version int, dirty bool) error {
	_, err := d.tx.ExecContext(d.ctx, `DELETE FROM schema_migrations; INSERT INTO schema_migrations(version,dirty) VALUES(?,?)`, version, dirty)
	if err != nil {
		return fmt.Errorf("record history migration version: %w", err)
	}
	return nil
}

func (d *transactionDriver) Run(r io.Reader) error {
	b, err := io.ReadAll(r)
	if err != nil {
		return fmt.Errorf("read history baseline: %w", err)
	}
	_, err = d.tx.ExecContext(d.ctx, string(b))
	if err != nil {
		return fmt.Errorf("execute history baseline: %w", err)
	}
	return nil
}

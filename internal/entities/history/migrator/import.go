// Package migrator imports legacy JSONL history into SQLite.
package migrator

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/ra1phdd/zev/internal/core/domain"
	"github.com/ra1phdd/zev/internal/entities/history/repository"
	"github.com/ra1phdd/zev/internal/entities/history/service"
)

const (
	migrationStatusVerified = "verified"
	initialPromptKind       = "initial_prompt"
	assistantRole           = "assistant"
)

func migrationStamp(t time.Time) string {
	if t.IsZero() {
		t = time.Now()
	}
	return t.UTC().Format("2006-01-02T15:04:05.000000000Z")
}

type legacyEntry struct {
	Prompt           string                  `json:"prompt"`
	Response         *domain.OptionsResponse `json:"response"`
	Provider         string                  `json:"provider"`
	Model            string                  `json:"model"`
	Timestamp        time.Time               `json:"timestamp"`
	SelectedCommand  string                  `json:"selected_command"` //nolint:tagliatelle // Persisted legacy JSON contract requires snake_case field names.
	SelectedIndex    int                     `json:"selected_index"`   //nolint:tagliatelle // Persisted legacy JSON contract requires snake_case field names.
	selectedIndexSet bool
	selectedOrdinal  int
}

type legacySnapshot struct {
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
	Mtime  string `json:"mtime"`
}

func (p legacySnapshot) fingerprint() string {
	return fmt.Sprintf("%s:%d:%s", p.SHA256, p.Size, p.Mtime)
}

type legacyRunMetadata struct {
	FormatVersion int `json:"format_version"` //nolint:tagliatelle // Persisted legacy JSON contract requires snake_case field names.
	legacySnapshot
	ArchivedPath      string `json:"archived_path"`               //nolint:tagliatelle // Persisted legacy JSON contract requires snake_case field names.
	ArchiveSHA256     string `json:"archive_sha256,omitempty"`    //nolint:tagliatelle // Persisted legacy JSON contract requires snake_case field names.
	ArchiveSize       int64  `json:"archive_size,omitempty"`      //nolint:tagliatelle // Persisted legacy JSON contract requires snake_case field names.
	ArchiveMtime      string `json:"archive_mtime,omitempty"`     //nolint:tagliatelle // Persisted legacy JSON contract requires snake_case field names.
	LineCount         int    `json:"line_count"`                  //nolint:tagliatelle // Persisted legacy JSON contract requires snake_case field names.
	ConversationCount int    `json:"imported_conversation_count"` //nolint:tagliatelle // Persisted legacy JSON contract requires snake_case field names.
	SkippedCount      int    `json:"skipped_count"`               //nolint:tagliatelle // Persisted legacy JSON contract requires snake_case field names.
	ErrorCount        int    `json:"error_count"`                 //nolint:tagliatelle // Persisted legacy JSON contract requires snake_case field names.
}

type legacyRun struct {
	ID, Fingerprint, Status string
	Metadata                legacyRunMetadata
}

func (m *migrator) importLegacyLocked(ctx context.Context, source string) (err error) {
	file, openErr := openLegacySource(source)
	if errors.Is(openErr, os.ErrNotExist) {
		return m.recoverMissingLegacy(ctx, source)
	}
	if openErr != nil {
		return fmt.Errorf("open legacy history: %w", openErr)
	}
	defer func() { err = errors.Join(err, file.Close()) }()
	return m.importLegacyFile(ctx, file, source)
}

func (m *migrator) recoverMissingLegacy(ctx context.Context, source string) error {
	var run legacyRun
	found, err := m.readUnfinishedLegacyRun(ctx, &run)
	if err != nil {
		return err
	}
	if !found {
		return nil
	}
	if run.Metadata.ArchivedPath == "" {
		return fmt.Errorf("%w: unfinished migration has no archive path", service.ErrConflict)
	}
	if err := m.validateLegacyRun(ctx, run, source); err != nil {
		return err
	}
	if err := m.completeLegacyRun(ctx, run); err != nil {
		return fmt.Errorf("recover legacy migration: %w", err)
	}
	return nil
}

func (m *migrator) importLegacyFile(ctx context.Context, file *os.File, source string) error {
	snapshot, err := fingerprintLegacy(ctx, file)
	if err != nil {
		return err
	}
	if err := verifyLegacyIdentity(file, source); err != nil {
		return err
	}
	completed, err := m.completedLegacyFingerprint(ctx)
	if err != nil {
		return err
	}
	if completed != "" {
		if completed != snapshot.fingerprint() {
			return fmt.Errorf("%w: completed legacy migration has a different source fingerprint", service.ErrConflict)
		}
		return nil
	}
	var unfinished legacyRun
	found, err := m.readUnfinishedLegacyRun(ctx, &unfinished)
	if err != nil {
		return err
	}
	if found && unfinished.Fingerprint == snapshot.fingerprint() && unfinished.Status == migrationStatusVerified {
		if err := m.validateLegacyRun(ctx, unfinished, source); err != nil {
			return err
		}
		if err := renameExclusive(source, unfinished.Metadata.ArchivedPath); err != nil {
			return err
		}
		return m.completeLegacyRun(ctx, unfinished)
	}
	if found {
		if err := m.discardLegacyRun(ctx, unfinished.ID); err != nil {
			return err
		}
	}
	run := newLegacyRun(snapshot, source)
	if err := m.importLegacySnapshot(ctx, file, source, &run); err != nil {
		return err
	}
	if err := renameExclusive(source, run.Metadata.ArchivedPath); err != nil {
		return err
	}
	return m.completeLegacyRun(ctx, run)
}

func (m *migrator) completedLegacyFingerprint(ctx context.Context) (string, error) {
	var fingerprint string
	err := m.db.QueryRowContext(ctx, `SELECT source_fingerprint FROM migration_runs WHERE status='completed' LIMIT 1`).Scan(&fingerprint)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read legacy migration ledger: %w", err)
	}
	return fingerprint, nil
}

func newLegacyRun(snapshot legacySnapshot, source string) legacyRun {
	return legacyRun{ID: uuid.NewString(), Fingerprint: snapshot.fingerprint(), Metadata: legacyRunMetadata{
		FormatVersion: 1, legacySnapshot: snapshot,
		ArchivedPath: source + ".migrated." + snapshot.SHA256[:12] + "." + time.Now().UTC().Format("20060102T150405.000000000") + "." + uuid.NewString() + ".jsonl",
	}}
}

func (m *migrator) readUnfinishedLegacyRun(ctx context.Context, run *legacyRun) (bool, error) {
	var metadata string
	err := m.db.QueryRowContext(ctx, `SELECT id,source_fingerprint,status,COALESCE(metadata_json,'') FROM migration_runs WHERE status<>'completed' ORDER BY started_at DESC LIMIT 1`).Scan(&run.ID, &run.Fingerprint, &run.Status, &metadata)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read unfinished legacy migration: %w", err)
	}
	if err := json.Unmarshal([]byte(metadata), &run.Metadata); err != nil {
		return false, fmt.Errorf("decode unfinished legacy migration: %w", err)
	}
	return true, nil
}

func (m *migrator) validateLegacyRun(ctx context.Context, run legacyRun, source string) error {
	archive, err := filepath.Abs(run.Metadata.ArchivedPath)
	if err != nil {
		return fmt.Errorf("%w: invalid migration archive path", service.ErrConflict)
	}
	source, err = filepath.Abs(source)
	if err != nil || filepath.Dir(archive) != filepath.Dir(source) || !strings.HasPrefix(filepath.Base(archive), filepath.Base(source)+".migrated.") {
		return fmt.Errorf("%w: migration archive path is outside the legacy store", service.ErrConflict)
	}
	if run.Status != migrationStatusVerified {
		return fmt.Errorf("%w: unfinished migration is not verified", service.ErrConflict)
	}
	var lineCount, conversationCount int
	if err := m.db.QueryRowContext(ctx, `SELECT count(*) FROM migration_lines WHERE run_id=?`, run.ID).Scan(&lineCount); err != nil {
		return fmt.Errorf("verify migration lines: %w", err)
	}
	if err := m.db.QueryRowContext(ctx, `SELECT count(*) FROM conversations WHERE id IN (SELECT conversation_id FROM migration_lines WHERE run_id=?)`, run.ID).Scan(&conversationCount); err != nil {
		return fmt.Errorf("verify migration conversations: %w", err)
	}
	if lineCount != run.Metadata.LineCount || conversationCount != run.Metadata.ConversationCount || lineCount != conversationCount {
		return fmt.Errorf("%w: unfinished migration count mismatch", service.ErrConflict)
	}
	return nil
}

func (m *migrator) discardLegacyRun(ctx context.Context, runID string) (err error) {
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin legacy recovery: %w", err)
	}
	defer rollbackMigration(&err, tx)
	if _, err := tx.ExecContext(ctx, `DELETE FROM conversations WHERE id IN (SELECT conversation_id FROM migration_lines WHERE run_id=?)`, runID); err != nil {
		return fmt.Errorf("discard unfinished legacy records: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM migration_runs WHERE id=?`, runID); err != nil {
		return fmt.Errorf("discard unfinished legacy ledger: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit legacy recovery: %w", err)
	}
	return nil
}

func fingerprintLegacy(ctx context.Context, file *os.File) (legacySnapshot, error) {
	var snapshot legacySnapshot
	before, err := file.Stat()
	if err != nil {
		return snapshot, fmt.Errorf("stat legacy history: %w", err)
	}
	if !before.Mode().IsRegular() {
		return snapshot, fmt.Errorf("%w: legacy source is not regular", service.ErrInvalid)
	}
	if err := secureLegacyFile(file); err != nil {
		return snapshot, err
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return snapshot, fmt.Errorf("rewind legacy history: %w", err)
	}
	hash := sha256.New()
	buffer := make([]byte, 64*1024)
	var size int64
	for {
		if err := ctx.Err(); err != nil {
			return snapshot, fmt.Errorf("hash legacy snapshot: %w", err)
		}
		n, readErr := file.Read(buffer)
		if n > 0 {
			_, _ = hash.Write(buffer[:n])
			size += int64(n)
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return snapshot, fmt.Errorf("hash legacy history: %w", readErr)
		}
	}
	after, err := file.Stat()
	if err != nil {
		return snapshot, fmt.Errorf("stat legacy history: %w", err)
	}
	if size != before.Size() || after.Size() != before.Size() || !after.ModTime().Equal(before.ModTime()) {
		return snapshot, fmt.Errorf("%w: legacy source changed; retry migration", service.ErrConflict)
	}
	return legacySnapshot{SHA256: hex.EncodeToString(hash.Sum(nil)), Size: size, Mtime: before.ModTime().UTC().Format(time.RFC3339Nano)}, nil
}

func verifyLegacyIdentity(file *os.File, path string) error {
	opened, err := file.Stat()
	if err != nil {
		return fmt.Errorf("stat open legacy history: %w", err)
	}
	if err := validateLegacyFileInfo(opened); err != nil {
		return err
	}
	named, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("stat legacy history path: %w", err)
	}
	if err := validateLegacyFileInfo(named); err != nil {
		return err
	}
	if !os.SameFile(opened, named) {
		return fmt.Errorf("%w: legacy source changed; retry migration", service.ErrConflict)
	}
	return nil
}

func parseLegacyEntry(line []byte) (legacyEntry, string) {
	var entry legacyEntry
	trimmed := bytes.TrimSpace(line)
	if !utf8.Valid(line) || len(trimmed) == 0 || trimmed[0] != '{' || json.Unmarshal(line, &entry) != nil || entry.Response == nil {
		return entry, "invalid_json"
	}
	if err := entry.Response.Validate(); err != nil {
		return entry, "invalid_response"
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(line, &fields); err != nil {
		return entry, "invalid_json"
	}
	_, entry.selectedIndexSet = fields["selected_index"]
	entry.selectedOrdinal = -1
	for i, command := range entry.Response.Commands {
		if entry.SelectedCommand == "" || command.Command != entry.SelectedCommand {
			continue
		}
		if entry.selectedOrdinal >= 0 {
			return entry, "ambiguous_selection"
		}
		entry.selectedOrdinal = i
	}
	if !entry.selectedIndexSet {
		return entry, ""
	}
	if entry.SelectedIndex < 0 || entry.SelectedIndex >= len(entry.Response.Commands) {
		return entry, "selection_index"
	}
	if entry.selectedOrdinal >= 0 && entry.selectedOrdinal != entry.SelectedIndex {
		return entry, "selection_mismatch"
	}
	entry.selectedOrdinal = entry.SelectedIndex
	return entry, ""
}

func importLegacyLines(ctx context.Context, tx *sql.Tx, file *os.File, run *legacyRun) error {
	reader := bufio.NewReader(file)
	hash := sha256.New()
	var size int64
	for {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("read legacy lines: %w", err)
		}
		line, readErr := reader.ReadBytes('\n')
		if len(line) == 0 && errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			return fmt.Errorf("read legacy history: %w", readErr)
		}
		_, _ = hash.Write(line)
		size += int64(len(line))
		line = bytes.TrimSuffix(bytes.TrimSuffix(line, []byte{'\n'}), []byte{'\r'})
		lineNumber := run.Metadata.LineCount + 1
		entry, code := parseLegacyEntry(line)
		if code != "" {
			return fmt.Errorf("%w: legacy migration line %d code=%s processed=%d", service.ErrInvalid, lineNumber, code, run.Metadata.ConversationCount)
		}
		if err := insertLegacyEntry(ctx, tx, legacyInsert{Entry: entry, RunID: run.ID, LineNumber: lineNumber, Line: line}); err != nil {
			return err
		}
		run.Metadata.LineCount++
		run.Metadata.ConversationCount++
		if errors.Is(readErr, io.EOF) {
			break
		}
	}
	if size != run.Metadata.Size || hex.EncodeToString(hash.Sum(nil)) != run.Metadata.SHA256 {
		return fmt.Errorf("%w: legacy source changed; retry migration", service.ErrConflict)
	}
	return nil
}

func (m *migrator) importLegacySnapshot(ctx context.Context, file *os.File, source string, run *legacyRun) (err error) {
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin legacy migration: %w", err)
	}
	defer rollbackMigration(&err, tx)
	metadata, err := json.Marshal(run.Metadata)
	if err != nil {
		return fmt.Errorf("encode migration metadata: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO migration_runs(id,source_fingerprint,status,started_at,metadata_json) VALUES(?,?,'running',?,?)`, run.ID, run.Fingerprint, migrationStamp(time.Now()), string(metadata)); err != nil {
		return fmt.Errorf("record legacy migration: %w", err)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("rewind legacy history: %w", err)
	}
	if err := importLegacyLines(ctx, tx, file, run); err != nil {
		return err
	}
	after, err := fingerprintLegacy(ctx, file)
	if err != nil {
		return err
	}
	if after != run.Metadata.legacySnapshot {
		return fmt.Errorf("%w: legacy source changed; retry migration", service.ErrConflict)
	}
	if err := verifyLegacyIdentity(file, source); err != nil {
		return err
	}
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM migration_lines WHERE run_id=?`, run.ID).Scan(&count); err != nil {
		return fmt.Errorf("verify imported count: %w", err)
	}
	if count != run.Metadata.ConversationCount {
		return fmt.Errorf("%w: legacy migration count mismatch", service.ErrConflict)
	}
	metadata, err = json.Marshal(run.Metadata)
	if err != nil {
		return fmt.Errorf("encode migration metadata: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE migration_runs SET status='verified',metadata_json=? WHERE id=?`, string(metadata), run.ID); err != nil {
		return fmt.Errorf("verify migration ledger: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit legacy migration: %w", err)
	}
	run.Status = migrationStatusVerified
	return nil
}

type legacyInsert struct {
	Entry      legacyEntry
	RunID      string
	LineNumber int
	Line       []byte
}

func insertLegacyEntry(ctx context.Context, tx *sql.Tx, in legacyInsert) error {
	entry := in.Entry
	conversationID := uuid.NewString()
	created := migrationStamp(entry.Timestamp)
	hash := sha256.Sum256(in.Line)
	metadata, err := json.Marshal(struct {
		Source          string `json:"source"`
		SourceLine      int    `json:"source_line"`      //nolint:tagliatelle // Persisted legacy provenance uses the specified snake_case contract.
		LineFingerprint string `json:"line_fingerprint"` //nolint:tagliatelle // Persisted legacy provenance uses the specified snake_case contract.
	}{"legacy_jsonl", in.LineNumber, hex.EncodeToString(hash[:])})
	if err != nil {
		return fmt.Errorf("encode legacy provenance: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO conversations(id,created_at,updated_at,status,kind,provider,model,metadata_json) VALUES(?,?,?,'completed','query',?,?,?)`, conversationID, created, created, entry.Provider, entry.Model, string(metadata)); err != nil {
		return fmt.Errorf("insert legacy conversation: %w", err)
	}
	const userRole = "user"
	if _, err := repository.AppendMessage(ctx, tx, service.Message{ID: uuid.NewString(), ConversationID: conversationID, Role: userRole, Kind: initialPromptKind, Content: entry.Prompt, CreatedAt: entry.Timestamp}); err != nil {
		return fmt.Errorf("insert legacy prompt: %w", err)
	}
	if err := insertLegacyResponse(ctx, tx, entry, conversationID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO migration_lines(run_id,line_number,conversation_id) VALUES(?,?,?)`, in.RunID, in.LineNumber, conversationID); err != nil {
		return fmt.Errorf("record legacy line: %w", err)
	}
	return nil
}

func insertLegacyResponse(ctx context.Context, tx *sql.Tx, entry legacyEntry, conversationID string) error {
	if entry.Response == nil {
		return fmt.Errorf("%w: missing legacy response", service.ErrInvalid)
	}
	assistantID, responseID := uuid.NewString(), uuid.NewString()
	created := migrationStamp(entry.Timestamp)
	payload, err := json.Marshal(entry.Response)
	if err != nil {
		return fmt.Errorf("encode legacy response: %w", err)
	}
	if _, err := repository.AppendMessage(ctx, tx, service.Message{ID: assistantID, ConversationID: conversationID, Role: assistantRole, Kind: "response", Content: string(payload), CreatedAt: entry.Timestamp}); err != nil {
		return fmt.Errorf("insert legacy assistant: %w", err)
	}
	const assistantDocumentKind = "assistant_message"
	if text := strings.TrimSpace(entry.Response.ExplanationIfNotValid); text != "" {
		if err := repository.InsertSearchDocument(ctx, tx, repository.SearchDocument{ConversationID: conversationID, MessageID: assistantID, Kind: assistantDocumentKind, Content: text, Created: created}); err != nil {
			return fmt.Errorf("index legacy assistant: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO responses(id,conversation_id,message_id,provider,model,payload_json,is_valid,created_at) VALUES(?,?,?,?,?,?,?,?)`, responseID, conversationID, assistantID, entry.Provider, entry.Model, string(payload), entry.Response.IsValid, created); err != nil {
		return fmt.Errorf("insert legacy response: %w", err)
	}
	for i, command := range entry.Response.Commands {
		in := legacyCommandInsert{Command: command, ConversationID: conversationID, ResponseID: responseID, Ordinal: i, Created: created, Selected: entry.selectedOrdinal == i}
		if err := insertLegacyCommand(ctx, tx, in); err != nil {
			return err
		}
	}
	return nil
}

type legacyCommandInsert struct {
	Command                             domain.Command
	ConversationID, ResponseID, Created string
	Ordinal                             int
	Selected                            bool
}

func insertLegacyCommand(ctx context.Context, tx *sql.Tx, in legacyCommandInsert) error {
	id := uuid.NewString()
	var selectedAt any
	if in.Selected {
		selectedAt = in.Created
	}
	command := in.Command
	if _, err := tx.ExecContext(ctx, `INSERT INTO commands(id,response_id,conversation_id,ordinal,command,short_explanation,is_dangerous,dangerous_explanation,selected_at) VALUES(?,?,?,?,?,?,?,?,?)`, id, in.ResponseID, in.ConversationID, in.Ordinal, command.Command, command.ShortExplanation, command.IsDangerous, command.DangerousExplanation, selectedAt); err != nil {
		return fmt.Errorf("insert legacy command: %w", err)
	}
	if err := repository.InsertSearchDocument(ctx, tx, repository.SearchDocument{ConversationID: in.ConversationID, CommandID: id, Kind: "command", Content: command.Command, Created: in.Created}); err != nil {
		return fmt.Errorf("index legacy command: %w", err)
	}
	explanation := strings.TrimSpace(strings.Join([]string{command.ShortExplanation, command.DangerousExplanation}, "\n"))
	if explanation != "" {
		if err := repository.InsertSearchDocument(ctx, tx, repository.SearchDocument{ConversationID: in.ConversationID, CommandID: id, Kind: "explanation", Content: explanation, Created: in.Created}); err != nil {
			return fmt.Errorf("index legacy explanation: %w", err)
		}
	}
	if !in.Selected {
		return nil
	}
	if _, err := tx.ExecContext(ctx, `UPDATE conversations SET selected_command_id=? WHERE id=?`, id, in.ConversationID); err != nil {
		return fmt.Errorf("select legacy command: %w", err)
	}
	return nil
}

func (m *migrator) completeLegacyRun(ctx context.Context, run legacyRun) (err error) {
	archive, err := openLegacySource(run.Metadata.ArchivedPath)
	if err != nil {
		return fmt.Errorf("open migration archive: %w", err)
	}
	defer func() { err = errors.Join(err, archive.Close()) }()
	if err := verifyLegacyIdentity(archive, run.Metadata.ArchivedPath); err != nil {
		return fmt.Errorf("verify migration archive identity: %w", err)
	}
	if err := secureLegacyFile(archive); err != nil {
		return fmt.Errorf("secure migration archive: %w", err)
	}
	snapshot, err := fingerprintLegacy(ctx, archive)
	if err != nil {
		return err
	}
	if snapshot.SHA256 != run.Metadata.SHA256 || snapshot.Size != run.Metadata.Size {
		return fmt.Errorf("%w: legacy archive fingerprint mismatch", service.ErrConflict)
	}
	if err := verifyLegacyIdentity(archive, run.Metadata.ArchivedPath); err != nil {
		return fmt.Errorf("verify migration archive identity: %w", err)
	}
	if err := syncArchiveDirectory(run.Metadata.ArchivedPath); err != nil {
		return fmt.Errorf("sync migration archive directory: %w", err)
	}
	run.Metadata.ArchiveSHA256 = snapshot.SHA256
	run.Metadata.ArchiveSize = snapshot.Size
	run.Metadata.ArchiveMtime = snapshot.Mtime
	metadata, err := json.Marshal(run.Metadata)
	if err != nil {
		return fmt.Errorf("encode completed migration metadata: %w", err)
	}
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin legacy activation: %w", err)
	}
	defer rollbackMigration(&err, tx)
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM migration_lines WHERE run_id=?`, run.ID).Scan(&count); err != nil {
		return fmt.Errorf("verify resumed migration count: %w", err)
	}
	if count != run.Metadata.ConversationCount {
		return fmt.Errorf("%w: resumed migration count mismatch", service.ErrConflict)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE migration_runs SET status='completed',completed_at=?,metadata_json=? WHERE id=?`, migrationStamp(time.Now()), string(metadata), run.ID); err != nil {
		return fmt.Errorf("activate legacy migration: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit legacy activation: %w", err)
	}
	return nil
}

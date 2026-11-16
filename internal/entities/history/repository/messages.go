package repository

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/ra1phdd/zev/internal/entities/history/service"
)

const (
	startedStatus       = "started"
	userRole            = "user"
	responseKind        = "response"
	refinementKind      = "refinement"
	commandDocument     = "command"
	firstCommandFixture = "cmd1"
	completedStatus     = "completed"
	failedStatus        = "failed"
	readyEmbedding      = "ready"
	userDocument        = "user_message"
	fuzzyAlways         = "always"
	fuzzyOff            = "off"
	morphologyTable     = "history_fts_morphology"
	filesFixture        = "files"
	modelFixture        = "model"
	wantedFixture       = "wanted"
	commonFixture       = "common"
)

func historyError(err error) error {
	if err == nil {
		return nil
	}
	return classifyStorageError(err)
}

var _ service.Store = (*Store)(nil)

func stamp(t time.Time) string {
	if t.IsZero() {
		t = time.Now()
	}
	return t.UTC().Format("2006-01-02T15:04:05.000000000Z")
}

func optional(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func parseTime(s string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339Nano, s)
	return t, historyError(err)
}

func missing(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return service.ErrNotFound
	}
	return historyError(err)
}

type queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func getMessage(ctx context.Context, q queryer, id string) (service.Message, error) {
	var m service.Message
	var created string
	err := q.QueryRowContext(ctx, `SELECT id,conversation_id,sequence,role,kind,content,created_at,COALESCE(metadata_json,''),COALESCE(parts_json,'') FROM messages WHERE id=?`, id).Scan(&m.ID, &m.ConversationID, &m.Sequence, &m.Role, &m.Kind, &m.Content, &created, &m.MetadataJSON, &m.PartsJSON)
	if err != nil {
		return m, missing(err)
	}
	m.CreatedAt, err = parseTime(created)
	return m, historyError(err)
}

func appendMessage(ctx context.Context, tx *sql.Tx, m service.Message) (service.Message, error) {
	if m.ID == "" || m.ConversationID == "" {
		return m, fmt.Errorf("%w: message identifiers required", service.ErrInvalid)
	}
	existing, err := getMessage(ctx, tx, m.ID)
	if err == nil {
		if existing.ConversationID != m.ConversationID || existing.Role != m.Role || existing.Kind != m.Kind || existing.Content != m.Content || existing.MetadataJSON != m.MetadataJSON || existing.PartsJSON != m.PartsJSON || (!m.CreatedAt.IsZero() && !existing.CreatedAt.Equal(m.CreatedAt)) {
			return m, fmt.Errorf("%w: message ID already used", service.ErrConflict)
		}
		return existing, nil
	}
	if !errors.Is(err, service.ErrNotFound) {
		return m, historyError(err)
	}
	var seq int64
	if err = tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(sequence),-1)+1 FROM messages WHERE conversation_id=?`, m.ConversationID).Scan(&seq); err != nil {
		return m, historyError(err)
	}
	m.Sequence = seq
	created := stamp(m.CreatedAt)
	m.CreatedAt, err = parseTime(created)
	if err != nil {
		return m, historyError(err)
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO messages(id,conversation_id,sequence,role,kind,content,created_at,metadata_json,parts_json) VALUES(?,?,?,?,?,?,?,?,?)`, m.ID, m.ConversationID, m.Sequence, m.Role, m.Kind, m.Content, created, optional(m.MetadataJSON), optional(m.PartsJSON))
	if err != nil {
		return m, fmt.Errorf("insert message: %w", err)
	}
	if m.Role == userRole && (m.Kind == initialPromptKind || m.Kind == refinementKind) {
		res, e := tx.ExecContext(ctx, `INSERT INTO history_documents(conversation_id,message_id,kind,content) VALUES(?,?,?,?)`, m.ConversationID, m.ID, userDocument, m.Content)
		if e != nil {
			return m, historyError(e)
		}
		id, e := res.LastInsertId()
		if e != nil {
			return m, historyError(e)
		}
		if e = indexDocument(ctx, tx, id, m.Content, created); e != nil {
			return m, historyError(e)
		}
	}
	_, err = tx.ExecContext(ctx, `UPDATE conversations SET updated_at=? WHERE id=?`, created, m.ConversationID)
	return m, historyError(err)
}

// StartConversation atomically creates a conversation and its first user message.
func (s *Store) StartConversation(ctx context.Context, in service.StartConversationInput) (result service.Conversation, err error) {
	c := in.Conversation
	m := in.InitialMessage
	if c.ID == "" || m.ID == "" || (m.ConversationID != "" && m.ConversationID != c.ID) || m.Role != userRole || m.Kind != initialPromptKind {
		return c, fmt.Errorf("%w: initial conversation", service.ErrInvalid)
	}
	if c.Kind == "" {
		c.Kind = "query"
	}
	if c.Status == "" {
		c.Status = startedStatus
	}
	if c.Status != startedStatus {
		return c, fmt.Errorf("%w: initial status", service.ErrInvalid)
	}
	m.ConversationID = c.ID
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return c, historyError(err)
	}
	defer rollbackHistory(tx, &err)
	old, err := getConversation(ctx, tx, c.ID)
	if err == nil {
		if err = retryCreation(ctx, tx, in, old); err != nil {
			return c, historyError(err)
		}
		if err = tx.Commit(); err != nil {
			return c, fmt.Errorf("commit conversation retry: %w", err)
		}
		return old, nil
	}
	if !errors.Is(err, service.ErrNotFound) {
		return c, historyError(err)
	}
	created := stamp(c.CreatedAt)
	c.CreatedAt, err = parseTime(created)
	if err != nil {
		return c, historyError(err)
	}
	c.UpdatedAt = c.CreatedAt
	_, err = tx.ExecContext(ctx, `INSERT INTO conversations(id,created_at,updated_at,status,kind,provider,model,metadata_json) VALUES(?,?,?,?,?,?,?,?)`, c.ID, created, created, c.Status, c.Kind, c.Provider, c.Model, optional(c.MetadataJSON))
	if err != nil {
		return c, fmt.Errorf("insert conversation: %w", err)
	}
	if m.CreatedAt.IsZero() {
		m.CreatedAt = c.CreatedAt
	}
	m, err = appendMessage(ctx, tx, m)
	if err != nil {
		return c, historyError(err)
	}
	if err = recordCreation(ctx, tx, in, m.ID); err != nil {
		return c, err
	}
	c, err = getConversation(ctx, tx, c.ID)
	if err != nil {
		return c, historyError(err)
	}
	if err = tx.Commit(); err != nil {
		return c, historyError(err)
	}
	return c, nil
}

func recordCreation(ctx context.Context, tx *sql.Tx, in service.StartConversationInput, messageID string) error {
	fingerprint, err := creationFingerprint(in.Conversation, in.InitialMessage)
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO history_writes(conversation_id,initial_message_id,fingerprint) VALUES(?,?,?)`, in.Conversation.ID, messageID, fingerprint); err != nil {
		return historyError(err)
	}
	return nil
}

func inputTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return stamp(t)
}

func creationFingerprint(c service.Conversation, m service.Message) (string, error) {
	if c.Kind == "" {
		c.Kind = "query"
	}
	if c.Status == "" {
		c.Status = startedStatus
	}
	payload, err := json.Marshal(struct {
		ConversationID, Kind, Status, Provider, Model, MetadataJSON, CreatedAt                  string
		MessageID, Role, MessageKind, Content, MessageMetadataJSON, PartsJSON, MessageCreatedAt string
	}{
		c.ID, c.Kind, c.Status, c.Provider, c.Model, c.MetadataJSON, inputTime(c.CreatedAt),
		m.ID, m.Role, m.Kind, m.Content, m.MetadataJSON, m.PartsJSON, inputTime(m.CreatedAt),
	})
	if err != nil {
		return "", historyError(err)
	}
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:]), nil
}

func retryCreation(ctx context.Context, tx *sql.Tx, in service.StartConversationInput, old service.Conversation) error {
	m := in.InitialMessage
	m.ConversationID = in.Conversation.ID
	want, err := creationFingerprint(in.Conversation, m)
	if err != nil {
		return err
	}
	var got, messageID string
	err = tx.QueryRowContext(ctx, `SELECT fingerprint,initial_message_id FROM history_writes WHERE conversation_id=?`, old.ID).Scan(&got, &messageID)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%w: conversation creation ledger missing", service.ErrConflict)
	}
	if err != nil {
		return historyError(err)
	}
	if got != want || messageID != m.ID {
		return fmt.Errorf("%w: conversation ID already used", service.ErrConflict)
	}
	_, err = appendMessage(ctx, tx, m)
	return historyError(err)
}

// AppendMessage appends a message, returning the original on an identical retry.
func (s *Store) AppendMessage(ctx context.Context, in service.AppendMessageInput) (result service.Message, err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return service.Message{}, historyError(err)
	}
	defer rollbackHistory(tx, &err)
	m, err := appendMessage(ctx, tx, in.Message)
	if err != nil {
		return m, historyError(err)
	}
	if err = tx.Commit(); err != nil {
		return m, historyError(err)
	}
	return m, nil
}

// GetMessage retrieves a message by its complete ID.
func (s *Store) GetMessage(ctx context.Context, id string) (service.Message, error) {
	return getMessage(ctx, s.db, id)
}

func getConversation(ctx context.Context, q queryer, id string) (service.Conversation, error) {
	var c service.Conversation
	var created, updated string
	err := q.QueryRowContext(ctx, `SELECT id,created_at,updated_at,status,kind,provider,model,COALESCE(selected_command_id,''),COALESCE(metadata_json,'') FROM conversations WHERE id=?`, id).Scan(&c.ID, &created, &updated, &c.Status, &c.Kind, &c.Provider, &c.Model, &c.SelectedCommandID, &c.MetadataJSON)
	if err != nil {
		return c, missing(err)
	}
	c.CreatedAt, err = parseTime(created)
	if err != nil {
		return c, historyError(err)
	}
	c.UpdatedAt, err = parseTime(updated)
	return c, historyError(err)
}

// CompleteConversation updates a conversation to a terminal status.
func (s *Store) CompleteConversation(ctx context.Context, in service.CompleteConversationInput) error {
	if in.Status != completedStatus && in.Status != "cancelled" && in.Status != failedStatus {
		return fmt.Errorf("%w: terminal status", service.ErrInvalid)
	}
	res, err := s.db.ExecContext(ctx, `UPDATE conversations SET status=?,updated_at=? WHERE id=?`, in.Status, stamp(in.CompletedAt), in.ConversationID)
	if err != nil {
		return historyError(err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return historyError(err)
	}
	if n == 0 {
		return service.ErrNotFound
	}
	return nil
}

// DeleteConversation removes the conversation and all dependent records.
func (s *Store) DeleteConversation(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM conversations WHERE id=?`, id)
	if err != nil {
		return historyError(err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return historyError(err)
	}
	if n == 0 {
		return service.ErrNotFound
	}
	return nil
}

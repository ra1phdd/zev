package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ra1phdd/zev/internal/core/domain"
	"github.com/ra1phdd/zev/internal/entities/history/service"
)

// RecordResponse stores an assistant message, structured response, and commands atomically.
func (s *Store) RecordResponse(ctx context.Context, in service.RecordResponseInput) (result service.Response, err error) {
	r := in.Response
	m := in.Message
	if r.ID == "" || r.ConversationID == "" || m.ID == "" || m.Role != assistantRole || m.Kind != responseKind || (m.ConversationID != "" && m.ConversationID != r.ConversationID) || (r.MessageID != "" && r.MessageID != m.ID) {
		return r, fmt.Errorf("%w: response identifiers or message", service.ErrInvalid)
	}
	m.ConversationID = r.ConversationID
	r.MessageID = m.ID
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return r, historyError(err)
	}
	defer rollbackHistory(tx, &err)
	old, err := getResponse(ctx, tx, r.ID)
	if err == nil {
		if err = retryResponse(ctx, tx, m, r, old); err != nil {
			return r, err
		}
		if err = tx.Commit(); err != nil {
			return r, historyError(err)
		}
		return old, nil
	}
	if !errors.Is(err, service.ErrNotFound) {
		return r, historyError(err)
	}
	if _, err = appendMessage(ctx, tx, m); err != nil {
		return r, historyError(err)
	}
	created := stamp(r.CreatedAt)
	r.CreatedAt, err = parseTime(created)
	if err != nil {
		return r, historyError(err)
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO responses(id,conversation_id,message_id,provider,model,payload_json,is_valid,created_at) VALUES(?,?,?,?,?,?,?,?)`, r.ID, r.ConversationID, r.MessageID, r.Provider, r.Model, r.PayloadJSON, r.IsValid, created)
	if err != nil {
		return r, fmt.Errorf("insert response: %w", err)
	}
	if err = indexAssistantResponse(ctx, tx, m); err != nil {
		return r, historyError(err)
	}
	for i := range r.Commands {
		if err = insertResponseCommand(ctx, tx, &r.Commands[i], r, i); err != nil {
			return r, historyError(err)
		}
	}
	_, err = tx.ExecContext(ctx, `UPDATE conversations SET provider=?,model=?,updated_at=? WHERE id=?`, r.Provider, r.Model, created, r.ConversationID)
	if err != nil {
		return r, historyError(err)
	}
	if err = tx.Commit(); err != nil {
		return r, historyError(err)
	}
	return r, nil
}

func retryResponse(ctx context.Context, tx *sql.Tx, m service.Message, r, old service.Response) error {
	if _, err := appendMessage(ctx, tx, m); err != nil {
		return err
	}
	if old.ConversationID != r.ConversationID || old.MessageID != r.MessageID || old.Provider != r.Provider || old.Model != r.Model || old.PayloadJSON != r.PayloadJSON || old.IsValid != r.IsValid || (!r.CreatedAt.IsZero() && !old.CreatedAt.Equal(r.CreatedAt)) || !sameCommands(old.Commands, r.Commands, r.ID, r.ConversationID) {
		return fmt.Errorf("%w: response ID already used", service.ErrConflict)
	}
	return nil
}

func indexAssistantResponse(ctx context.Context, tx *sql.Tx, m service.Message) error {
	text := m.Content
	var payload domain.OptionsResponse
	if json.Unmarshal([]byte(m.Content), &payload) == nil {
		text = payload.ExplanationIfNotValid
	}
	if strings.TrimSpace(text) == "" {
		return nil
	}
	return insertSearchDocument(ctx, tx, searchDocument{conversationID: m.ConversationID, messageID: m.ID, kind: "assistant_message", content: text, created: stamp(m.CreatedAt)})
}

func insertResponseCommand(ctx context.Context, tx *sql.Tx, c *service.Command, r service.Response, ordinal int) error {
	if c.ID == "" || (c.ResponseID != "" && c.ResponseID != r.ID) || (c.ConversationID != "" && c.ConversationID != r.ConversationID) {
		return fmt.Errorf("%w: command identifiers", service.ErrInvalid)
	}
	c.ResponseID, c.ConversationID, c.Ordinal = r.ID, r.ConversationID, ordinal
	_, err := tx.ExecContext(ctx, `INSERT INTO commands(id,response_id,conversation_id,ordinal,command,short_explanation,is_dangerous,dangerous_explanation,delivery_outcome) VALUES(?,?,?,?,?,?,?,?,?)`, c.ID, r.ID, r.ConversationID, ordinal, c.Command, c.ShortExplanation, c.IsDangerous, c.DangerousExplanation, "unknown")
	if err != nil {
		return fmt.Errorf("insert command: %w", err)
	}
	created := stamp(r.CreatedAt)
	if err = insertSearchDocument(ctx, tx, searchDocument{conversationID: r.ConversationID, commandID: c.ID, kind: commandDocument, content: c.Command, created: created}); err != nil {
		return historyError(err)
	}
	explanation := strings.TrimSpace(strings.Join([]string{c.ShortExplanation, c.DangerousExplanation}, "\n"))
	if explanation == "" {
		return nil
	}
	return insertSearchDocument(ctx, tx, searchDocument{conversationID: r.ConversationID, commandID: c.ID, kind: "explanation", content: explanation, created: created})
}

func sameCommands(old, input []service.Command, responseID, conversationID string) bool {
	if len(old) != len(input) {
		return false
	}
	for i := range old {
		a, b := old[i], input[i]
		if b.ResponseID != "" && b.ResponseID != responseID {
			return false
		}
		if b.ConversationID != "" && b.ConversationID != conversationID {
			return false
		}
		if a.ID != b.ID || a.Command != b.Command || a.ShortExplanation != b.ShortExplanation || a.DangerousExplanation != b.DangerousExplanation || a.IsDangerous != b.IsDangerous {
			return false
		}
	}
	return true
}

type dbQuery interface {
	queryer
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func getResponse(ctx context.Context, q dbQuery, id string) (r service.Response, err error) {
	var created string
	err = q.QueryRowContext(ctx, `SELECT id,conversation_id,message_id,provider,model,payload_json,is_valid,created_at FROM responses WHERE id=?`, id).Scan(&r.ID, &r.ConversationID, &r.MessageID, &r.Provider, &r.Model, &r.PayloadJSON, &r.IsValid, &created)
	if err != nil {
		return r, missing(err)
	}
	r.CreatedAt, err = parseTime(created)
	if err != nil {
		return r, historyError(err)
	}
	rows, err := q.QueryContext(ctx, `SELECT id,response_id,conversation_id,ordinal,command,short_explanation,is_dangerous,dangerous_explanation,selected_at,executed_at,execution_exit_code,execution_error,delivery_outcome FROM commands WHERE response_id=? ORDER BY ordinal`, id)
	if err != nil {
		return r, historyError(err)
	}
	defer closeHistoryRows(rows, &err)
	for rows.Next() {
		var c service.Command
		var selected, executed sql.NullString
		var exit sql.NullInt64
		var delivery string
		if err = rows.Scan(&c.ID, &c.ResponseID, &c.ConversationID, &c.Ordinal, &c.Command, &c.ShortExplanation, &c.IsDangerous, &c.DangerousExplanation, &selected, &executed, &exit, &c.ExecutionError, &delivery); err != nil {
			return r, historyError(err)
		}
		c.DeliveryOutcome = delivery
		if selected.Valid {
			t, e := parseTime(selected.String)
			if e != nil {
				return r, historyError(e)
			}
			c.SelectedAt = &t
		}
		if executed.Valid {
			t, e := parseTime(executed.String)
			if e != nil {
				return r, historyError(e)
			}
			c.ExecutedAt = &t
		}
		if exit.Valid {
			v := int(exit.Int64)
			c.ExecutionExitCode = &v
		}
		r.Commands = append(r.Commands, c)
	}
	return r, historyError(rows.Err())
}

const (
	selectedOutcome = "selected"
	unknownOutcome  = "unknown"
)

// RecordSelection records selection and optional execution only for a command in the conversation.
//
//nolint:gocognit // Transactional history orchestration keeps validation, retry and rollback together.
func (s *Store) RecordSelection(ctx context.Context, in service.RecordSelectionInput) (err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return historyError(err)
	}
	defer rollbackHistory(tx, &err)
	var owner, oldOutcome, oldError string
	var selected, oldExecuted sql.NullString
	var oldExit sql.NullInt64
	err = tx.QueryRowContext(ctx, `SELECT conversation_id,selected_at,executed_at,execution_exit_code,execution_error,delivery_outcome FROM commands WHERE id=?`, in.CommandID).Scan(&owner, &selected, &oldExecuted, &oldExit, &oldError, &oldOutcome)
	if err != nil {
		return missing(err)
	}
	if owner != in.ConversationID {
		return fmt.Errorf("%w: command belongs to another conversation", service.ErrInvalid)
	}
	var executed any
	if in.ExecutedAt != nil {
		executed = stamp(*in.ExecutedAt)
	}
	outcome := in.DeliveryOutcome
	if outcome == "" {
		outcome = selectedOutcome
	}
	if outcome != selectedOutcome && outcome != "injected" && outcome != "printed" && outcome != "executed" && outcome != failedStatus && outcome != unknownOutcome {
		return fmt.Errorf("%w: delivery outcome", service.ErrInvalid)
	}
	if selected.Valid && oldOutcome != selectedOutcome {
		if outcome == selectedOutcome || outcome == unknownOutcome && oldOutcome != unknownOutcome {
			return historyError(tx.Commit())
		}
		if oldOutcome != outcome && (oldOutcome != unknownOutcome || (outcome != "executed" && outcome != failedStatus)) {
			return fmt.Errorf("%w: delivery outcome already recorded", service.ErrConflict)
		}
	}
	if oldExecuted.Valid && (executed == nil || oldExecuted.String != executed) || oldExit.Valid && (in.ExecutionExitCode == nil || int64(*in.ExecutionExitCode) != oldExit.Int64) || oldError != "" && oldError != in.ExecutionError {
		return fmt.Errorf("%w: execution result already recorded", service.ErrConflict)
	}
	_, err = tx.ExecContext(ctx, `UPDATE commands SET selected_at=COALESCE(selected_at,?),executed_at=COALESCE(executed_at,?),execution_exit_code=COALESCE(execution_exit_code,?),execution_error=?,delivery_outcome=? WHERE id=?`, stamp(in.SelectedAt), executed, in.ExecutionExitCode, in.ExecutionError, outcome, in.CommandID)
	if err != nil {
		return historyError(err)
	}
	_, err = tx.ExecContext(ctx, `UPDATE conversations SET selected_command_id=?,updated_at=? WHERE id=?`, in.CommandID, stamp(in.SelectedAt), in.ConversationID)
	if err != nil {
		return historyError(err)
	}
	return historyError(tx.Commit())
}

// GetConversation reads a consistent snapshot of a conversation and its children.
func (s *Store) GetConversation(ctx context.Context, id string) (view service.ConversationView, err error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return view, historyError(err)
	}
	defer rollbackHistory(tx, &err)
	view.Conversation, err = getConversation(ctx, tx, id)
	if err != nil {
		return view, historyError(err)
	}
	rows, err := tx.QueryContext(ctx, `SELECT id FROM messages WHERE conversation_id=? ORDER BY sequence`, id)
	if err != nil {
		return view, historyError(err)
	}
	var ids []string
	for rows.Next() {
		var v string
		if err = rows.Scan(&v); err != nil {
			closeHistoryRows(rows, &err)
			return view, historyError(err)
		}
		ids = append(ids, v)
	}
	err = rows.Err()
	closeHistoryRows(rows, &err)
	if err != nil {
		return view, historyError(err)
	}
	for _, v := range ids {
		m, e := getMessage(ctx, tx, v)
		if e != nil {
			return view, historyError(e)
		}
		view.Messages = append(view.Messages, m)
	}
	rows, err = tx.QueryContext(ctx, `SELECT r.id FROM responses r JOIN messages m ON m.id=r.message_id WHERE r.conversation_id=? ORDER BY m.sequence`, id)
	if err != nil {
		return view, historyError(err)
	}
	ids = nil
	for rows.Next() {
		var v string
		if err = rows.Scan(&v); err != nil {
			closeHistoryRows(rows, &err)
			return view, historyError(err)
		}
		ids = append(ids, v)
	}
	err = rows.Err()
	closeHistoryRows(rows, &err)
	if err != nil {
		return view, historyError(err)
	}
	for _, v := range ids {
		r, e := getResponse(ctx, tx, v)
		if e != nil {
			return view, historyError(e)
		}
		view.Responses = append(view.Responses, r)
	}
	if err = tx.Commit(); err != nil {
		return view, historyError(err)
	}
	return view, nil
}

// LastUsedCommand returns the newest user-selected command whose delivery was
// observed as injected, printed, or executed successfully.
func (s *Store) LastUsedCommand(ctx context.Context) (service.Command, error) {
	var c service.Command
	var selected, executed sql.NullString
	var exit sql.NullInt64
	err := s.db.QueryRowContext(ctx, `
SELECT c.id,c.response_id,c.conversation_id,c.ordinal,c.command,c.short_explanation,c.is_dangerous,
       c.dangerous_explanation,c.selected_at,c.executed_at,c.execution_exit_code,c.execution_error,c.delivery_outcome
FROM commands c
JOIN conversations v ON v.id=c.conversation_id
WHERE v.selected_command_id=c.id AND v.status='completed'
  AND c.selected_at IS NOT NULL
  AND trim(c.command, char(9)||char(10)||char(11)||char(12)||char(13)||' ') <> ''
  AND ((c.delivery_outcome IN ('injected','printed'))
       OR (c.delivery_outcome='executed' AND COALESCE(c.execution_exit_code,1)=0 AND c.execution_error=''))
ORDER BY c.selected_at DESC, c.id DESC LIMIT 1`).Scan(
		&c.ID, &c.ResponseID, &c.ConversationID, &c.Ordinal, &c.Command,
		&c.ShortExplanation, &c.IsDangerous, &c.DangerousExplanation,
		&selected, &executed, &exit, &c.ExecutionError, &c.DeliveryOutcome,
	)
	if err != nil {
		return c, missing(err)
	}
	if selected.Valid {
		parsed, parseErr := parseTime(selected.String)
		if parseErr != nil {
			return c, historyError(parseErr)
		}
		c.SelectedAt = &parsed
	}
	if executed.Valid {
		parsed, parseErr := parseTime(executed.String)
		if parseErr != nil {
			return c, historyError(parseErr)
		}
		c.ExecutedAt = &parsed
	}
	if exit.Valid {
		value := int(exit.Int64)
		c.ExecutionExitCode = &value
	}
	return c, nil
}

// ListConversations returns a bounded newest-first page using a timestamp/ID cursor.
func (s *Store) ListConversations(ctx context.Context, in service.ListRequest) ([]service.Conversation, error) {
	if in.Limit <= 0 || in.Limit > 1000 {
		return nil, fmt.Errorf("%w: page size must be 1..1000", service.ErrInvalid)
	}
	if in.BeforeCreatedAt.IsZero() != (in.BeforeID == "") {
		return nil, fmt.Errorf("%w: incomplete page cursor", service.ErrInvalid)
	}
	cursor := stamp(in.BeforeCreatedAt)
	if in.BeforeCreatedAt.IsZero() {
		cursor = "9999-12-31T23:59:59.999999999Z"
		in.BeforeID = "~"
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id,created_at,updated_at,status,kind,provider,model,COALESCE(selected_command_id,''),COALESCE(metadata_json,'') FROM conversations WHERE created_at < ? OR (created_at = ? AND id < ?) ORDER BY created_at DESC,id DESC LIMIT ?`, cursor, cursor, in.BeforeID, in.Limit)
	if err != nil {
		return nil, historyError(err)
	}
	defer closeHistoryRows(rows, &err)
	var result []service.Conversation
	for rows.Next() {
		var c service.Conversation
		var created, updated string
		if err = rows.Scan(&c.ID, &created, &updated, &c.Status, &c.Kind, &c.Provider, &c.Model, &c.SelectedCommandID, &c.MetadataJSON); err != nil {
			return nil, historyError(err)
		}
		c.CreatedAt, err = time.Parse(time.RFC3339Nano, created)
		if err != nil {
			return nil, historyError(err)
		}
		c.UpdatedAt, err = time.Parse(time.RFC3339Nano, updated)
		if err != nil {
			return nil, historyError(err)
		}
		result = append(result, c)
	}
	return result, historyError(rows.Err())
}

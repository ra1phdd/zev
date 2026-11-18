package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/ra1phdd/zev/internal/entities/history/lexical"
	"github.com/ra1phdd/zev/internal/entities/history/service"
)

// SearchDocument describes one history document and its lexical index metadata.
type SearchDocument struct {
	ConversationID string
	MessageID      string
	CommandID      string
	Kind           string
	Content        string
	Created        string
}

type searchDocument struct{ conversationID, messageID, commandID, kind, content, created string }

// InsertSearchDocument inserts a document and updates all lexical indexes in tx.
func InsertSearchDocument(ctx context.Context, tx *sql.Tx, doc SearchDocument) error {
	return insertSearchDocument(ctx, tx, searchDocument{conversationID: doc.ConversationID, messageID: doc.MessageID, commandID: doc.CommandID, kind: doc.Kind, content: doc.Content, created: doc.Created})
}

func insertSearchDocument(ctx context.Context, tx *sql.Tx, doc searchDocument) error {
	res, err := tx.ExecContext(ctx, `INSERT INTO history_documents(conversation_id,message_id,command_id,kind,content) VALUES(?,?,?,?,?)`, doc.conversationID, optional(doc.messageID), optional(doc.commandID), doc.kind, doc.content)
	if err != nil {
		return historyError(err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return historyError(err)
	}
	return indexDocument(ctx, tx, id, doc.content, doc.created)
}

func indexDocument(ctx context.Context, tx *sql.Tx, id int64, content, created string) error {
	normalized, morphology := lexical.Normalize(content), lexical.Morphology(content)
	var kind string
	if err := tx.QueryRowContext(ctx, `SELECT kind FROM history_documents WHERE id=?`, id).Scan(&kind); err != nil {
		return historyError(err)
	}
	documentType := kind
	if kind == initialPromptKind || kind == refinementKind || kind == userDocument {
		documentType = userDocument
	}
	if _, err := tx.ExecContext(ctx, `UPDATE history_documents SET document_type=?,created_at=?,profile_id=?,normalized=?,morphology=? WHERE id=?`, documentType, created, lexical.ProfileID, normalized, morphology, id); err != nil {
		return historyError(err)
	}
	for _, q := range []string{`INSERT INTO history_fts_surface(rowid,content) VALUES(?,?)`, `INSERT INTO history_fts_morphology(rowid,content) VALUES(?,?)`, `INSERT INTO history_fts_trigram(rowid,content) VALUES(?,?)`} {
		value := normalized
		if strings.Contains(q, "morphology") {
			value = morphology
		}
		if _, err := tx.ExecContext(ctx, q, id, value); err != nil {
			return historyError(err)
		}
	}
	seen := map[string]bool{}
	tokens := lexical.Tokens(content)
	for _, token := range lexical.Tokens(content) {
		tokens = append(tokens, lexical.Stem(token))
	}
	for _, token := range tokens {
		if seen[token] {
			continue
		}
		seen[token] = true
		if _, err := tx.ExecContext(ctx, `INSERT INTO history_tokens(document_id,token) VALUES(?,?)`, id, token); err != nil {
			return historyError(err)
		}
		for _, key := range lexical.DeletionKeys(token) {
			if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO history_deletions(token,deletion_key) VALUES(?,?)`, token, key); err != nil {
				return historyError(err)
			}
		}
	}
	return nil
}

// RebuildFTS transactionally rebuilds the derived indexes for the current profile.
func (s *Store) RebuildFTS(ctx context.Context) (err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return historyError(err)
	}
	defer rollbackHistory(tx, &err)
	if err = rebuildFTS(ctx, tx); err != nil {
		return fmt.Errorf("rebuild history index: %w", err)
	}
	return historyError(tx.Commit())
}

// Revision returns a durable dataset watermark, advanced on history or vector changes.
func (s *Store) Revision(ctx context.Context) (int64, error) {
	var n int64
	err := s.db.QueryRowContext(ctx, `SELECT revision FROM history_revision WHERE id=1`).Scan(&n)
	return n, historyError(err)
}

// Clear deletes all conversations and their dependent documents and vectors.
func (s *Store) Clear(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM conversations`)
	return historyError(err)
}

// IndexStatus reports the lexical index and optional vector coverage for a profile.
func (s *Store) IndexStatus(ctx context.Context, profile string) (service.IndexStatus, error) {
	var out service.IndexStatus
	out.ProfileID = lexical.ProfileID
	out.EmbeddingProfileID = profile
	err := s.db.QueryRowContext(ctx, `SELECT count(*),COALESCE(sum(profile_id != ?),0)=0 FROM history_documents`, lexical.ProfileID).Scan(&out.Documents, &out.LexicalReady)
	if err != nil {
		return out, historyError(err)
	}
	if profile != "" {
		out.EmbeddingCoverage, err = s.Coverage(ctx, profile)
	}
	return out, historyError(err)
}

func rebuildFTS(ctx context.Context, tx *sql.Tx) error {
	for _, q := range []string{`DELETE FROM history_fts_surface`, `DELETE FROM history_fts_morphology`, `DELETE FROM history_fts_trigram`, `DELETE FROM history_tokens`, `DELETE FROM history_deletions`} {
		if _, err := tx.ExecContext(ctx, q); err != nil {
			return fmt.Errorf("clear history index: %w", err)
		}
	}
	var after int64
	for {
		rows, err := tx.QueryContext(ctx, `SELECT d.id,d.content,COALESCE(m.created_at,r.created_at,c.created_at) FROM history_documents d JOIN conversations c ON c.id=d.conversation_id LEFT JOIN messages m ON m.id=d.message_id LEFT JOIN commands cmd ON cmd.id=d.command_id LEFT JOIN responses r ON r.id=cmd.response_id WHERE d.id>? ORDER BY d.id LIMIT 256`, after)
		if err != nil {
			return fmt.Errorf("read history documents: %w", err)
		}
		docs, err := scanRebuildDocuments(rows)
		if err != nil {
			return fmt.Errorf("read history documents: %w", err)
		}
		if len(docs) == 0 {
			break
		}
		for _, d := range docs {
			if err := indexDocument(ctx, tx, d.id, d.content, d.created); err != nil {
				return err
			}
			after = d.id
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE history_revision SET revision=revision+1`); err != nil {
		return fmt.Errorf("advance history revision: %w", err)
	}
	return nil
}

type rebuildDocument struct {
	id               int64
	content, created string
}

func scanRebuildDocuments(rows *sql.Rows) (_ []rebuildDocument, err error) {
	defer func() { err = errors.Join(err, rows.Close()) }()
	docs := make([]rebuildDocument, 0, 256)
	for rows.Next() {
		var d rebuildDocument
		if err := rows.Scan(&d.id, &d.content, &d.created); err != nil {
			return nil, fmt.Errorf("scan document: %w", err)
		}
		docs = append(docs, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate documents: %w", err)
	}
	return docs, nil
}

package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/ra1phdd/zev/internal/entities/history/lexical"
	"github.com/ra1phdd/zev/internal/entities/history/service"
)

const schema2 = `
ALTER TABLE message_embeddings RENAME TO message_embeddings_v1;
CREATE TABLE message_embeddings (message_id TEXT NOT NULL REFERENCES messages(id) ON DELETE CASCADE, profile_id TEXT NOT NULL, model TEXT NOT NULL, dimensions INTEGER NOT NULL CHECK(dimensions>0), embedding BLOB, content_hash TEXT NOT NULL, created_at TEXT NOT NULL, status TEXT NOT NULL CHECK(status IN ('ready','failed','stale')), error TEXT NOT NULL DEFAULT '', PRIMARY KEY(message_id,profile_id));
INSERT INTO message_embeddings SELECT message_id, 'legacy:' || legacy.model, model, dimensions, embedding, '', strftime('%Y-%m-%dT%H:%M:%fZ','now'), 'stale', '' FROM message_embeddings_v1 AS legacy;
DROP TABLE message_embeddings_v1;
ALTER TABLE commands ADD COLUMN delivery_outcome TEXT NOT NULL DEFAULT 'unknown' CHECK(delivery_outcome IN ('selected','injected','printed','executed','failed','unknown'));
ALTER TABLE history_documents ADD COLUMN document_type TEXT NOT NULL DEFAULT '';
ALTER TABLE history_documents ADD COLUMN created_at TEXT NOT NULL DEFAULT '';
ALTER TABLE history_documents ADD COLUMN profile_id TEXT NOT NULL DEFAULT '';
ALTER TABLE history_documents ADD COLUMN normalized TEXT NOT NULL DEFAULT '';
ALTER TABLE history_documents ADD COLUMN morphology TEXT NOT NULL DEFAULT '';
CREATE VIRTUAL TABLE history_fts_surface USING fts5(content);
CREATE VIRTUAL TABLE history_fts_morphology USING fts5(content);
CREATE VIRTUAL TABLE history_fts_trigram USING fts5(content, tokenize='trigram');
CREATE TABLE history_tokens (document_id INTEGER NOT NULL REFERENCES history_documents(id) ON DELETE CASCADE, token TEXT NOT NULL, PRIMARY KEY(document_id,token));
CREATE TABLE history_deletions (token TEXT NOT NULL, deletion_key TEXT NOT NULL, PRIMARY KEY(token,deletion_key));
CREATE INDEX history_deletions_key ON history_deletions(deletion_key,token);
CREATE TABLE history_revision (id INTEGER PRIMARY KEY CHECK(id=1), revision INTEGER NOT NULL);
INSERT INTO history_revision VALUES(1,0);
CREATE TRIGGER history_revision_ci AFTER INSERT ON conversations BEGIN UPDATE history_revision SET revision=revision+1; END;
CREATE TRIGGER history_revision_cu AFTER UPDATE ON conversations BEGIN UPDATE history_revision SET revision=revision+1; END;
CREATE TRIGGER history_revision_cd AFTER DELETE ON conversations BEGIN UPDATE history_revision SET revision=revision+1; END;
CREATE TRIGGER history_revision_mi AFTER INSERT ON messages BEGIN UPDATE history_revision SET revision=revision+1; END;
CREATE TRIGGER history_revision_mu AFTER UPDATE ON messages BEGIN UPDATE history_revision SET revision=revision+1; END;
CREATE TRIGGER history_revision_md AFTER DELETE ON messages BEGIN UPDATE history_revision SET revision=revision+1; END;
CREATE TRIGGER history_revision_ei AFTER INSERT ON message_embeddings BEGIN UPDATE history_revision SET revision=revision+1; END;
CREATE TRIGGER history_revision_eu AFTER UPDATE ON message_embeddings BEGIN UPDATE history_revision SET revision=revision+1; END;
CREATE TRIGGER history_revision_ed AFTER DELETE ON message_embeddings BEGIN UPDATE history_revision SET revision=revision+1; END;
CREATE TRIGGER history_token_delete AFTER DELETE ON history_tokens WHEN NOT EXISTS (SELECT 1 FROM history_tokens WHERE token=old.token) BEGIN DELETE FROM history_deletions WHERE token=old.token; END;
CREATE TRIGGER history_document_derived_delete AFTER DELETE ON history_documents BEGIN DELETE FROM history_fts_surface WHERE rowid=old.id; DELETE FROM history_fts_morphology WHERE rowid=old.id; DELETE FROM history_fts_trigram WHERE rowid=old.id; END;
`

const schema3 = `
CREATE TABLE history_writes (
 conversation_id TEXT PRIMARY KEY REFERENCES conversations(id) ON DELETE CASCADE,
 initial_message_id TEXT NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
 fingerprint TEXT NOT NULL
);
`

type searchDocument struct{ conversationID, messageID, commandID, kind, content, created string }

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

func rebuildFTSTx(ctx context.Context, tx *sql.Tx) error {
	for _, q := range []string{`DELETE FROM history_fts_surface`, `DELETE FROM history_fts_morphology`, `DELETE FROM history_fts_trigram`, `DELETE FROM history_tokens`, `DELETE FROM history_deletions`} {
		if _, err := tx.ExecContext(ctx, q); err != nil {
			return historyError(err)
		}
	}
	var afterID int64
	for {
		docs, err := rebuildDocuments(ctx, tx, afterID)
		if err != nil {
			return err
		}
		if len(docs) == 0 {
			break
		}
		for _, d := range docs {
			if err = indexDocument(ctx, tx, d.id, d.content, d.created); err != nil {
				return historyError(err)
			}
			afterID = d.id
		}
	}
	_, err := tx.ExecContext(ctx, `UPDATE history_revision SET revision=revision+1`)
	return historyError(err)
}

type rebuildDocument struct {
	id               int64
	content, created string
}

func rebuildDocuments(ctx context.Context, tx *sql.Tx, afterID int64) (docs []rebuildDocument, err error) {
	rows, err := tx.QueryContext(ctx, `SELECT d.id,d.content,COALESCE(m.created_at,r.created_at,c.created_at) FROM history_documents d JOIN conversations c ON c.id=d.conversation_id LEFT JOIN messages m ON m.id=d.message_id LEFT JOIN commands cmd ON cmd.id=d.command_id LEFT JOIN responses r ON r.id=cmd.response_id WHERE d.id>? ORDER BY d.id LIMIT 256`, afterID)
	if err != nil {
		return nil, historyError(err)
	}
	defer closeHistoryRows(rows, &err)
	for rows.Next() {
		var d rebuildDocument
		if err = rows.Scan(&d.id, &d.content, &d.created); err != nil {
			return nil, historyError(err)
		}
		docs = append(docs, d)
	}
	return docs, historyError(rows.Err())
}

// RebuildFTS transactionally rebuilds the derived indexes for the current profile.
func (s *Store) RebuildFTS(ctx context.Context) (err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return historyError(err)
	}
	defer rollbackHistory(tx, &err)
	if err = rebuildFTSTx(ctx, tx); err != nil {
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

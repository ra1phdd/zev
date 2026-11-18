-- Released v1 history schema. This is a fresh baseline, not a development upgrade.
CREATE TABLE history_schema (
 id INTEGER PRIMARY KEY CHECK(id=1),
 format TEXT NOT NULL CHECK(format='zev-history-release-v1')
);
INSERT INTO history_schema(id,format) VALUES(1,'zev-history-release-v1');
CREATE TABLE conversations (id TEXT PRIMARY KEY, created_at TEXT NOT NULL, updated_at TEXT NOT NULL, status TEXT NOT NULL CHECK(status IN ('started','completed','cancelled','failed')), kind TEXT NOT NULL, provider TEXT NOT NULL DEFAULT '', model TEXT NOT NULL DEFAULT '', selected_command_id TEXT REFERENCES commands(id) ON DELETE SET NULL, metadata_json TEXT CHECK(metadata_json IS NULL OR json_valid(metadata_json)));
CREATE TABLE messages (id TEXT PRIMARY KEY, conversation_id TEXT NOT NULL REFERENCES conversations(id) ON DELETE CASCADE, sequence INTEGER NOT NULL CHECK(sequence >= 0), role TEXT NOT NULL CHECK(role IN ('user','assistant','system')), kind TEXT NOT NULL CHECK(kind IN ('initial_prompt','refinement','response','system_prompt','error')), content TEXT NOT NULL, created_at TEXT NOT NULL, metadata_json TEXT CHECK(metadata_json IS NULL OR json_valid(metadata_json)), parts_json TEXT CHECK(parts_json IS NULL OR json_valid(parts_json)), UNIQUE(conversation_id,sequence), UNIQUE(id,conversation_id));
CREATE TABLE responses (id TEXT PRIMARY KEY, conversation_id TEXT NOT NULL REFERENCES conversations(id) ON DELETE CASCADE, message_id TEXT NOT NULL UNIQUE, provider TEXT NOT NULL, model TEXT NOT NULL, payload_json TEXT NOT NULL CHECK(json_valid(payload_json)), is_valid INTEGER NOT NULL CHECK(is_valid IN (0,1)), created_at TEXT NOT NULL, FOREIGN KEY(message_id,conversation_id) REFERENCES messages(id,conversation_id) ON DELETE CASCADE, UNIQUE(id,conversation_id));
CREATE TABLE commands (id TEXT PRIMARY KEY, response_id TEXT NOT NULL, conversation_id TEXT NOT NULL REFERENCES conversations(id) ON DELETE CASCADE, ordinal INTEGER NOT NULL CHECK(ordinal >= 0), command TEXT NOT NULL, short_explanation TEXT NOT NULL, is_dangerous INTEGER NOT NULL CHECK(is_dangerous IN (0,1)), dangerous_explanation TEXT NOT NULL, selected_at TEXT, executed_at TEXT, execution_exit_code INTEGER, execution_error TEXT NOT NULL DEFAULT '', delivery_outcome TEXT NOT NULL DEFAULT 'unknown' CHECK(delivery_outcome IN ('selected','injected','printed','executed','failed','unknown')), FOREIGN KEY(response_id,conversation_id) REFERENCES responses(id,conversation_id) ON DELETE CASCADE, UNIQUE(response_id,ordinal));
CREATE TABLE message_embeddings (message_id TEXT NOT NULL REFERENCES messages(id) ON DELETE CASCADE, profile_id TEXT NOT NULL, model TEXT NOT NULL, dimensions INTEGER NOT NULL CHECK(dimensions>0), embedding BLOB, content_hash TEXT NOT NULL, created_at TEXT NOT NULL, status TEXT NOT NULL CHECK(status IN ('ready','failed','stale')), error TEXT NOT NULL DEFAULT '', PRIMARY KEY(message_id,profile_id));
CREATE TABLE history_documents (id INTEGER PRIMARY KEY, conversation_id TEXT NOT NULL REFERENCES conversations(id) ON DELETE CASCADE, message_id TEXT REFERENCES messages(id) ON DELETE CASCADE, command_id TEXT REFERENCES commands(id) ON DELETE CASCADE, kind TEXT NOT NULL, content TEXT NOT NULL, document_type TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL DEFAULT '', profile_id TEXT NOT NULL DEFAULT '', normalized TEXT NOT NULL DEFAULT '', morphology TEXT NOT NULL DEFAULT '', CHECK((message_id IS NOT NULL) != (command_id IS NOT NULL)));
CREATE VIRTUAL TABLE history_fts USING fts5(content,content='history_documents',content_rowid='id');
CREATE VIRTUAL TABLE history_fts_surface USING fts5(content);
CREATE VIRTUAL TABLE history_fts_morphology USING fts5(content);
CREATE VIRTUAL TABLE history_fts_trigram USING fts5(content,tokenize='trigram');
CREATE TABLE history_tokens (document_id INTEGER NOT NULL REFERENCES history_documents(id) ON DELETE CASCADE, token TEXT NOT NULL, PRIMARY KEY(document_id,token));
CREATE TABLE history_deletions (token TEXT NOT NULL, deletion_key TEXT NOT NULL, PRIMARY KEY(token,deletion_key));
CREATE TABLE history_revision (id INTEGER PRIMARY KEY CHECK(id=1), revision INTEGER NOT NULL);
INSERT INTO history_revision(id,revision) VALUES(1,0);
CREATE TABLE history_writes (conversation_id TEXT PRIMARY KEY REFERENCES conversations(id) ON DELETE CASCADE, initial_message_id TEXT NOT NULL REFERENCES messages(id) ON DELETE CASCADE, fingerprint TEXT NOT NULL);
CREATE TABLE migration_runs (id TEXT PRIMARY KEY, source_fingerprint TEXT NOT NULL, status TEXT NOT NULL, started_at TEXT NOT NULL, completed_at TEXT, metadata_json TEXT CHECK(metadata_json IS NULL OR json_valid(metadata_json)));
CREATE TABLE migration_lines (run_id TEXT NOT NULL REFERENCES migration_runs(id) ON DELETE CASCADE, line_number INTEGER NOT NULL CHECK(line_number > 0), conversation_id TEXT NOT NULL REFERENCES conversations(id) ON DELETE CASCADE, PRIMARY KEY(run_id,line_number));
CREATE INDEX conversations_created_idx ON conversations(created_at DESC,id DESC);
CREATE INDEX messages_conversation_idx ON messages(conversation_id,sequence);
CREATE INDEX history_deletions_key ON history_deletions(deletion_key,token);
CREATE TRIGGER history_documents_ai AFTER INSERT ON history_documents BEGIN INSERT INTO history_fts(rowid,content) VALUES(new.id,new.content); END;
CREATE TRIGGER history_documents_ad AFTER DELETE ON history_documents BEGIN INSERT INTO history_fts(history_fts,rowid,content) VALUES('delete',old.id,old.content); END;
CREATE TRIGGER history_documents_au AFTER UPDATE ON history_documents BEGIN INSERT INTO history_fts(history_fts,rowid,content) VALUES('delete',old.id,old.content); INSERT INTO history_fts(rowid,content) VALUES(new.id,new.content); END;
CREATE TRIGGER history_document_derived_delete AFTER DELETE ON history_documents BEGIN DELETE FROM history_fts_surface WHERE rowid=old.id; DELETE FROM history_fts_morphology WHERE rowid=old.id; DELETE FROM history_fts_trigram WHERE rowid=old.id; END;
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

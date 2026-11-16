package repository

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
	"sort"
	"strings"

	"github.com/ra1phdd/zev/internal/entities/history/lexical"
	"github.com/ra1phdd/zev/internal/entities/history/service"
)

// Coverage counts eligible messages and their current profile embedding states.
func (s *Store) Coverage(ctx context.Context, profile string) (c service.EmbeddingCoverage, err error) {
	rows, err := s.db.QueryContext(ctx, `SELECT m.content, COALESCE(e.content_hash,''), COALESCE(e.status,''), e.message_id FROM messages m LEFT JOIN message_embeddings e ON e.message_id=m.id AND e.profile_id=? WHERE m.role='user' AND m.kind IN ('initial_prompt','refinement')`, profile)
	if err != nil {
		return c, fmt.Errorf("read embedding coverage: %w", err)
	}
	defer closeHistoryRows(rows, &err)
	for rows.Next() {
		var content, hash, status string
		var id sql.NullString
		if err := rows.Scan(&content, &hash, &status, &id); err != nil {
			return c, fmt.Errorf("read embedding coverage: %w", err)
		}
		c.Eligible++
		digest := fmt.Sprintf("%x", sha256.Sum256([]byte(content)))
		switch {
		case status == readyEmbedding && hash == digest:
			c.Ready++
		case status == failedStatus:
			c.Failed++
		case status == "stale" || status == readyEmbedding:
			c.Stale++
		}
	}
	return c, historyError(rows.Err())
}

func validEmbedding(e service.Embedding) error {
	if e.MessageID == "" || e.ProfileID == "" || e.Model == "" || e.Dimensions <= 0 || e.Status == "" || (e.Status == readyEmbedding && (len(e.Vector)%4 != 0 || len(e.Vector)/4 != e.Dimensions)) {
		return fmt.Errorf("%w: embedding fields", service.ErrInvalid)
	}
	if e.Status != readyEmbedding && e.Status != failedStatus && e.Status != "stale" {
		return fmt.Errorf("%w: embedding status", service.ErrInvalid)
	}
	if e.Status == readyEmbedding {
		var norm float64
		for i := range e.Dimensions {
			v := math.Float32frombits(binary.LittleEndian.Uint32(e.Vector[i*4:]))
			if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
				return fmt.Errorf("%w: nonfinite embedding", service.ErrInvalid)
			}
			norm += float64(v) * float64(v)
		}
		if norm == 0 {
			return fmt.Errorf("%w: zero embedding", service.ErrInvalid)
		}
	}
	return nil
}

// SaveEmbedding stores a vector only for an eligible message and profile.
func (s *Store) SaveEmbedding(ctx context.Context, e service.Embedding) (err error) {
	if err := validEmbedding(e); err != nil {
		return historyError(err)
	}
	var content, role, kind string
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return historyError(err)
	}
	defer rollbackHistory(tx, &err)
	if err := tx.QueryRowContext(ctx, `SELECT content,role,kind FROM messages WHERE id=?`, e.MessageID).Scan(&content, &role, &kind); err != nil {
		return missing(err)
	}
	if role != userRole || (kind != initialPromptKind && kind != refinementKind) {
		return fmt.Errorf("%w: embeddings require user message", service.ErrInvalid)
	}
	want := sha256.Sum256([]byte(content))
	if e.ContentHash == "" || e.ContentHash != hex.EncodeToString(want[:]) {
		return fmt.Errorf("%w: embedding content hash", service.ErrConflict)
	}
	now := stamp(e.CreatedAt)
	if _, err = tx.ExecContext(ctx, `INSERT INTO message_embeddings(message_id,profile_id,model,dimensions,embedding,content_hash,created_at,status,error) VALUES(?,?,?,?,?,?,?,?,?) ON CONFLICT(message_id,profile_id) DO UPDATE SET model=excluded.model,dimensions=excluded.dimensions,embedding=excluded.embedding,content_hash=excluded.content_hash,created_at=excluded.created_at,status=excluded.status,error=excluded.error`, e.MessageID, e.ProfileID, e.Model, e.Dimensions, e.Vector, e.ContentHash, now, e.Status, e.Error); err != nil {
		return historyError(err)
	}
	return historyError(tx.Commit())
}

// PendingMessages returns user messages without a current ready vector.
func (s *Store) PendingMessages(ctx context.Context, profile string, retry bool, afterID string, limit int) (out []service.Message, err error) {
	if limit <= 0 || limit > 1000 {
		return nil, fmt.Errorf("%w: embedding page size", service.ErrInvalid)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT m.id,m.conversation_id,m.sequence,m.role,m.kind,m.content,m.created_at,COALESCE(m.metadata_json,''),COALESCE(m.parts_json,''),COALESCE(e.status,''),COALESCE(e.content_hash,'') FROM messages m LEFT JOIN message_embeddings e ON e.message_id=m.id AND e.profile_id=? WHERE m.role='user' AND m.kind IN ('initial_prompt','refinement') AND m.id>? ORDER BY m.id`, profile, afterID)
	if err != nil {
		return nil, historyError(err)
	}
	defer closeHistoryRows(rows, &err)
	for rows.Next() {
		var m service.Message
		var ts, status, hash string
		if err = rows.Scan(&m.ID, &m.ConversationID, &m.Sequence, &m.Role, &m.Kind, &m.Content, &ts, &m.MetadataJSON, &m.PartsJSON, &status, &hash); err != nil {
			return nil, historyError(err)
		}
		if status == failedStatus && !retry || status == readyEmbedding && hash == fmt.Sprintf("%x", sha256.Sum256([]byte(m.Content))) {
			continue
		}
		m.CreatedAt, err = parseTime(ts)
		if err != nil {
			return nil, historyError(err)
		}
		out = append(out, m)
		if len(out) == limit {
			break
		}
	}
	return out, historyError(rows.Err())
}

// VisitVectors streams current profile vectors without retaining the callback or all blobs.
func (s *Store) VisitVectors(ctx context.Context, req service.SearchRequest, profile string, visit func(service.VectorCandidate) error) (err error) {
	if visit == nil {
		return fmt.Errorf("%w: nil vector visitor", service.ErrInvalid)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT e.embedding,e.dimensions,e.content_hash,m.id,m.conversation_id,m.id,m.kind,m.content,m.created_at,c.id,c.status,c.kind,c.provider,c.model,COALESCE(c.selected_command_id,''),COALESCE(c.metadata_json,'') FROM message_embeddings e JOIN messages m ON m.id=e.message_id JOIN conversations c ON c.id=m.conversation_id WHERE e.profile_id=? AND e.status='ready' AND m.role='user' AND m.kind IN ('initial_prompt','refinement') ORDER BY m.created_at DESC,m.id DESC`, profile)
	if err != nil {
		return historyError(err)
	}
	defer closeHistoryRows(rows, &err)
	for rows.Next() {
		var blob []byte
		var dims int
		var h service.SearchHit
		var ts, selected, meta, hash string
		if err = rows.Scan(&blob, &dims, &hash, &h.DocumentID, &h.ConversationID, &h.MessageID, &h.DocumentType, &h.Snippet, &ts, &h.Conversation.ID, &h.Conversation.Status, &h.Conversation.Kind, &h.Conversation.Provider, &h.Conversation.Model, &selected, &meta); err != nil {
			return historyError(err)
		}
		if hash != fmt.Sprintf("%x", sha256.Sum256([]byte(h.Snippet))) {
			continue
		}
		h.DocumentID, h.DocumentType = h.MessageID, userDocument
		h.CreatedAt, err = parseTime(ts)
		if err != nil {
			return historyError(err)
		}
		h.Conversation.ID, h.Conversation.SelectedCommandID, h.Conversation.MetadataJSON = h.ConversationID, selected, meta
		h.ScoreKind = "cosine"
		if !vectorMatchesFilters(req, h) {
			continue
		}
		if err = visit(service.VectorCandidate{Hit: h, Vector: blob, Dimensions: dims}); err != nil {
			return historyError(err)
		}
	}
	return historyError(rows.Err())
}

type pageCursor struct {
	Revision int64  `json:"r"`
	Query    string `json:"q"`
	Offset   int    `json:"o"`
}

type lexicalCandidate struct {
	id      int64
	score   float64
	sources map[string]bool
	tier    int
}

func ftsTerm(term string) string { return `"` + strings.ReplaceAll(term, `"`, `""`) + `"` }
func ftsAll(terms []string) string {
	parts := make([]string, len(terms))
	for i, term := range terms {
		parts[i] = ftsTerm(term)
	}
	return strings.Join(parts, " AND ")
}

func ftsAny(terms []string) string {
	parts := make([]string, len(terms))
	for i, term := range terms {
		parts[i] = ftsTerm(term)
	}
	return strings.Join(parts, " OR ")
}

// searchFilters returns SQL predicates and bound values, keeping filters inside
// the bounded FTS candidate query rather than applying them after top-K.
func searchFilters(req service.SearchRequest, alias string) (string, []any) {
	predicates := []string{"1=1"}
	args := []any{}
	if req.Provider != "" {
		predicates = append(predicates, "c.provider=?")
		args = append(args, req.Provider)
	}
	if req.Model != "" {
		predicates = append(predicates, "c.model=?")
		args = append(args, req.Model)
	}
	if req.Status != "" {
		predicates = append(predicates, "c.status=?")
		args = append(args, req.Status)
	}
	if !req.From.IsZero() {
		predicates = append(predicates, alias+".created_at>=?")
		args = append(args, stamp(req.From))
	}
	if !req.To.IsZero() {
		predicates = append(predicates, alias+".created_at<?")
		args = append(args, stamp(req.To))
	}
	if req.Dangerous != nil {
		predicates = append(predicates, "cmd.is_dangerous=?")
		args = append(args, *req.Dangerous)
	}
	if req.Type != "" {
		typ := req.Type
		if typ == assistantRole {
			typ = "assistant_message"
		}
		if typ == userRole {
			typ = userDocument
		}
		predicates = append(predicates, alias+".document_type=?")
		args = append(args, typ)
	}
	return strings.Join(predicates, " AND "), args
}

type tokenExpansion struct {
	token     string
	values    map[string]float64
	truncated bool
}

func tokenCandidates(ctx context.Context, tx *sql.Tx, token string) (map[string]float64, bool, error) {
	expansion := tokenExpansion{token: token, values: map[string]float64{}}
	keys := append([]string{token}, lexical.DeletionKeys(token)...)
	for _, key := range keys {
		rows, err := tx.QueryContext(ctx, `SELECT token FROM history_deletions WHERE deletion_key=? UNION SELECT token FROM history_tokens WHERE token=? LIMIT ?`, key, key, lexical.CandidateLimit+1)
		if err != nil {
			return nil, false, fmt.Errorf("query deletion candidates: %w", err)
		}
		if err = expansion.read(rows); err != nil {
			return nil, false, err
		}
		if len(expansion.values) >= lexical.CandidateLimit {
			expansion.truncated = true
			break
		}
	}
	if trigrams := lexical.Trigrams(token); len(trigrams) > 0 {
		rows, err := tx.QueryContext(ctx, `SELECT DISTINCT t.token FROM history_fts_trigram JOIN history_tokens t ON t.document_id=history_fts_trigram.rowid WHERE history_fts_trigram MATCH ? ORDER BY t.token LIMIT ?`, ftsAny(trigrams), lexical.CandidateLimit+1)
		if err != nil {
			return nil, false, fmt.Errorf("query trigram candidates: %w", err)
		}
		if err = expansion.read(rows); err != nil {
			return nil, false, err
		}
	}
	return expansion.values, expansion.truncated, nil
}

func (e *tokenExpansion) read(rows *sql.Rows) (err error) {
	defer closeHistoryRows(rows, &err)
	count := 0
	for rows.Next() {
		count++
		if count > lexical.CandidateLimit {
			e.truncated = true
			break
		}
		var token string
		if err = rows.Scan(&token); err != nil {
			return fmt.Errorf("scan token candidate: %w", err)
		}
		if len(e.values) >= lexical.CandidateLimit {
			e.truncated = true
			continue
		}
		if score, ok := lexical.Similarity(e.token, token); ok && token != e.token {
			e.values[token] = score
		}
	}
	if err = rows.Err(); err != nil {
		return fmt.Errorf("iterate token candidates: %w", err)
	}
	return nil
}

func documentSimilarity(ctx context.Context, tx *sql.Tx, id int64, alternatives []map[string]float64) (float64, bool, error) {
	rows, err := tx.QueryContext(ctx, `SELECT token FROM history_tokens WHERE document_id=?`, id)
	if err != nil {
		return 0, false, historyError(err)
	}
	defer closeHistoryRows(rows, &err)
	var indexed []string
	for rows.Next() {
		var token string
		if err := rows.Scan(&token); err != nil {
			return 0, false, historyError(err)
		}
		indexed = append(indexed, token)
	}
	if err := rows.Err(); err != nil {
		return 0, false, historyError(err)
	}
	if len(alternatives) == 0 {
		return 0, false, nil
	}
	minimum := 1.0
	for _, options := range alternatives {
		best := 0.0
		for _, token := range indexed {
			best = max(best, options[lexical.Stem(token)])
		}
		if best == 0 {
			return 0, false, nil
		}
		minimum = min(minimum, best)
	}
	return minimum, true, nil
}

func rollbackHistory(tx *sql.Tx, errp *error) {
	if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) && *errp == nil {
		*errp = fmt.Errorf("rollback lexical search: %w", err)
	}
}

func closeHistoryRows(rows *sql.Rows, errp *error) {
	if err := rows.Close(); err != nil && *errp == nil {
		*errp = fmt.Errorf("close lexical rows: %w", err)
	}
}
func contentDigest(data []byte) string { return fmt.Sprintf("%x", sha256.Sum256(data)) }

type lexicalSearch struct {
	tx         *sql.Tx
	request    service.SearchRequest
	result     service.SearchResult
	candidates map[int64]*lexicalCandidate
	expansions []map[string]float64
	tokens     []string
	cursor     pageCursor
}

type rankedHit struct {
	hit       service.SearchHit
	candidate *lexicalCandidate
}

// LexicalSearch returns deterministic pages of exact, morphological, and bounded
// typo matches. SQL filters precede each candidate cap; cursors bind the snapshot.
func (s *Store) LexicalSearch(ctx context.Context, req service.SearchRequest) (out service.SearchResult, err error) {
	if req.Limit == 0 {
		req.Limit = 20
	}
	if req.Limit < 0 || req.Limit > 1000 || strings.TrimSpace(req.Query) == "" && req.ExactCommand == "" {
		return out, fmt.Errorf("%w: search request", service.ErrInvalid)
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return out, fmt.Errorf("begin lexical search: %w", err)
	}
	defer rollbackHistory(tx, &err)
	search := lexicalSearch{tx: tx, request: req, result: service.SearchResult{Mode: "lexical", RequestedMode: req.Mode, ProfileID: lexical.ProfileID}, candidates: map[int64]*lexicalCandidate{}, tokens: lexical.Tokens(req.Query)}
	if err = search.readCursor(ctx); err != nil {
		return out, historyError(err)
	}
	if err = search.collect(ctx); err != nil {
		return out, historyError(err)
	}
	hits, err := search.readHits(ctx)
	if err != nil {
		return out, historyError(err)
	}
	rankLexicalHits(hits)
	if err = search.page(hits); err != nil {
		return out, historyError(err)
	}
	if err = tx.Commit(); err != nil {
		return out, fmt.Errorf("commit lexical search: %w", err)
	}
	return search.result, nil
}

func (l *lexicalSearch) readCursor(ctx context.Context) error {
	if err := l.tx.QueryRowContext(ctx, `SELECT revision FROM history_revision WHERE id=1`).Scan(&l.cursor.Revision); err != nil {
		return fmt.Errorf("read lexical revision: %w", err)
	}
	request := l.request
	request.Cursor = ""
	data, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("encode lexical request: %w", err)
	}
	l.cursor.Query = contentDigest(data)
	if request = l.request; request.Cursor == "" {
		return nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(request.Cursor)
	var previous pageCursor
	if err != nil || json.Unmarshal(raw, &previous) != nil || previous.Offset < 0 || previous.Query != l.cursor.Query {
		return fmt.Errorf("%w: invalid search cursor", service.ErrInvalid)
	}
	if previous.Revision != l.cursor.Revision {
		return fmt.Errorf("%w: history changed since cursor", service.ErrConflict)
	}
	l.cursor.Offset = previous.Offset
	return nil
}

func (l *lexicalSearch) collect(ctx context.Context) error {
	switch {
	case l.request.ExactCommand != "":
		return l.collectLiteral(ctx, true)
	case l.request.Literal:
		return l.collectLiteral(ctx, false)
	}
	if err := l.addList(ctx, lexicalList{table: "history_fts_surface", expression: ftsAll(l.tokens), source: "exact", tier: 0}); err != nil {
		return historyError(err)
	}
	stems := make([]string, len(l.tokens))
	for i, token := range l.tokens {
		stems[i] = lexical.Stem(token)
	}
	if err := l.addList(ctx, lexicalList{table: morphologyTable, expression: ftsAll(stems), source: "morphology", tier: 1}); err != nil {
		return historyError(err)
	}
	if l.request.Fuzzy == fuzzyOff || l.request.Fuzzy != fuzzyAlways && len(l.candidates) >= l.request.Limit {
		return nil
	}
	expression, err := l.expand(ctx)
	if err != nil {
		return historyError(err)
	}
	return l.addList(ctx, lexicalList{table: morphologyTable, expression: expression, source: "typo", tier: 2})
}

type lexicalList struct {
	table, expression, source string
	tier                      int
}

func (l *lexicalSearch) addList(ctx context.Context, list lexicalList) (err error) {
	if list.expression == "" {
		return nil
	}
	switch list.table {
	case "history_fts_surface", morphologyTable:
	default:
		return fmt.Errorf("%w: lexical table", service.ErrInvalid)
	}
	filter, args := searchFilters(l.request, "d")
	// The table is allowlisted above; filter contains only fixed predicates. All
	// query text and filter values are bound parameters, never SQL expressions.
	query := `SELECT ` + list.table + `.rowid FROM ` + list.table + ` JOIN history_documents d ON d.id=` + list.table + `.rowid JOIN conversations c ON c.id=d.conversation_id LEFT JOIN commands cmd ON cmd.id=d.command_id WHERE ` + list.table + ` MATCH ? AND ` + filter + ` ORDER BY bm25(` + list.table + `),d.created_at DESC,COALESCE(d.message_id,d.command_id),d.kind LIMIT 5001` //nolint:gosec // G202: allowlisted table and constant predicates; values are bound.
	rows, err := l.tx.QueryContext(ctx, query, append([]any{list.expression}, args...)...)
	if err != nil {
		return fmt.Errorf("query lexical candidates: %w", err)
	}
	defer closeHistoryRows(rows, &err)
	rank := 0
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			return fmt.Errorf("scan lexical candidate: %w", err)
		}
		rank++
		if rank > 5000 {
			l.result.Truncated = true
			break
		}
		candidate := l.candidates[id]
		if candidate == nil {
			candidate = &lexicalCandidate{id: id, tier: list.tier, sources: map[string]bool{}}
			l.candidates[id] = candidate
		}
		// Lower-quality lists explain a hit but cannot boost its higher-quality tier.
		if candidate.tier == list.tier {
			candidate.score += 1 / float64(60+rank)
		}
		candidate.sources[list.source] = true
	}
	if err = rows.Err(); err != nil {
		return fmt.Errorf("iterate lexical candidates: %w", err)
	}
	return nil
}

func (l *lexicalSearch) collectLiteral(ctx context.Context, exact bool) (err error) {
	filter, args := searchFilters(l.request, "d")
	predicate, value := `instr(d.content,?)>0`, l.request.Query
	if exact {
		predicate, value = `d.kind='command' AND cmd.command=?`, l.request.ExactCommand
	}
	// Predicates are selected from constants; user text remains a bound value.
	query := `SELECT d.id FROM history_documents d JOIN conversations c ON c.id=d.conversation_id LEFT JOIN commands cmd ON cmd.id=d.command_id WHERE ` + predicate + ` AND ` + filter //nolint:gosec // G202: constant predicates only; all values are bound.
	rows, err := l.tx.QueryContext(ctx, query, append([]any{value}, args...)...)
	if err != nil {
		return fmt.Errorf("query literal history: %w", err)
	}
	defer closeHistoryRows(rows, &err)
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			return fmt.Errorf("scan literal history: %w", err)
		}
		l.candidates[id] = &lexicalCandidate{id: id, score: 1, tier: 0, sources: map[string]bool{"exact": true}}
	}
	if err = rows.Err(); err != nil {
		return fmt.Errorf("iterate literal history: %w", err)
	}
	return nil
}

type tokenOption struct {
	token string
	score float64
}

func (l *lexicalSearch) expand(ctx context.Context) (string, error) {
	alternatives := make([]string, len(l.tokens))
	for i, token := range l.tokens {
		values, cut, err := expandedStems(ctx, l.tx, token)
		if err != nil {
			return "", err
		}
		l.result.Truncated = l.result.Truncated || cut
		l.expansions = append(l.expansions, values)
		stems := make([]string, 0, len(values))
		for stem := range values {
			stems = append(stems, stem)
		}
		slices.Sort(stems)
		alternatives[i] = `(` + ftsAny(stems) + `)`
	}
	return strings.Join(alternatives, " AND "), nil
}

func expandedStems(ctx context.Context, tx *sql.Tx, token string) (map[string]float64, bool, error) {
	values := map[string]float64{lexical.Stem(token): 1}
	if n := len([]rune(token)); n < 4 || n > lexical.MaxTokenRunes {
		return values, false, nil
	}
	options, cut, err := tokenCandidates(ctx, tx, token)
	if err != nil {
		return nil, cut, err
	}
	ranked := make([]tokenOption, 0, len(options))
	for value, score := range options {
		ranked = append(ranked, tokenOption{value, score})
	}
	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].score != ranked[j].score {
			return ranked[i].score > ranked[j].score
		}
		return ranked[i].token < ranked[j].token
	})
	if len(ranked) > lexical.CandidateLimit {
		ranked = ranked[:lexical.CandidateLimit]
		cut = true
	}
	for _, option := range ranked {
		stem := lexical.Stem(option.token)
		values[stem] = max(values[stem], option.score)
	}
	return values, cut, nil
}

func (l *lexicalSearch) readHits(ctx context.Context) (hits []rankedHit, err error) {
	if len(l.candidates) == 0 {
		return nil, nil
	}
	ids := make([]int64, 0, len(l.candidates))
	for id := range l.candidates {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		placeholders[i], args[i] = "?", id
	}
	rows, err := l.tx.QueryContext(ctx, `SELECT d.id,COALESCE(d.message_id,''),COALESCE(d.command_id,''),d.kind,d.content,d.created_at,d.profile_id,d.conversation_id,c.created_at,c.updated_at,c.status,c.kind,c.provider,c.model,COALESCE(c.selected_command_id,''),COALESCE(c.metadata_json,'') FROM history_documents d JOIN conversations c ON c.id=d.conversation_id WHERE d.id IN (`+strings.Join(placeholders, ",")+`)`, args...) //nolint:gosec // G202: only generated ? placeholders are concatenated; IDs are bound.
	if err != nil {
		return nil, fmt.Errorf("read lexical documents: %w", err)
	}
	defer closeHistoryRows(rows, &err)
	for rows.Next() {
		hit, id, profile, e := scanLexicalHit(rows)
		if e != nil {
			return nil, historyError(e)
		}
		if profile != lexical.ProfileID {
			l.result.Degraded = true
			l.result.Warning = "lexical index profile is stale; rebuild required"
		}
		candidate := l.candidates[id]
		hit, matched, e := l.scoreHit(ctx, hit, candidate)
		if e != nil {
			return nil, historyError(e)
		}
		if matched {
			hits = append(hits, rankedHit{hit, candidate})
		}
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate lexical documents: %w", err)
	}
	return hits, nil
}

func scanLexicalHit(rows *sql.Rows) (service.SearchHit, int64, string, error) {
	var hit service.SearchHit
	var id int64
	var created, cc, cu, profile string
	if err := rows.Scan(&id, &hit.MessageID, &hit.CommandID, &hit.DocumentType, &hit.Snippet, &created, &profile, &hit.ConversationID, &cc, &cu, &hit.Conversation.Status, &hit.Conversation.Kind, &hit.Conversation.Provider, &hit.Conversation.Model, &hit.Conversation.SelectedCommandID, &hit.Conversation.MetadataJSON); err != nil {
		return hit, 0, "", fmt.Errorf("scan lexical document: %w", err)
	}
	var err error
	hit.CreatedAt, err = parseTime(created)
	if err != nil {
		return hit, 0, "", err
	}
	hit.Conversation.CreatedAt, err = parseTime(cc)
	if err != nil {
		return hit, 0, "", err
	}
	hit.Conversation.UpdatedAt, err = parseTime(cu)
	if err != nil {
		return hit, 0, "", err
	}
	hit.Conversation.ID = hit.ConversationID
	if hit.DocumentType == initialPromptKind || hit.DocumentType == refinementKind {
		hit.DocumentType = userDocument
	}
	hit.DocumentID = hit.CommandID
	if hit.DocumentID == "" {
		hit.DocumentID = hit.MessageID
	}
	return hit, id, profile, nil
}

func (l *lexicalSearch) scoreHit(ctx context.Context, hit service.SearchHit, candidate *lexicalCandidate) (service.SearchHit, bool, error) {
	hit.Score = candidate.score
	if candidate.tier == 2 {
		similarity, matched, err := documentSimilarity(ctx, l.tx, candidate.id, l.expansions)
		if err != nil {
			return hit, false, err
		}
		if !matched {
			return hit, false, nil
		}
		hit.Score = similarity
	}
	hit.ScoreKind = "rrf"
	for source := range candidate.sources {
		hit.MatchSources = append(hit.MatchSources, source)
	}
	slices.Sort(hit.MatchSources)
	return hit, true, nil
}

func rankLexicalHits(hits []rankedHit) {
	sort.Slice(hits, func(i, j int) bool {
		a, b := hits[i], hits[j]
		if a.candidate.tier != b.candidate.tier {
			return a.candidate.tier < b.candidate.tier
		}
		if a.hit.Score != b.hit.Score {
			return a.hit.Score > b.hit.Score
		}
		if !a.hit.CreatedAt.Equal(b.hit.CreatedAt) {
			return a.hit.CreatedAt.After(b.hit.CreatedAt)
		}
		if a.hit.DocumentID != b.hit.DocumentID {
			return a.hit.DocumentID < b.hit.DocumentID
		}
		return a.hit.DocumentType < b.hit.DocumentType
	})
	rank := 0
	for i := range hits {
		if hits[i].candidate.tier == 2 {
			rank++
			hits[i].hit.Score = 1 / float64(60+rank)
		}
	}
}

func (l *lexicalSearch) page(hits []rankedHit) error {
	offset := l.cursor.Offset
	if offset > len(hits) {
		return fmt.Errorf("%w: cursor offset", service.ErrInvalid)
	}
	end := min(len(hits), offset+l.request.Limit)
	for _, hit := range hits[offset:end] {
		l.result.Hits = append(l.result.Hits, hit.hit)
	}
	if end == len(hits) {
		return nil
	}
	next := l.cursor
	next.Offset = end
	raw, err := json.Marshal(next)
	if err != nil {
		return fmt.Errorf("encode lexical cursor: %w", err)
	}
	l.result.NextCursor = base64.RawURLEncoding.EncodeToString(raw)
	return nil
}

func vectorMatchesFilters(req service.SearchRequest, h service.SearchHit) bool {
	if req.Provider != "" && req.Provider != h.Conversation.Provider || req.Model != "" && req.Model != h.Conversation.Model || req.Status != "" && req.Status != h.Conversation.Status {
		return false
	}
	return (req.Type == "" || req.Type == userRole) && (req.From.IsZero() || !h.CreatedAt.Before(req.From)) && (req.To.IsZero() || h.CreatedAt.Before(req.To)) && req.Dangerous == nil
}

package service

import (
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	modeLexical         = "lexical"
	modeSemantic        = "semantic"
	modeHybrid          = "hybrid"
	typeUser            = "user"
	typeUserMessage     = "user_message"
	typeCommand         = "command"
	typeExplanation     = "explanation"
	maxSearchLimit      = 1000
	rrfK                = 60.0
	cursorPrefix        = "sv1."
	lexicalCursorPrefix = "sl1."
)

var errEmbeddingUnavailable = ErrEmbeddingUnavailable

// SearchService coordinates lexical search and optional user-message embeddings.
// It never backfills embeddings as a side effect of searching.
type SearchService struct {
	store    SearchStore
	embedder Embedder
}

// NewSearch creates a service using store. A nil embedder permits lexical search
// and makes semantic/hybrid requests explicitly fall back to lexical search.
func NewSearch(store SearchStore, embedder Embedder) *SearchService {
	return &SearchService{store: store, embedder: embedder}
}

// Search returns ranked matching documents. Invalid input and stale cursors
// return ErrInvalid; storage and cancellation errors preserve their causes.
// Literal and exact-command searches bypass the embedding provider.
func (s *SearchService) Search(ctx context.Context, request SearchRequest) (SearchResult, error) {
	if err := validateSearch(&request); err != nil {
		return SearchResult{}, err
	}
	if err := ctx.Err(); err != nil {
		return SearchResult{}, fmt.Errorf("search history: %w", err)
	}
	semantic := request.Mode != modeLexical && !request.Literal && request.ExactCommand == ""
	lexicalCursor := strings.HasPrefix(request.Cursor, lexicalCursorPrefix)
	if lexicalCursor {
		request.Cursor = strings.TrimPrefix(request.Cursor, lexicalCursorPrefix)
	}
	if strings.HasPrefix(request.Cursor, cursorPrefix) && !semantic {
		return SearchResult{}, fmt.Errorf("%w: vector cursor requires the original search mode", ErrInvalid)
	}
	if s.embedder != nil && semantic && request.Cursor != "" && !strings.HasPrefix(request.Cursor, cursorPrefix) && !lexicalCursor {
		return SearchResult{}, fmt.Errorf("%w: malformed vector cursor", ErrInvalid)
	}
	if lexicalCursor {
		return s.lexicalSearch(ctx, request)
	}
	if !semantic || request.Cursor != "" && !strings.HasPrefix(request.Cursor, cursorPrefix) {
		return s.lexicalSearch(ctx, request)
	}
	cursor, err := s.readSearchCursor(request)
	if err != nil {
		return SearchResult{}, err
	}
	if s.embedder == nil {
		return s.lexicalSearch(ctx, request)
	}
	result, err := s.vectorSearch(ctx, request, cursor)
	if !errors.Is(err, errEmbeddingUnavailable) {
		return result, err
	}
	if request.Cursor != "" {
		return SearchResult{}, fmt.Errorf("%w: cannot continue vector search while embedding is unavailable", ErrInvalid)
	}
	return s.lexicalSearch(ctx, request)
}

func (s *SearchService) lexicalSearch(ctx context.Context, request SearchRequest) (SearchResult, error) {
	requestedMode := request.Mode
	request.Mode = modeLexical
	if request.Literal || request.ExactCommand != "" {
		request.Fuzzy = "off"
	}
	result, err := s.store.LexicalSearch(ctx, request)
	if err != nil {
		return SearchResult{}, fmt.Errorf("lexical search: %w", err)
	}
	result.RequestedMode, result.Mode = requestedMode, modeLexical
	if requestedMode != modeLexical && !request.Literal && request.ExactCommand == "" {
		result.Degraded = true
		result.Warning = strings.TrimSpace(result.Warning + " Semantic search unavailable; using lexical search.")
		if result.NextCursor != "" {
			result.NextCursor = lexicalCursorPrefix + result.NextCursor
		}
	}
	return result, nil
}

type searchCursor struct {
	Request       string    `json:"request"`
	Profile       string    `json:"profile"`
	VectorProfile string    `json:"vectorProfile"`
	Watermark     int64     `json:"watermark"`
	Score         float64   `json:"score"`
	CreatedAt     time.Time `json:"createdAt"`
	ID            string    `json:"id"`
	Type          string    `json:"type"`
}

func requestFingerprint(request SearchRequest) (string, error) {
	request.Cursor = ""
	request.From, request.To = request.From.UTC(), request.To.UTC()
	data, err := json.Marshal(request)
	if err != nil {
		return "", fmt.Errorf("encode cursor request: %w", err)
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:]), nil
}

func (s *SearchService) readSearchCursor(request SearchRequest) (searchCursor, error) {
	if request.Cursor == "" {
		return searchCursor{}, nil
	}
	if !strings.HasPrefix(request.Cursor, cursorPrefix) || len(request.Cursor) > 4096 {
		return searchCursor{}, fmt.Errorf("%w: malformed vector cursor", ErrInvalid)
	}
	data, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(request.Cursor, cursorPrefix))
	if err != nil {
		return searchCursor{}, fmt.Errorf("%w: malformed vector cursor", ErrInvalid)
	}
	var cursor searchCursor
	if err := json.Unmarshal(data, &cursor); err != nil {
		return searchCursor{}, fmt.Errorf("%w: malformed vector cursor", ErrInvalid)
	}
	fingerprint, err := requestFingerprint(request)
	if err != nil {
		return searchCursor{}, err
	}
	if s.embedder == nil || cursor.ID == "" || cursor.Request != fingerprint || cursor.VectorProfile != s.embedder.ProfileID() {
		return searchCursor{}, fmt.Errorf("%w: vector cursor does not match request/profile", ErrInvalid)
	}
	return cursor, nil
}

func nextSearchCursor(request SearchRequest, cursor searchCursor, last SearchHit) (string, error) {
	fingerprint, err := requestFingerprint(request)
	if err != nil {
		return "", err
	}
	cursor.Request = fingerprint
	cursor.Score, cursor.CreatedAt, cursor.ID, cursor.Type = last.Score, last.CreatedAt, last.DocumentID, last.DocumentType
	data, err := json.Marshal(cursor)
	if err != nil {
		return "", fmt.Errorf("encode vector cursor: %w", err)
	}
	return cursorPrefix + base64.RawURLEncoding.EncodeToString(data), nil
}

func (s *SearchService) checkRevision(ctx context.Context, watermark int64) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("search history: %w", err)
	}
	current, err := s.store.Revision(ctx)
	if err != nil {
		return fmt.Errorf("read search revision: %w", err)
	}
	if current != watermark {
		return fmt.Errorf("%w: search dataset changed; restart search", ErrInvalid)
	}
	return nil
}

func (s *SearchService) vectorSearch(ctx context.Context, request SearchRequest, cursor searchCursor) (SearchResult, error) {
	watermark, err := s.store.Revision(ctx)
	if err != nil {
		return SearchResult{}, fmt.Errorf("read search revision: %w", err)
	}
	if request.Cursor != "" && cursor.Watermark != watermark {
		return SearchResult{}, fmt.Errorf("%w: stale vector cursor", ErrInvalid)
	}
	profile := s.embedder.ProfileID()
	vector, embedErr := s.embedder.Embed(ctx, request.Query)
	if err := s.checkRevision(ctx, watermark); err != nil {
		return SearchResult{}, err
	}
	if embedErr != nil {
		return SearchResult{}, errEmbeddingUnavailable
	}
	norm, err := vectorNorm(vector, s.embedder.Dimensions())
	if err != nil {
		return SearchResult{}, errEmbeddingUnavailable
	}
	if s.embedder.ProfileID() != profile {
		return SearchResult{}, fmt.Errorf("%w: embedding profile changed during search", ErrInvalid)
	}
	coverage, err := s.store.Coverage(ctx, profile)
	if err != nil {
		return SearchResult{}, fmt.Errorf("read embedding coverage: %w", err)
	}
	scan := vectorScan{request: request, vector: vector, norm: norm, profile: profile, cursor: cursor}
	hits, truncated, err := s.scanVectors(ctx, scan)
	if err != nil {
		return SearchResult{}, err
	}
	result := SearchResult{Hits: hits, Mode: request.Mode, RequestedMode: request.Mode, ProfileID: profile, EmbeddingCoverage: coverage, Truncated: truncated}
	if request.Mode == modeHybrid {
		result, err = s.fuseHybrid(ctx, request, result)
		if err != nil {
			return SearchResult{}, err
		}
	}
	if request.Cursor != "" && cursor.Profile != result.ProfileID {
		return SearchResult{}, fmt.Errorf("%w: search profile changed", ErrInvalid)
	}
	if err := s.checkRevision(ctx, watermark); err != nil {
		return SearchResult{}, err
	}
	return pageVectorResult(request, result, searchCursor{Profile: result.ProfileID, VectorProfile: profile, Watermark: watermark}, cursor)
}

type vectorScan struct {
	request SearchRequest
	vector  []float32
	norm    float64
	profile string
	cursor  searchCursor
}

func (s *SearchService) scanVectors(ctx context.Context, scan vectorScan) ([]SearchHit, bool, error) {
	// Semantic keysets retain one page plus a lookahead. Hybrid has a fixed
	// 1000-candidate window per rank list, keeping RRF stable between pages.
	limit := scan.request.Limit + 1
	if scan.request.Mode == modeHybrid {
		limit = maxSearchLimit
	}
	hits := make([]SearchHit, 0, limit)
	count := 0
	err := s.store.VisitVectors(ctx, scan.request, scan.profile, func(candidate VectorCandidate) error {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("scan vectors: %w", err)
		}
		if candidate.Hit.DocumentType != typeUserMessage && candidate.Hit.DocumentType != typeUser {
			return nil
		}
		if candidate.Dimensions != len(scan.vector) {
			return fmt.Errorf("%w: incompatible vector dimensions", ErrInvalid)
		}
		score, err := cosineBlob(scan.vector, scan.norm, candidate.Vector, candidate.Dimensions)
		if err != nil {
			return err
		}
		hit := candidate.Hit
		hit.Score, hit.ScoreKind = score, "cosine"
		hit.MatchSources = []string{modeSemantic}
		if scan.request.Mode != modeHybrid && !afterSearchCursor(hit, scan.cursor) {
			return nil
		}
		count++
		insertTopHit(&hits, hit, limit)
		return nil
	})
	if err != nil {
		return nil, false, fmt.Errorf("visit vectors: %w", err)
	}
	return hits, count > limit, nil
}

func afterSearchCursor(hit SearchHit, cursor searchCursor) bool {
	if cursor.ID == "" {
		return true
	}
	last := SearchHit{DocumentID: cursor.ID, DocumentType: cursor.Type, CreatedAt: cursor.CreatedAt, Score: cursor.Score}
	return compareHits(hit, last) > 0
}

func (s *SearchService) fuseHybrid(ctx context.Context, request SearchRequest, result SearchResult) (SearchResult, error) {
	lexicalRequest := request
	lexicalRequest.Mode, lexicalRequest.Cursor, lexicalRequest.Limit = modeLexical, "", maxSearchLimit
	lexical, err := s.store.LexicalSearch(ctx, lexicalRequest)
	if err != nil {
		return SearchResult{}, fmt.Errorf("hybrid lexical search: %w", err)
	}
	result.Hits = mergeRRF(suppressResponseCopies(lexical.Hits), result.Hits)
	result.Degraded, result.Warning = lexical.Degraded, lexical.Warning
	result.Truncated = result.Truncated || lexical.Truncated
	if lexical.ProfileID != "" {
		result.ProfileID = lexical.ProfileID + "+" + result.ProfileID
	}
	return result, nil
}

func pageVectorResult(request SearchRequest, result SearchResult, next, last searchCursor) (SearchResult, error) {
	if request.Mode == modeHybrid {
		result.Hits = slices.DeleteFunc(result.Hits, func(hit SearchHit) bool { return !afterSearchCursor(hit, last) })
	}
	if len(result.Hits) <= request.Limit {
		return result, nil
	}
	result.Hits = result.Hits[:request.Limit]
	cursor, err := nextSearchCursor(request, next, result.Hits[len(result.Hits)-1])
	if err != nil {
		return SearchResult{}, err
	}
	result.NextCursor = cursor
	return result, nil
}

func insertTopHit(hits *[]SearchHit, hit SearchHit, limit int) {
	index, _ := slices.BinarySearchFunc(*hits, hit, compareHits)
	if index >= limit {
		return
	}
	if len(*hits) == limit {
		*hits = (*hits)[:limit-1]
	}
	*hits = slices.Insert(*hits, index, hit)
}

func compareHits(a, b SearchHit) int {
	if order := cmp.Compare(b.Score, a.Score); order != 0 {
		return order
	}
	if order := b.CreatedAt.Compare(a.CreatedAt); order != 0 {
		return order
	}
	if order := cmp.Compare(a.DocumentID, b.DocumentID); order != 0 {
		return order
	}
	return cmp.Compare(a.DocumentType, b.DocumentType)
}

func suppressResponseCopies(hits []SearchHit) []SearchHit {
	present := make(map[string]bool, len(hits))
	for _, hit := range hits {
		if hit.DocumentType == typeCommand || hit.DocumentType == typeExplanation {
			present[hit.ConversationID] = true
		}
	}
	out := make([]SearchHit, 0, len(hits))
	for _, hit := range hits {
		if hit.DocumentType == "assistant_message" && present[hit.ConversationID] && json.Valid([]byte(hit.Snippet)) {
			continue
		}
		out = append(out, hit)
	}
	return out
}

// mergeRRF uses sum(1/(60+rank)), with one-based ranks. Repeated documents
// contribute at most once per list; the document type is part of identity.
func mergeRRF(lexical, semantic []SearchHit) []SearchHit {
	merged := make(map[string]SearchHit, len(lexical)+len(semantic))
	for _, hits := range [][]SearchHit{lexical, semantic} {
		seen := make(map[string]bool, len(hits))
		for rank, hit := range hits {
			key := hit.DocumentID + "\x00" + hit.DocumentType
			if seen[key] {
				continue
			}
			seen[key] = true
			item, exists := merged[key]
			if !exists {
				item = hit
				item.Score = 0
				item.MatchSources = slices.Clone(hit.MatchSources)
			}
			item.Score += 1 / (rrfK + float64(rank+1))
			item.ScoreKind = "rrf"
			for _, source := range hit.MatchSources {
				if !slices.Contains(item.MatchSources, source) {
					item.MatchSources = append(item.MatchSources, source)
				}
			}
			merged[key] = item
		}
	}
	out := make([]SearchHit, 0, len(merged))
	for _, hit := range merged {
		out = append(out, hit)
	}
	slices.SortFunc(out, compareHits)
	return out
}

// RebuildVectors embeds all pending eligible messages in bounded repository pages.
func (s *SearchService) RebuildVectors(ctx context.Context, retryFailed bool, limit int) (RebuildResult, error) {
	if s.embedder == nil {
		return RebuildResult{}, fmt.Errorf("%w: embedding provider unavailable", ErrInvalid)
	}
	profile := s.embedder.ProfileID()
	model := s.embedder.Model()
	dimensions := s.embedder.Dimensions()
	if profile == "" || model == "" || dimensions <= 0 {
		return RebuildResult{}, fmt.Errorf("%w: invalid embedding profile", ErrInvalid)
	}
	if limit < 0 {
		return RebuildResult{}, fmt.Errorf("%w: rebuild limit", ErrInvalid)
	}
	input := embeddingInput{profile: profile, model: model, dimensions: dimensions}
	after := ""
	var result RebuildResult
	for limit == 0 || result.Ready+result.Failed < limit {
		if err := ctx.Err(); err != nil {
			return result, fmt.Errorf("rebuild vectors: %w", err)
		}
		page := maxSearchLimit
		if limit > 0 {
			page = min(page, limit-result.Ready-result.Failed)
		}
		messages, err := s.store.PendingMessages(ctx, profile, retryFailed, after, page)
		if err != nil {
			return result, fmt.Errorf("load pending messages: %w", err)
		}
		if len(messages) == 0 {
			break
		}
		batch, err := s.rebuildPage(ctx, messages, input)
		result.Ready += batch.Ready
		result.Failed += batch.Failed
		if err != nil {
			return result, err
		}
		next := messages[len(messages)-1].ID
		if next <= after {
			return result, fmt.Errorf("%w: nonadvancing embedding page", ErrInvalid)
		}
		after = next
	}
	return result, nil
}

func (s *SearchService) rebuildPage(ctx context.Context, messages []Message, input embeddingInput) (RebuildResult, error) {
	var result RebuildResult
	for _, message := range messages {
		if message.Role != typeUser || (message.Kind != "initial_prompt" && message.Kind != "refinement") {
			continue
		}
		input.message = message
		embedding, err := s.embedMessage(ctx, input)
		if err != nil {
			return result, err
		}
		if err := s.store.SaveEmbedding(ctx, embedding); err != nil {
			return result, fmt.Errorf("save embedding: %w", err)
		}
		if embedding.Status == "ready" {
			result.Ready++
		} else {
			result.Failed++
		}
	}
	return result, nil
}

type embeddingInput struct {
	message        Message
	profile, model string
	dimensions     int
}

func (s *SearchService) embedMessage(ctx context.Context, input embeddingInput) (Embedding, error) {
	if err := ctx.Err(); err != nil {
		return Embedding{}, fmt.Errorf("rebuild vectors: %w", err)
	}
	digest := sha256.Sum256([]byte(input.message.Content))
	embedding := Embedding{MessageID: input.message.ID, Model: input.model, Dimensions: input.dimensions, ContentHash: hex.EncodeToString(digest[:]), CreatedAt: time.Now().UTC(), ProfileID: input.profile, Status: "ready"}
	vector, embedErr := s.embedder.Embed(ctx, input.message.Content)
	if err := ctx.Err(); err != nil {
		return Embedding{}, fmt.Errorf("rebuild vectors: %w", err)
	}
	if s.embedder.ProfileID() != input.profile || s.embedder.Model() != input.model || s.embedder.Dimensions() != input.dimensions {
		return Embedding{}, fmt.Errorf("%w: embedding profile changed during rebuild", ErrInvalid)
	}
	if embedErr != nil {
		embedding.Status, embedding.Error = "failed", "embedding provider failed"
	} else {
		blob, err := EncodeVector(vector, input.dimensions)
		if err != nil {
			embedding.Status, embedding.Error = "failed", "embedding output invalid"
		} else {
			embedding.Vector = blob
		}
	}
	return embedding, nil
}

func validateSearch(request *SearchRequest) error {
	if request.Mode == "" {
		request.Mode = modeLexical
	}
	if request.Fuzzy == "" {
		request.Fuzzy = "auto"
	}
	if request.Limit == 0 {
		request.Limit = 20
	}
	if request.Mode != modeLexical && request.Mode != modeSemantic && request.Mode != modeHybrid {
		return fmt.Errorf("%w: search mode", ErrInvalid)
	}
	if request.Fuzzy != "auto" && request.Fuzzy != "always" && request.Fuzzy != "off" {
		return fmt.Errorf("%w: fuzzy mode", ErrInvalid)
	}
	switch request.Type {
	case "", typeUser, "assistant", typeCommand, typeExplanation:
	default:
		return fmt.Errorf("%w: document type", ErrInvalid)
	}
	if request.Limit < 0 || request.Limit > maxSearchLimit {
		return fmt.Errorf("%w: search limit must be between 1 and %d", ErrInvalid, maxSearchLimit)
	}
	if !utf8.ValidString(request.Query) || !utf8.ValidString(request.ExactCommand) {
		return fmt.Errorf("%w: query must be UTF-8", ErrInvalid)
	}
	if strings.TrimSpace(request.Query) == "" && request.ExactCommand == "" {
		return fmt.Errorf("%w: empty search query", ErrInvalid)
	}
	if !request.From.IsZero() && !request.To.IsZero() && !request.From.Before(request.To) {
		return fmt.Errorf("%w: empty date interval", ErrInvalid)
	}
	return nil
}

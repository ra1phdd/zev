package service

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"testing"
	"time"
)

const (
	queryFixture        = "query text"
	embeddingPhase      = "embedding phase"
	conversationFixture = "conversation test"
)

func (f *orchestrationStore) Revision(ctx context.Context) (int64, error) {
	return f.base.Revision(ctx)
}

type orchestrationStore struct {
	base            searchFake
	candidates      []VectorCandidate
	coverage        EmbeddingCoverage
	lexicalRequests []SearchRequest
	vectorRequests  []SearchRequest
	visit           func(context.Context, func(VectorCandidate) error) error
	pending         func(string, bool, string, int) ([]Message, error)
	saved           []Embedding
	saveError       error
}

func (f *orchestrationStore) LexicalSearch(_ context.Context, request SearchRequest) (SearchResult, error) {
	f.lexicalRequests = append(f.lexicalRequests, request)
	return f.base.result, nil
}

func (f *orchestrationStore) Coverage(context.Context, string) (EmbeddingCoverage, error) {
	return f.coverage, nil
}

func (f *orchestrationStore) VisitVectors(ctx context.Context, request SearchRequest, _ string, visit func(VectorCandidate) error) error {
	f.vectorRequests = append(f.vectorRequests, request)
	if f.visit != nil {
		return f.visit(ctx, visit)
	}
	for _, candidate := range f.candidates {
		if err := visit(candidate); err != nil {
			return err
		}
	}
	return nil
}

func (f *orchestrationStore) PendingMessages(_ context.Context, profile string, retry bool, after string, limit int) ([]Message, error) {
	if f.pending != nil {
		return f.pending(profile, retry, after, limit)
	}
	return nil, nil
}

func (f *orchestrationStore) SaveEmbedding(_ context.Context, embedding Embedding) error {
	f.saved = append(f.saved, embedding)
	return f.saveError
}

type orchestrationEmbedder struct {
	embed func(context.Context, string) ([]float32, error)
}

func (e orchestrationEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	return e.embed(ctx, text)
}
func (orchestrationEmbedder) ProfileID() string { return "test-profile" }
func (orchestrationEmbedder) Model() string     { return "test-model" }
func (orchestrationEmbedder) Dimensions() int   { return 2 }

func vectorCandidate(t *testing.T, id string, created time.Time, values ...float32) VectorCandidate {
	t.Helper()
	blob, err := EncodeVector(values, 2)
	if err != nil {
		t.Fatal(err)
	}
	return VectorCandidate{Hit: SearchHit{DocumentID: id, MessageID: id, DocumentType: "user_message", Snippet: "original " + id, CreatedAt: created}, Vector: blob, Dimensions: 2}
}

func TestSemanticSearchStreamsBoundedRankedUserDocuments(t *testing.T) {
	t.Parallel()
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	f := &orchestrationStore{coverage: EmbeddingCoverage{Eligible: 4, Ready: 4}, candidates: []VectorCandidate{
		vectorCandidate(t, "low", created, 0, 1), vectorCandidate(t, "b", created, 1, 0),
		vectorCandidate(t, "a", created, 1, 0), vectorCandidate(t, "new", created.Add(time.Second), 1, 0),
	}}
	e := orchestrationEmbedder{embed: func(_ context.Context, text string) ([]float32, error) {
		if text != "exact query" {
			t.Errorf("embedding text %q", text)
		}
		return []float32{1, 0}, nil
	}}
	result, err := NewSearch(f, e).Search(t.Context(), SearchRequest{Query: "exact query", Mode: modeSemantic, Limit: 2, Provider: "provider", Type: typeUser})
	if err != nil {
		t.Fatal(err)
	}
	if result.Mode != modeSemantic || result.Degraded || result.EmbeddingCoverage.Ready != 4 || len(result.Hits) != 2 {
		t.Fatalf("unexpected semantic result: %+v", result)
	}
	if result.Hits[0].DocumentID != "new" || result.Hits[1].DocumentID != "a" || result.Hits[1].Snippet != "original a" || result.Hits[0].ScoreKind != "cosine" || !slices.Contains(result.Hits[0].MatchSources, modeSemantic) {
		t.Fatalf("wrong ranking/document: %+v", result.Hits)
	}
	if len(f.lexicalRequests) != 0 || len(f.vectorRequests) != 1 || f.vectorRequests[0].Provider != "provider" {
		t.Fatalf("incorrect routing: %+v", f)
	}
}

func TestHybridUsesRRFAndPreservesLexicalMetadata(t *testing.T) {
	t.Parallel()
	f := &orchestrationStore{
		base: searchFake{result: SearchResult{ProfileID: "lexical-profile", Degraded: true, Warning: "lexical warning", Truncated: true, Hits: []SearchHit{
			{DocumentID: "b", MessageID: "b", DocumentType: "user_message", Score: -100, MatchSources: []string{"exact"}},
			{DocumentID: typeCommand, DocumentType: typeCommand, Score: -1, MatchSources: []string{"morphology"}},
		}}},
		coverage:   EmbeddingCoverage{Eligible: 2, Ready: 2},
		candidates: []VectorCandidate{vectorCandidate(t, "a", time.Time{}, 1, 0), vectorCandidate(t, "b", time.Time{}, 0, 1)},
	}
	e := orchestrationEmbedder{embed: func(context.Context, string) ([]float32, error) { return []float32{1, 0}, nil }}
	result, err := NewSearch(f, e).Search(t.Context(), SearchRequest{Query: searchFilesFixture, Mode: modeHybrid, Limit: 3})
	if err != nil {
		t.Fatal(err)
	}
	if result.Mode != modeHybrid || len(result.Hits) != 3 || !result.Degraded || result.Warning != "lexical warning" || !result.Truncated {
		t.Fatalf("metadata/result: %+v", result)
	}
	if result.Hits[0].DocumentID != "b" || result.Hits[0].ScoreKind != "rrf" || math.Abs(result.Hits[0].Score-(1.0/61+1.0/62)) > 1e-10 || !slices.Equal(result.Hits[0].MatchSources, []string{"exact", modeSemantic}) {
		t.Fatalf("wrong fusion: %+v", result.Hits)
	}
}

func TestSearchFallbackAndCancellation(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name         string
		request      SearchRequest
		embed        func(context.Context, string) ([]float32, error)
		wantDegraded bool
	}{
		{"provider failure", SearchRequest{Query: queryFixture, Mode: modeSemantic}, func(context.Context, string) ([]float32, error) { return nil, errors.New("private provider payload") }, true},
		{"invalid vector", SearchRequest{Query: queryFixture, Mode: modeHybrid}, func(context.Context, string) ([]float32, error) { return []float32{0, 0}, nil }, true},
		{"literal", SearchRequest{Query: "--flag", Mode: modeHybrid, Literal: true}, nil, false},
		{"exact command", SearchRequest{ExactCommand: "git status", Mode: modeSemantic}, nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &orchestrationStore{base: searchFake{result: SearchResult{Hits: []SearchHit{{DocumentID: modeLexical}}}}}
			e := orchestrationEmbedder{embed: tc.embed}
			result, err := NewSearch(f, e).Search(t.Context(), tc.request)
			if err != nil || result.Degraded != tc.wantDegraded || result.Mode != modeLexical || len(result.Hits) != 1 {
				t.Fatalf("result %+v, error %v", result, err)
			}
			if tc.wantDegraded && result.Warning == "" {
				t.Fatal("silent fallback")
			}
			if len(f.lexicalRequests) != 1 || f.lexicalRequests[0].Mode != modeLexical {
				t.Fatalf("lexical request: %+v", f.lexicalRequests)
			}
		})
	}
	ctx, cancel := context.WithCancel(t.Context())
	f := &orchestrationStore{visit: func(_ context.Context, visit func(VectorCandidate) error) error {
		cancel()
		return visit(VectorCandidate{})
	}}
	e := orchestrationEmbedder{embed: func(context.Context, string) ([]float32, error) { return []float32{1, 0}, nil }}
	if _, err := NewSearch(f, e).Search(ctx, SearchRequest{Query: "x", Mode: modeSemantic}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation lost: %v", err)
	}
}

func TestSemanticCursorBindsRequestProfileAndWatermark(t *testing.T) {
	t.Parallel()
	f := &orchestrationStore{candidates: []VectorCandidate{vectorCandidate(t, "a", time.Time{}, 1, 0), vectorCandidate(t, "b", time.Time{}, 1, 0), vectorCandidate(t, "c", time.Time{}, 1, 0)}}
	e := orchestrationEmbedder{embed: func(context.Context, string) ([]float32, error) { return []float32{1, 0}, nil }}
	s := NewSearch(f, e)
	request := SearchRequest{Query: queryFixture, Mode: modeSemantic, Limit: 1}
	first, err := s.Search(t.Context(), request)
	if err != nil || first.NextCursor == "" || len(first.Hits) != 1 {
		t.Fatalf("page one: %+v, %v", first, err)
	}
	request.Cursor = first.NextCursor
	second, err := s.Search(t.Context(), request)
	if err != nil || len(second.Hits) != 1 || second.Hits[0].DocumentID != "b" {
		t.Fatalf("page two: %+v, %v", second, err)
	}
	for _, mutate := range []func(*SearchRequest){func(r *SearchRequest) { r.Query = "changed" }, func(r *SearchRequest) { r.Status = "failed" }, func(r *SearchRequest) { r.Literal = true }, func(r *SearchRequest) { r.From = time.Now() }, func(r *SearchRequest) { r.Fuzzy = "off" }, func(r *SearchRequest) { r.Cursor = cursorPrefix + "garbage" }} {
		changed := request
		mutate(&changed)
		if _, err := s.Search(t.Context(), changed); !errors.Is(err, ErrInvalid) {
			t.Errorf("accepted changed request: %+v, %v", changed, err)
		}
	}
	f.base.revision++
	if _, err := s.Search(t.Context(), request); !errors.Is(err, ErrInvalid) {
		t.Errorf("accepted stale revision: %v", err)
	}
}

func TestSearchRejectsDatasetChangeDuringEmbeddingOrScan(t *testing.T) {
	t.Parallel()
	for _, phase := range []string{embeddingPhase, "scan"} {
		t.Run(phase, func(t *testing.T) {
			f := &orchestrationStore{}
			e := orchestrationEmbedder{embed: func(context.Context, string) ([]float32, error) {
				if phase == embeddingPhase {
					f.base.revision++
				}
				return []float32{1, 0}, nil
			}}
			f.visit = func(context.Context, func(VectorCandidate) error) error {
				if phase == "scan" {
					f.base.revision++
				}
				return nil
			}
			if _, err := NewSearch(f, e).Search(t.Context(), SearchRequest{Query: "q", Mode: modeSemantic}); !errors.Is(err, ErrInvalid) {
				t.Fatalf("accepted concurrent mutation: %v", err)
			}
		})
	}
}

func TestHybridDeduplicatesByDocumentTypeAndNotResponseCopy(t *testing.T) {
	t.Parallel()
	const sharedID = "same"
	f := &orchestrationStore{base: searchFake{result: SearchResult{ProfileID: "lexical-profile", Hits: []SearchHit{
		{DocumentID: sharedID, DocumentType: typeCommand, CommandID: sharedID, ConversationID: conversationFixture, Snippet: "git status"},
		{DocumentID: sharedID, DocumentType: "explanation", CommandID: sharedID, ConversationID: conversationFixture, Snippet: "show status"},
		{DocumentID: "assistant", DocumentType: "assistant_message", ConversationID: conversationFixture, Snippet: `{"commands":[{"command":"git status"}]}`},
	}}}}
	e := orchestrationEmbedder{embed: func(context.Context, string) ([]float32, error) { return []float32{1, 0}, nil }}
	result, err := NewSearch(f, e).Search(t.Context(), SearchRequest{Query: "status", Mode: modeHybrid})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Hits) != 2 || result.ProfileID != "lexical-profile+test-profile" {
		t.Fatalf("dedup/profile: %+v", result)
	}
}

func TestRebuildVectorsPagesAndRecordsSafeFailure(t *testing.T) {
	t.Parallel()
	f := &orchestrationStore{}
	pages := 0
	f.pending = func(profile string, retry bool, after string, limit int) ([]Message, error) {
		pages++
		if profile != "test-profile" || !retry || limit <= 0 || limit > 1000 {
			t.Fatalf("invalid page request: %s %v %s %d", profile, retry, after, limit)
		}
		switch after {
		case "":
			return []Message{{ID: "a", Role: typeUser, Kind: "initial_prompt", Content: " точный\n"}, {ID: "b", Role: "assistant", Kind: "response", Content: "private"}}, nil
		case "b":
			return []Message{{ID: "c", Role: typeUser, Kind: "refinement", Content: "failure"}}, nil
		case "c":
			return nil, nil
		default:
			t.Fatalf("unexpected after %q", after)
			return nil, nil
		}
	}
	e := orchestrationEmbedder{embed: func(_ context.Context, text string) ([]float32, error) {
		if text == "failure" {
			return nil, errors.New("secret private payload")
		}
		if text != " точный\n" {
			t.Errorf("embedded nonuser %q", text)
		}
		return []float32{1, 2}, nil
	}}
	result, err := NewSearch(f, e).RebuildVectors(t.Context(), true, 0)
	if err != nil || result.Ready != 1 || result.Failed != 1 || len(f.saved) != 2 || pages != 3 {
		t.Fatalf("rebuild %+v, %v, saved %+v pages %d", result, err, f.saved, pages)
	}
	if f.saved[0].ContentHash != fmt.Sprintf("%x", sha256.Sum256([]byte(" точный\n"))) || f.saved[1].Status != "failed" || f.saved[1].Error == "" || len(f.saved[1].Vector) != 0 || strings.Contains(f.saved[1].Error, "secret") {
		t.Fatalf("unsafe/incorrect embeddings: %+v", f.saved)
	}
}

func TestRebuildCancellationAndSaveFailureDoNotCountSuccess(t *testing.T) {
	t.Parallel()
	for _, phase := range []string{embeddingPhase, "save"} {
		t.Run(phase, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			f := &orchestrationStore{pending: func(string, bool, string, int) ([]Message, error) {
				return []Message{{ID: "a", Role: typeUser, Kind: "initial_prompt", Content: "x"}}, nil
			}}
			e := orchestrationEmbedder{embed: func(context.Context, string) ([]float32, error) {
				if phase == embeddingPhase {
					cancel()
					return nil, ctx.Err()
				}
				return []float32{1, 0}, nil
			}}
			if phase == "save" {
				f.saveError = errors.New("save failed")
			}
			result, err := NewSearch(f, e).RebuildVectors(ctx, false, 1)
			if err == nil || result.Ready != 0 || result.Failed != 0 {
				t.Fatalf("counted uncommitted outcome %+v, %v", result, err)
			}
			if phase == embeddingPhase && (len(f.saved) != 0 || !errors.Is(err, context.Canceled)) {
				t.Fatalf("cancellation: saved %+v, %v", f.saved, err)
			}
		})
	}
}

func BenchmarkCosineFixedDataset(b *testing.B) {
	const dimensions = 384
	query := make([]float32, dimensions)
	for i := range query {
		query[i] = float32(i+1) / dimensions
	}
	blobs := make([][]byte, 1000)
	for i := range blobs {
		vector := make([]float32, dimensions)
		for j := range vector {
			vector[j] = float32((i+j)%17 + 1)
		}
		var err error
		blobs[i], err = EncodeVector(vector, dimensions)
		if err != nil {
			b.Fatal(err)
		}
	}
	b.ReportAllocs()
	for b.Loop() {
		for _, blob := range blobs {
			if _, err := cosineForTest(query, blob, dimensions); err != nil {
				b.Fatal(err)
			}
		}
	}
}

func TestSemanticInvalidCursorDoesNotCallEmbedder(t *testing.T) {
	t.Parallel()
	calls := 0
	e := orchestrationEmbedder{embed: func(context.Context, string) ([]float32, error) { calls++; return []float32{1, 0}, nil }}
	_, err := NewSearch(&orchestrationStore{}, e).Search(t.Context(), SearchRequest{Query: "q", Mode: modeSemantic, Cursor: "invalid"})
	if !errors.Is(err, ErrInvalid) || calls != 0 {
		t.Fatalf("invalid cursor: %v calls %d", err, calls)
	}
}

func TestCosineRejectsInvalidQueryAndBlob(t *testing.T) {
	t.Parallel()
	valid, err := EncodeVector([]float32{1, 0}, 2)
	if err != nil {
		t.Fatal(err)
	}
	nan := slices.Clone(valid)
	binary.LittleEndian.PutUint32(nan, math.Float32bits(float32(math.NaN())))
	for _, tc := range []struct {
		query []float32
		blob  []byte
		dims  int
	}{
		{nil, valid, 2},
		{[]float32{0, 0}, valid, 2},
		{[]float32{1, 0}, make([]byte, 8), 2},
		{[]float32{1, 0}, nan, 2},
		{[]float32{1, 0}, valid, -1},
	} {
		if _, err := cosineForTest(tc.query, tc.blob, tc.dims); !errors.Is(err, ErrInvalid) {
			t.Errorf("invalid cosine: %v", err)
		}
	}
	if binary.LittleEndian.Uint32(valid) != math.Float32bits(1) {
		t.Fatal("encoding is not little-endian")
	}
}

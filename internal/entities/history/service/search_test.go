package service

import (
	"context"
	"errors"
	"math"
	"testing"
)

func TestVectorEncodingAndCosine(t *testing.T) {
	t.Parallel()
	for _, vector := range [][]float32{nil, {0, 0}, {float32(math.NaN()), 1}, {float32(math.Inf(1)), 1}} {
		if _, err := EncodeVector(vector, 2); err == nil {
			t.Errorf("accepted invalid vector %v", vector)
		}
	}
	blob, err := EncodeVector([]float32{3, 4}, 2)
	if err != nil {
		t.Fatal(err)
	}
	score, err := Cosine([]float32{3, 4}, blob, 2)
	if err != nil || math.Abs(score-1) > 1e-6 {
		t.Fatalf("cosine %v, %v", score, err)
	}
	if _, err := Cosine([]float32{1, 2}, blob[:7], 2); err == nil {
		t.Fatal("accepted wrong blob size")
	}
}

const searchFilesFixture = "files"

type searchFake struct {
	result   SearchResult
	revision int64
}

func (f *searchFake) LexicalSearch(context.Context, SearchRequest) (SearchResult, error) {
	return f.result, nil
}
func (f *searchFake) Revision(context.Context) (int64, error) { return f.revision, nil }
func (*searchFake) Coverage(context.Context, string) (EmbeddingCoverage, error) {
	return EmbeddingCoverage{}, nil
}

func (*searchFake) VisitVectors(context.Context, SearchRequest, string, func(VectorCandidate) error) error {
	return nil
}

func (*searchFake) PendingMessages(context.Context, string, bool, string, int) ([]Message, error) {
	return nil, nil
}
func (*searchFake) SaveEmbedding(context.Context, Embedding) error { return nil }

func TestSemanticUnavailableFallbackIsExplicit(t *testing.T) {
	t.Parallel()
	f := &searchFake{result: SearchResult{Hits: []SearchHit{{DocumentID: "document"}}, Mode: "lexical"}}
	result, err := NewSearch(f, nil).Search(t.Context(), SearchRequest{Query: searchFilesFixture, Mode: "semantic"})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Degraded || result.Mode != "lexical" || result.Warning == "" || len(result.Hits) != 1 {
		t.Fatalf("silent fallback: %+v", result)
	}
}

func TestSemanticUnavailableFallbackPaginatesWithLexicalCursor(t *testing.T) {
	t.Parallel()
	store := &fallbackPaginationStore{}
	search := NewSearch(store, nil)
	request := SearchRequest{Query: searchFilesFixture, Mode: modeSemantic, Limit: 1}
	first, err := search.Search(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	if first.Mode != modeLexical || first.RequestedMode != modeSemantic || first.NextCursor == "" || len(first.Hits) != 1 || first.Hits[0].DocumentID != "first" {
		t.Fatalf("first page: %+v", first)
	}
	search.embedder = orchestrationEmbedder{embed: func(context.Context, string) ([]float32, error) {
		t.Fatal("fallback cursor must not call embedder")
		return nil, ErrEmbeddingUnavailable
	}}
	request.Cursor = first.NextCursor
	second, err := search.Search(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	if second.Mode != modeLexical || second.RequestedMode != modeSemantic || second.NextCursor != "" || len(second.Hits) != 1 || second.Hits[0].DocumentID != "second" {
		t.Fatalf("second page: %+v", second)
	}
	if len(store.requests) != 2 || store.requests[1].Cursor != "lexical-page-2" || store.requests[1].Mode != modeLexical {
		t.Fatalf("lexical pagination requests: %+v", store.requests)
	}
}

type fallbackPaginationStore struct {
	searchFake
	requests []SearchRequest
}

func (f *fallbackPaginationStore) LexicalSearch(_ context.Context, request SearchRequest) (SearchResult, error) {
	f.requests = append(f.requests, request)
	if request.Cursor == "" {
		return SearchResult{Hits: []SearchHit{{DocumentID: "first"}}, NextCursor: "lexical-page-2"}, nil
	}
	return SearchResult{Hits: []SearchHit{{DocumentID: "second"}}}, nil
}

func TestSearchValidatesModesAndLiteral(t *testing.T) {
	t.Parallel()
	for _, request := range []SearchRequest{{}, {Query: "x", Mode: "unknown"}, {Query: "x", Fuzzy: "unknown"}, {Query: "x", Type: "system"}, {Query: "x", Limit: -1}} {
		if _, err := NewSearch(&searchFake{}, nil).Search(t.Context(), request); !errors.Is(err, ErrInvalid) {
			t.Errorf("request %+v: %v", request, err)
		}
	}
}

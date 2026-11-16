package repository

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"math"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/ra1phdd/zev/internal/entities/history/service"
)

func TestSearchAndEmbeddingLifecycle(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "private", "h.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	})
	_, err = s.StartConversation(ctx, service.StartConversationInput{Conversation: service.Conversation{ID: "c"}, InitialMessage: service.Message{ID: "m", Role: userRole, Kind: initialPromptKind, Content: "find files"}})
	if err != nil {
		t.Fatal(err)
	}
	result, err := s.LexicalSearch(ctx, service.SearchRequest{Query: filesFixture, Limit: 10})
	if err != nil || len(result.Hits) != 1 || result.Hits[0].MessageID != "m" {
		t.Fatalf("search: %+v %v", result, err)
	}
	pending, err := s.PendingMessages(ctx, "profile", false, "", 10)
	if err != nil || len(pending) != 1 {
		t.Fatalf("pending: %+v %v", pending, err)
	}
	hash := sha256.Sum256([]byte("find files"))
	vector := make([]byte, 8)
	binary.LittleEndian.PutUint32(vector, math.Float32bits(1))
	binary.LittleEndian.PutUint32(vector[4:], math.Float32bits(2))
	err = s.SaveEmbedding(ctx, service.Embedding{MessageID: "m", ProfileID: "profile", Model: modelFixture, Dimensions: 2, Vector: vector, ContentHash: hex.EncodeToString(hash[:]), Status: readyEmbedding})
	if err != nil {
		t.Fatal(err)
	}
	coverage, err := s.Coverage(ctx, "profile")
	if err != nil || coverage.Eligible != 1 || coverage.Ready != 1 {
		t.Fatalf("coverage %+v %v", coverage, err)
	}
	count := 0
	err = s.VisitVectors(ctx, service.SearchRequest{}, "profile", func(c service.VectorCandidate) error {
		count++
		if c.Hit.MessageID != "m" || len(c.Vector) != 8 {
			t.Errorf("candidate %+v", c)
		}
		return nil
	})
	if err != nil || count != 1 {
		t.Fatalf("vectors %d %v", count, err)
	}
	if err = s.DeleteConversation(ctx, "c"); err != nil {
		t.Fatal(err)
	}
	coverage, err = s.Coverage(ctx, "profile")
	if err != nil || coverage.Eligible != 0 {
		t.Fatalf("coverage after delete %+v %v", coverage, err)
	}
}

func TestLexicalMorphologyTypoAndCursor(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "private", "h.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	})
	for i, text := range []string{"файлов docker", "файла docker", "unrelated"} {
		id := strconv.Itoa(i)
		_, err = s.StartConversation(ctx, service.StartConversationInput{Conversation: service.Conversation{ID: id, Provider: "p"}, InitialMessage: service.Message{ID: "m" + id, Role: userRole, Kind: initialPromptKind, Content: text}})
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, q := range []string{"файл", "фалй", "dokcer"} {
		r, e := s.LexicalSearch(ctx, service.SearchRequest{Query: q, Limit: 10, Fuzzy: fuzzyAlways})
		if e != nil || len(r.Hits) != 2 {
			t.Errorf("%q: %+v %v", q, r, e)
		}
	}
	req := service.SearchRequest{Query: "docker", Limit: 1}
	page, err := s.LexicalSearch(ctx, req)
	if err != nil || len(page.Hits) != 1 || page.NextCursor == "" {
		t.Fatalf("first: %+v %v", page, err)
	}
	req.Cursor = page.NextCursor
	next, err := s.LexicalSearch(ctx, req)
	if err != nil || len(next.Hits) != 1 || next.Hits[0].DocumentID == page.Hits[0].DocumentID {
		t.Fatalf("second: %+v %v", next, err)
	}
	_, err = s.AppendMessage(ctx, service.AppendMessageInput{Message: service.Message{ID: "new", ConversationID: "0", Role: userRole, Kind: refinementKind, Content: "new"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.LexicalSearch(ctx, req); !errors.Is(err, service.ErrConflict) {
		t.Fatalf("stale cursor: %v", err)
	}
}

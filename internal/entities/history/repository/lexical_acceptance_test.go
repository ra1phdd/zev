package repository

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/ra1phdd/zev/internal/entities/history/service"
)

func lexicalTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(t.Context(), filepath.Join(t.TempDir(), "private", "history.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	})
	return s
}

func lexicalTestMessage(t *testing.T, s *Store, id, text string) {
	t.Helper()
	_, err := s.StartConversation(t.Context(), service.StartConversationInput{
		Conversation:   service.Conversation{ID: "c-" + id},
		InitialMessage: service.Message{ID: id, Role: userRole, Kind: initialPromptKind, Content: text},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestLexicalTypoPreservesAllTerms(t *testing.T) {
	t.Parallel()
	s := lexicalTestStore(t)
	lexicalTestMessage(t, s, "both", "файлов docker")
	lexicalTestMessage(t, s, "one", "файлов unrelated")
	lexicalTestMessage(t, s, "other", "docker unrelated")
	result, err := s.LexicalSearch(t.Context(), service.SearchRequest{Query: "фалй doker", Fuzzy: fuzzyAlways})
	if err != nil || len(result.Hits) != 1 || result.Hits[0].MessageID != "both" {
		t.Fatalf("all-term typo search = %+v, %v", result, err)
	}
}

func TestLexicalReadsOnlyIndexedCandidates(t *testing.T) {
	t.Parallel()
	s := lexicalTestStore(t)
	lexicalTestMessage(t, s, wantedFixture, "needle")
	lexicalTestMessage(t, s, "other", "unrelated")
	if _, err := s.db.ExecContext(t.Context(), `UPDATE history_documents SET created_at='invalid' WHERE message_id='other'`); err != nil {
		t.Fatal(err)
	}
	result, err := s.LexicalSearch(t.Context(), service.SearchRequest{Query: "needle", Fuzzy: fuzzyOff})
	if err != nil || len(result.Hits) != 1 || result.Hits[0].MessageID != wantedFixture {
		t.Fatalf("unrelated document was read: %+v, %v", result, err)
	}
}

func TestLexicalResponseTextAndExplanationIndex(t *testing.T) {
	t.Parallel()
	s := lexicalTestStore(t)
	lexicalTestMessage(t, s, initialFixture, initialFixture)
	payload := `{"is_valid":true,"commands":[{"command":"echo uniquecommand","short_explanation":"резервные копии","dangerous_explanation":"irreversible deletion"}]}`
	in := service.RecordResponseInput{Message: service.Message{ID: assistantRole, Role: assistantRole, Kind: responseKind, Content: payload}, Response: service.Response{ID: responseKind, ConversationID: "c-initial", PayloadJSON: payload, IsValid: true, Commands: []service.Command{{ID: commandDocument, Command: "echo uniquecommand", ShortExplanation: "резервные копии", DangerousExplanation: "irreversible deletion"}}}}
	if _, err := s.RecordResponse(t.Context(), in); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ query, typ, id string }{
		{"резервная копия", "explanation", commandDocument},
		{"irreversible", "explanation", commandDocument},
		{"uniquecommand", commandDocument, commandDocument},
	} {
		t.Run(tc.query, func(t *testing.T) {
			r, err := s.LexicalSearch(t.Context(), service.SearchRequest{Query: tc.query, Fuzzy: fuzzyOff})
			if err != nil || len(r.Hits) != 1 || r.Hits[0].DocumentType != tc.typ || r.Hits[0].DocumentID != tc.id {
				t.Fatalf("index = %+v, %v", r, err)
			}
		})
	}
	if _, err := s.RecordResponse(t.Context(), service.RecordResponseInput{Message: service.Message{ID: "rejected", Role: assistantRole, Kind: responseKind, Content: `{"is_valid":false,"explanation_if_not_valid":"ambiguous request"}`}, Response: service.Response{ID: "rejection", ConversationID: "c-initial", PayloadJSON: `{"is_valid":false,"explanation_if_not_valid":"ambiguous request"}`}}); err != nil {
		t.Fatal(err)
	}
	r, err := s.LexicalSearch(t.Context(), service.SearchRequest{Query: "ambiguous", Type: assistantRole, Fuzzy: fuzzyOff})
	if err != nil || len(r.Hits) != 1 || r.Hits[0].DocumentType != "assistant_message" || r.Hits[0].Snippet != "ambiguous request" {
		t.Fatalf("assistant = %+v, %v", r, err)
	}
}

func TestLexicalTypoSimilarityOrdering(t *testing.T) {
	t.Parallel()
	s := lexicalTestStore(t)
	lexicalTestMessage(t, s, "worse", "abcxefyh")
	lexicalTestMessage(t, s, "better", "abcxefgh")
	lexicalTestMessage(t, s, "exact", "abcdefgh")
	r, err := s.LexicalSearch(t.Context(), service.SearchRequest{Query: "abcdefgh", Fuzzy: fuzzyAlways})
	if err != nil || len(r.Hits) != 3 || r.Hits[0].MessageID != "exact" || r.Hits[1].MessageID != "better" {
		t.Fatalf("similarity order: %+v %v", r, err)
	}
	if r.Hits[1].Score != 1.0/61 || r.Hits[2].Score != 1.0/62 {
		t.Fatalf("typo list was not RRF-ranked: %+v", r.Hits)
	}
}

func TestLexicalFiltersBeforeCandidateLimit(t *testing.T) {
	t.Parallel()
	s := lexicalTestStore(t)
	tx, err := s.db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for i := range 5001 {
		id := fmt.Sprintf("bulk-%05d", i)
		provider := "noise"
		if i == 5000 {
			provider = wantedFixture
		}
		created := "2026-01-01T00:00:00.000000000Z"
		if _, err = tx.ExecContext(t.Context(), `INSERT INTO conversations(id,created_at,updated_at,status,kind,provider,model) VALUES(?,?,?,'started','query',?,'model')`, id, created, created, provider); err != nil {
			t.Fatal(err)
		}
		if _, err = appendMessage(t.Context(), tx, service.Message{ID: "m-" + id, ConversationID: id, Role: userRole, Kind: initialPromptKind, Content: commonFixture, CreatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}); err != nil {
			t.Fatal(err)
		}
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	r, err := s.LexicalSearch(t.Context(), service.SearchRequest{Query: commonFixture, Provider: wantedFixture, Type: userRole, Fuzzy: fuzzyOff})
	if err != nil || len(r.Hits) != 1 || r.Hits[0].Conversation.Provider != wantedFixture {
		t.Fatalf("filtered topK: %+v %v", r, err)
	}
	r, err = s.LexicalSearch(t.Context(), service.SearchRequest{Query: commonFixture, Fuzzy: fuzzyOff})
	if err != nil || !r.Truncated {
		t.Fatalf("unreported cap: %+v %v", r, err)
	}
}

func TestLexicalAcceptanceFormsAndTypos(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		content string
		queries []string
	}{
		{"файл", []string{"файла", "файлы", "файлов", "фалй", "фаайл", "файр"}},
		{"резервные копии файлов", []string{"резервная копия файл", "резервная копяи фалй"}},
		{"ёлка", []string{"елка", "ёлки"}},
		{filesFixture, []string{"file", "fi" + "els", "fiiles", "fles", "fils"}},
		{"connected connecting", []string{"connect", "connecting", "connectign", "con" + "ected"}},
		{"cat", []string{"cat"}},
	} {
		t.Run(tc.content, func(t *testing.T) {
			t.Parallel()
			s := lexicalTestStore(t)
			lexicalTestMessage(t, s, "fixture", tc.content)
			for _, query := range tc.queries {
				r, err := s.LexicalSearch(t.Context(), service.SearchRequest{Query: query, Fuzzy: fuzzyAlways})
				if err != nil || len(r.Hits) != 1 {
					t.Errorf("%q => %d hits, %v", query, len(r.Hits), err)
				}
			}
		})
	}
}

func TestLexicalRejectsShortFuzzyTokens(t *testing.T) {
	t.Parallel()
	s := lexicalTestStore(t)
	lexicalTestMessage(t, s, "fixture", "cats файл")
	for _, query := range []string{"cta", "фат", "фай"} {
		r, err := s.LexicalSearch(t.Context(), service.SearchRequest{Query: query, Fuzzy: fuzzyAlways})
		if err != nil || len(r.Hits) != 0 {
			t.Errorf("%q => %+v %v", query, r, err)
		}
	}
}

func TestLexicalLiteralSyntaxAndCleanup(t *testing.T) {
	t.Parallel()
	s := lexicalTestStore(t)
	lexicalTestMessage(t, s, "literal", `find --name a_b% "AND" /tmp/file.txt`)
	lexicalTestMessage(t, s, "noise", `find --name axbz`)
	for _, q := range []string{"--name a_b%", `"AND"`, "/tmp/file.txt"} {
		r, err := s.LexicalSearch(t.Context(), service.SearchRequest{Query: q, Literal: true})
		if err != nil || len(r.Hits) != 1 || r.Hits[0].MessageID != "literal" {
			t.Errorf("literal %q = %+v %v", q, r, err)
		}
	}
	for _, q := range []string{`"AND"`, `file* OR NOT`, `"`, `a_b%`} {
		if _, err := s.LexicalSearch(t.Context(), service.SearchRequest{Query: q, Fuzzy: fuzzyOff}); err != nil {
			t.Errorf("syntax %q: %v", q, err)
		}
	}
	if err := s.Clear(t.Context()); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"history_tokens", "history_deletions", "history_fts_surface", morphologyTable, "history_fts_trigram"} {
		var n int
		if err := s.db.QueryRowContext(t.Context(), "SELECT count(*) FROM "+table).Scan(&n); err != nil || n != 0 {
			t.Errorf("cleanup %s: %d %v", table, n, err)
		}
	}
}

func TestLexicalDetectsStaleDerivedIndexes(t *testing.T) {
	t.Parallel()
	s := lexicalTestStore(t)
	lexicalTestMessage(t, s, "fixture", filesFixture)
	if _, err := s.db.ExecContext(t.Context(), `UPDATE history_documents SET profile_id='old-profile'`); err != nil {
		t.Fatal(err)
	}
	r, err := s.LexicalSearch(t.Context(), service.SearchRequest{Query: filesFixture, Fuzzy: fuzzyOff})
	if err != nil || !r.Degraded || r.Warning == "" {
		t.Fatalf("stale analyzer silently used: %+v %v", r, err)
	}
}

func TestLexicalFiltersAndExactCommand(t *testing.T) {
	t.Parallel()
	s := lexicalTestStore(t)
	lexicalTestMessage(t, s, "fixture", initialFixture)
	created := time.Date(2026, 4, 5, 6, 7, 8, 0, time.UTC)
	_, err := s.RecordResponse(t.Context(), service.RecordResponseInput{Message: service.Message{ID: "answer", Role: assistantRole, Kind: responseKind, Content: "plain response", CreatedAt: created}, Response: service.Response{ID: "response-filter", ConversationID: "c-fixture", Provider: providerFixture, Model: modelFixture, PayloadJSON: `{}`, CreatedAt: created, Commands: []service.Command{{ID: "command-filter", Command: `echo --name a_b%`, IsDangerous: true}}}})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.CompleteConversation(t.Context(), service.CompleteConversationInput{ConversationID: "c-fixture", Status: completedStatus}); err != nil {
		t.Fatal(err)
	}
	danger := true
	request := service.SearchRequest{Query: echoFixture, Fuzzy: fuzzyOff, Provider: providerFixture, Model: modelFixture, Status: completedStatus, Type: commandDocument, Dangerous: &danger, From: created, To: created.Add(time.Second)}
	r, err := s.LexicalSearch(t.Context(), request)
	if err != nil || len(r.Hits) != 1 || r.Hits[0].DocumentID != "command-filter" {
		t.Fatalf("filters: %+v %v", r, err)
	}
	request.ExactCommand = `echo --name a_b%`
	r, err = s.LexicalSearch(t.Context(), request)
	if err != nil || len(r.Hits) != 1 {
		t.Fatalf("exact: %+v %v", r, err)
	}
	request.ExactCommand = ""
	for _, filter := range []struct {
		name   string
		mutate func(*service.SearchRequest)
	}{
		{name: "model", mutate: func(r *service.SearchRequest) { r.Model = "other-model" }},
		{name: "status", mutate: func(r *service.SearchRequest) { r.Status = "started" }},
		{name: "dangerous", mutate: func(r *service.SearchRequest) { value := false; r.Dangerous = &value }},
		{name: "from", mutate: func(r *service.SearchRequest) { r.From = created.Add(time.Second) }},
		{name: "to is exclusive", mutate: func(r *service.SearchRequest) { r.To = created }},
		{name: "exact bytes", mutate: func(r *service.SearchRequest) { r.ExactCommand = `echo --name axbz` }},
	} {
		t.Run(filter.name, func(t *testing.T) {
			filtered := request
			filter.mutate(&filtered)
			result, searchErr := s.LexicalSearch(t.Context(), filtered)
			if searchErr != nil || len(result.Hits) != 0 {
				t.Fatalf("ignored %s filter: %+v %v", filter.name, result, searchErr)
			}
		})
	}
}

func TestLexicalReportsDictionaryCandidateCap(t *testing.T) {
	t.Parallel()
	s := lexicalTestStore(t)
	for i := range 300 {
		if _, err := s.db.ExecContext(t.Context(), `INSERT INTO history_deletions(token,deletion_key) VALUES(?,?)`, fmt.Sprintf("unrelated%d", i), "doker"); err != nil {
			t.Fatal(err)
		}
	}
	r, err := s.LexicalSearch(t.Context(), service.SearchRequest{Query: "doker", Fuzzy: fuzzyAlways})
	if err != nil || !r.Truncated {
		t.Fatalf("dictionary cap missing: %+v %v", r, err)
	}
}

func TestLexicalTypoKeepsOtherTermsMorphology(t *testing.T) {
	t.Parallel()
	s := lexicalTestStore(t)
	lexicalTestMessage(t, s, "fixture", "connected docker")
	r, err := s.LexicalSearch(t.Context(), service.SearchRequest{Query: "connecting dokcer", Fuzzy: fuzzyAlways})
	if err != nil || len(r.Hits) != 1 {
		t.Fatalf("combined morphology: %+v %v", r, err)
	}
}

func TestLexicalCanonicalDocumentID(t *testing.T) {
	t.Parallel()
	s := lexicalTestStore(t)
	const id = "325536da-ad4b-470a-9bc9-83ee54824168"
	lexicalTestMessage(t, s, id, "find files")
	result, err := s.LexicalSearch(t.Context(), service.SearchRequest{Query: filesFixture, Fuzzy: fuzzyOff})
	if err != nil || len(result.Hits) != 1 {
		t.Fatalf("search = %+v, %v", result, err)
	}
	if result.Hits[0].DocumentID != id || result.Hits[0].MessageID != id {
		t.Fatalf("canonical identity lost: %+v", result.Hits[0])
	}
}

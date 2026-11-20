package cli

import (
	"testing"

	"github.com/ra1phdd/zev/internal/core/domain"
	"github.com/ra1phdd/zev/internal/core/locale"
	"github.com/ra1phdd/zev/pkg/tui/list"
)

//nolint:paralleltest // locale.SetLanguage mutates process-global locale state.
func TestAutoLanguageFollowsNaturalLanguageQuery(t *testing.T) {
	restore := locale.SetLanguage(queryUILanguage("auto", false), "посмотреть стату докера одноразово снапшот")
	t.Cleanup(restore)
	if got := locale.Language(); got != "ru" {
		t.Fatalf("language=%q, want ru", got)
	}
}

//nolint:paralleltest // locale.SetLanguage mutates process-global locale state.
func TestAutoLanguageFollowsShortRussianTechnicalQuery(t *testing.T) {
	restore := locale.SetLanguage(queryUILanguage("auto", false), "пинг гугла")
	t.Cleanup(restore)
	if got := locale.Language(); got != "ru" {
		t.Fatalf("language=%q, want ru", got)
	}
}

//nolint:paralleltest // locale.SetLanguage mutates process-global locale state.
func TestRefinementUpdatesInterfaceWithoutChangingMessages(t *testing.T) {
	for _, configured := range []string{"auto", "ru", "en"} {
		t.Run(configured, func(t *testing.T) {
			restore := locale.SetLanguage(configured, "Show files in this directory")
			t.Cleanup(restore)
			p := &queryFlowProvider{}
			s := newTestQuerySession(t, p)
			s.updateLanguage = locale.UpdateRequestLanguage
			for _, tc := range []struct{ text, language string }{
				{"Покажи самые большие файлы в текущей директории", "ru"},
				{"Show the largest files in the current directory", "en"},
			} {
				p.results = append(p.results, queryProviderResult{response: &domain.OptionsResponse{IsValid: true, Commands: []domain.Command{{Command: "ls -la", ShortExplanation: "provider content"}}}})
				s.input = func(string) (string, error) { return tc.text, nil }
				if ready, err := s.refine(t.Context()); err != nil || !ready {
					t.Fatalf("refine: %v, %v", ready, err)
				}
				want := tc.language
				if configured != "auto" {
					want = configured
				}
				if got := locale.Language(); got != want {
					t.Fatalf("language=%s want %s", got, want)
				}
				if last := p.histories[len(p.histories)-1].Messages; last[len(last)-1].Content != tc.text {
					t.Fatal("user text changed")
				}
				s.selectItem = func(options list.SelectOptions) (list.Item, error) {
					if options.Title != locale.Text(locale.KeyQuerySelectCommandRun) {
						t.Fatalf("unexpected title: %s", options.Title)
					}
					return options.Items[0], nil
				}
				if _, err := s.chooseCommand(); err != nil {
					t.Fatal(err)
				}
				if s.response.Commands[0].ShortExplanation != "provider content" {
					t.Fatal("provider text was translated")
				}
			}
		})
	}
}

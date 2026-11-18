package service

import (
	"errors"
	"slices"
	"sync"
	"testing"

	"github.com/ra1phdd/zev/internal/core/domain"
	"github.com/ra1phdd/zev/internal/entities/alias/repository"
)

func TestPatchPreservesUnselectedFieldsAndDetectsStale(t *testing.T) {
	t.Parallel()
	svc := New(repository.New(t.TempDir()))
	a := domain.Alias{Name: "inspect", Template: "echo {{.arg1}}", Description: "old", Arguments: []domain.Argument{{Name: "target", Required: true}}}
	if err := svc.AddAlias(a); err != nil {
		t.Fatal(err)
	}
	const updatedDescription = "updated description"
	description := updatedDescription
	got, err := svc.Patch(a, Patch{Description: &description})
	if err != nil {
		t.Fatal(err)
	}
	if got.Template != a.Template || got.Description != updatedDescription {
		t.Fatal(got)
	}
	_, err = svc.Patch(a, Patch{Description: &description})
	if !errors.Is(err, ErrAliasChanged) {
		t.Fatalf("stale patch %v", err)
	}
	template := "printf {{.arg1}}"
	got, err = svc.Patch(a, Patch{Template: &template})
	if err != nil || got.Template != template || got.Description != updatedDescription || !slices.Equal(got.Arguments, a.Arguments) {
		t.Fatalf("unselected field conflict %#v %v", got, err)
	}
	resolved, err := svc.Resolve(a.Name, []string{"target"})
	if err != nil || resolved != "printf 'target'" {
		t.Fatalf("persisted template = %q, error = %v", resolved, err)
	}
}

func TestPatchDoesNotOverwriteConcurrentTemplate(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	snapshot := domain.Alias{Name: "deploy", Template: "echo original", Description: "preserved"}
	if err := New(repository.New(dir)).AddAlias(snapshot); err != nil {
		t.Fatal(err)
	}
	type result struct {
		template string
		err      error
	}
	results := make(chan result, 2)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for _, template := range []string{"echo first", "echo second"} {
		wg.Go(func() {
			<-start
			_, err := New(repository.New(dir)).Patch(snapshot, Patch{Template: &template})
			results <- result{template: template, err: err}
		})
	}
	close(start)
	wg.Wait()
	close(results)
	var successes, conflicts int
	var winner string
	for result := range results {
		switch {
		case result.err == nil:
			successes++
			winner = result.template
		case errors.Is(result.err, ErrAliasChanged):
			conflicts++
		default:
			t.Errorf("patch error = %v, want success or ErrAliasChanged", result.err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("concurrent patches: successes=%d conflicts=%d, want one each", successes, conflicts)
	}
	svc := New(repository.New(dir))
	resolved, err := svc.Resolve(snapshot.Name, nil)
	if err != nil || resolved != winner {
		t.Fatalf("winning template lost: %q, error = %v, want %q", resolved, err, winner)
	}
	aliases, err := svc.List()
	if err != nil || len(aliases) != 1 || aliases[0].Description != snapshot.Description {
		t.Fatalf("unselected metadata changed: %#v, error = %v", aliases, err)
	}
}

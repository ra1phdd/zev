package main

import (
	"strings"
	"testing"
)

func TestCollectContinuationInputsRequiresNewEvidence(t *testing.T) {
	t.Parallel()
	answers := []string{"", "", ""}
	_, err := collectContinuationInputs(func(string) (string, error) {
		value := answers[0]
		answers = answers[1:]
		return value, nil
	}, nil, true)
	if err == nil || !strings.Contains(err.Error(), "context files") {
		t.Fatalf("empty continuation error = %v", err)
	}
}

func TestCollectContinuationInputsAllowsContextWithoutClarification(t *testing.T) {
	t.Parallel()
	answers := []string{"README.md", "", ""}
	got, err := collectContinuationInputs(func(string) (string, error) {
		value := answers[0]
		answers = answers[1:]
		return value, nil
	}, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.ContextFiles) != 1 || got.ContextFiles[0] != "README.md" {
		t.Fatalf("context files = %+v", got.ContextFiles)
	}
	if got.CommandLog != "" || got.Clarification != "" {
		t.Fatalf("optional inputs = %+v", got)
	}
}

func TestSplitContinuationPaths(t *testing.T) {
	t.Parallel()
	got := splitContinuationPaths("one.md, two.md\nthree.md")
	if strings.Join(got, "|") != "one.md|two.md|three.md" {
		t.Fatalf("paths = %q", got)
	}
}

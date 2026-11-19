package cli

import (
	"context"
	"errors"
	"testing"

	"github.com/ra1phdd/zev/internal/core/config"
)

func TestRunSessionRegistryUsesUniqueTenCharacterIDs(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	if _, err := config.GetZevDir(); err != nil {
		t.Fatal(err)
	}
	conversationID := "a5aeb2b8-42c3-4f14-9a7b-fba914ef32c1"
	commandID := "b6d3fd36-02de-4d63-a057-1845876218e2"
	first, err := registerRunSession(context.Background(), conversationID, commandID)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != runSessionIDLength {
		t.Fatalf("session ID length = %d, want %d", len(first), runSessionIDLength)
	}
	second, err := registerRunSession(context.Background(), conversationID, commandID)
	if err != nil {
		t.Fatal(err)
	}
	if second != first {
		t.Fatalf("repeated registration = %q, want stable %q", second, first)
	}
	other, err := registerRunSession(context.Background(), conversationID, "c7e4ef47-13ef-4e74-b168-2956987329f3")
	if err != nil {
		t.Fatal(err)
	}
	if other == first {
		t.Fatal("different references received the same session ID")
	}
	gotConversation, gotCommand, err := resolveRunSession(context.Background(), first)
	if err != nil || gotConversation != conversationID || gotCommand != commandID {
		t.Fatalf("resolved session = %q, %q, %v", gotConversation, gotCommand, err)
	}
	if _, _, err := resolveRunSession(context.Background(), "123456789"); err == nil {
		t.Fatal("short session ID was accepted")
	}
	if _, _, err := resolveRunSession(context.Background(), "zzzzzzzzzz"); err == nil {
		t.Fatal("invalid session alphabet was accepted")
	}
}

func TestRunSessionRegistryRejectsMissingSession(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	_, _, err := resolveRunSession(context.Background(), "1234567890")
	if !errors.Is(err, errRunSessionNotFound) {
		t.Fatalf("missing session error = %v", err)
	}
}

package service

import (
	"errors"
	"testing"
)

func TestSearchRejectsWhitespaceExactCommand(t *testing.T) {
	t.Parallel()
	for _, exact := range []string{" ", "\t\n", "\u00a0"} {
		request := SearchRequest{ExactCommand: exact}
		if err := validateSearch(&request); !errors.Is(err, ErrInvalid) {
			t.Errorf("whitespace exact command accepted: %q error=%v", exact, err)
		}
	}
	request := SearchRequest{ExactCommand: "  echo fixture  "}
	if err := validateSearch(&request); err != nil {
		t.Fatal(err)
	}
	if request.ExactCommand != "  echo fixture  " {
		t.Fatal("valid exact command was normalized")
	}
}

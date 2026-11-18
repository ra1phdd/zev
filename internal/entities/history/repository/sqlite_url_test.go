package repository

import (
	"net/url"
	"testing"
)

func TestHistorySQLiteURLSupportsWindowsDrivePaths(t *testing.T) {
	t.Parallel()

	query := url.Values{}
	query.Set("mode", "rw")
	got := historySQLiteURL("C:/Users/ra1ph/.zev/history.db", query)
	want := "file:///C:/Users/ra1ph/.zev/history.db?mode=rw"
	if got != want {
		t.Fatalf("historySQLiteURL() = %q, want %q", got, want)
	}
}

func TestHistorySQLiteURLPreservesUNCPaths(t *testing.T) {
	t.Parallel()

	got := historySQLiteURL("//server/share/history.db", nil)
	want := "file://server/share/history.db"
	if got != want {
		t.Fatalf("historySQLiteURL() = %q, want %q", got, want)
	}
}

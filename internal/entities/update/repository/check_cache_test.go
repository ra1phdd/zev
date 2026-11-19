package repository

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	updateservice "github.com/ra1phdd/zev/internal/entities/update/service"
)

func TestCheckCacheRefreshesOncePerDay(t *testing.T) {
	t.Parallel()
	cache := NewCheckCache(filepath.Join(t.TempDir(), "update-check.json"))
	now := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	calls := 0
	check := func(context.Context) (updateservice.CheckResult, error) {
		calls++
		return updateservice.CheckResult{Branch: updateservice.Stable, Current: "1.0.0", Version: "1.1.0", Update: true}, nil
	}
	entry, checked, err := cache.Refresh(t.Context(), now, updateservice.Stable, "1.0.0", check)
	if err != nil || !checked || !entry.Available || entry.Latest != "1.1.0" {
		t.Fatalf("first refresh = %+v, checked=%t, err=%v", entry, checked, err)
	}
	entry, checked, err = cache.Refresh(t.Context(), now.Add(23*time.Hour), updateservice.Stable, "1.0.0", check)
	if err != nil || checked || !entry.Available || calls != 1 {
		t.Fatalf("fresh cache = %+v, checked=%t, calls=%d, err=%v", entry, checked, calls, err)
	}
	entry, checked, err = cache.Refresh(t.Context(), now.Add(24*time.Hour), updateservice.Stable, "1.0.0", check)
	if err != nil || !checked || calls != 2 || !entry.Available {
		t.Fatalf("expired cache = %+v, checked=%t, calls=%d, err=%v", entry, checked, calls, err)
	}
}

func TestCheckCacheStoresFailedAttemptForOneHour(t *testing.T) {
	t.Parallel()
	cache := NewCheckCache(filepath.Join(t.TempDir(), "update-check.json"))
	now := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	calls := 0
	check := func(context.Context) (updateservice.CheckResult, error) {
		calls++
		if calls == 1 {
			return updateservice.CheckResult{}, context.DeadlineExceeded
		}
		return updateservice.CheckResult{Branch: updateservice.Main, Current: "main-abc", Version: "main-def"}, nil
	}
	entry, checked, err := cache.Refresh(t.Context(), now, updateservice.Main, "main-abc", check)
	if err != nil || !checked || !entry.Failed || entry.Available {
		t.Fatalf("failed refresh = %+v, checked=%t, err=%v", entry, checked, err)
	}
	cache = NewCheckCache(cache.path)
	entry, checked, err = cache.Refresh(t.Context(), now.Add(59*time.Minute), updateservice.Main, "main-abc", check)
	if err != nil || checked || !entry.Failed || calls != 1 {
		t.Fatalf("cached failure at 59m = %+v, checked=%t, calls=%d, err=%v", entry, checked, calls, err)
	}
	entry, checked, err = cache.Refresh(t.Context(), now.Add(time.Hour), updateservice.Main, "main-abc", check)
	if err != nil || !checked || entry.Failed || calls != 2 || entry.Latest != "main-def" {
		t.Fatalf("retry at 1h = %+v, checked=%t, calls=%d, err=%v", entry, checked, calls, err)
	}
}

func TestCheckCacheRejectsInvalidRequests(t *testing.T) {
	t.Parallel()
	cache := NewCheckCache(filepath.Join(t.TempDir(), "update-check.json"))
	if _, _, err := cache.Refresh(t.Context(), time.Now(), updateservice.Stable, "1.0.0", nil); err == nil {
		t.Fatal("nil checker accepted")
	}
}

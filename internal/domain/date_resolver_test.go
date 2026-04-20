package domain

import (
	"testing"
	"time"
)

func TestResolveRelativeDateRussianWeekday(t *testing.T) {
	now := time.Date(2026, time.April, 13, 10, 0, 0, 0, time.UTC) // Monday
	resolved := ResolveRelativeDate("до пятницы", now)
	if resolved.Value == nil {
		t.Fatalf("expected resolved date")
	}
	if got := resolved.Value.Format("2006-01-02"); got != "2026-04-17" {
		t.Fatalf("expected 2026-04-17, got %s", got)
	}
}

func TestResolveRelativeDateAbsoluteWithoutYear(t *testing.T) {
	now := time.Date(2026, time.April, 13, 10, 0, 0, 0, time.UTC)
	resolved := ResolveRelativeDate("14.04", now)
	if resolved.Value == nil {
		t.Fatalf("expected resolved date")
	}
	if got := resolved.Value.Format("2006-01-02"); got != "2026-04-14" {
		t.Fatalf("expected 2026-04-14, got %s", got)
	}
}

func TestResolveRelativeDateRussianShortWeekday(t *testing.T) {
	now := time.Date(2026, time.April, 20, 10, 0, 0, 0, time.UTC) // Monday
	resolved := ResolveRelativeDate("сб", now)
	if resolved.Value == nil {
		t.Fatalf("expected resolved date")
	}
	if got := resolved.Value.Format("2006-01-02"); got != "2026-04-25" {
		t.Fatalf("expected 2026-04-25, got %s", got)
	}
}

func TestResolveRelativeDateRussianNamedMonth(t *testing.T) {
	now := time.Date(2026, time.April, 20, 10, 0, 0, 0, time.UTC)
	resolved := ResolveRelativeDate("1 мая", now)
	if resolved.Value == nil {
		t.Fatalf("expected resolved date")
	}
	if got := resolved.Value.Format("2006-01-02"); got != "2026-05-01" {
		t.Fatalf("expected 2026-05-01, got %s", got)
	}
}

func TestResolveRelativeDateDayOnly(t *testing.T) {
	now := time.Date(2026, time.April, 20, 10, 0, 0, 0, time.UTC)
	resolved := ResolveRelativeDate("26", now)
	if resolved.Value == nil {
		t.Fatalf("expected resolved date")
	}
	if got := resolved.Value.Format("2006-01-02"); got != "2026-04-26" {
		t.Fatalf("expected 2026-04-26, got %s", got)
	}
}

func TestResolveRelativeDateTypoWeekday(t *testing.T) {
	now := time.Date(2026, time.April, 20, 10, 0, 0, 0, time.UTC)
	resolved := ResolveRelativeDate("до пятницаы", now)
	if resolved.Value == nil {
		t.Fatalf("expected resolved date")
	}
	if got := resolved.Value.Format("2006-01-02"); got != "2026-04-24" {
		t.Fatalf("expected 2026-04-24, got %s", got)
	}
}

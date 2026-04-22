package domain

import (
	"testing"
	"time"
)

func TestLocationForTimezoneFallsBackToUTC(t *testing.T) {
	location := LocationForTimezone("Definitely/NotAZone")
	if location != time.UTC {
		t.Fatalf("location = %v, want UTC", location)
	}
}

func TestLocalizeTimeUsesTimezoneWhenValid(t *testing.T) {
	now := time.Date(2026, time.April, 22, 9, 0, 0, 0, time.UTC)
	local := LocalizeTime(now, "Europe/Moscow")
	if got := local.Format(time.RFC3339); got != "2026-04-22T12:00:00+03:00" {
		t.Fatalf("localized time = %s", got)
	}
}

package domain

import "time"

func LocationForTimezone(timezone string) *time.Location {
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return time.UTC
	}
	return location
}

func LocalizeTime(now time.Time, timezone string) time.Time {
	return now.In(LocationForTimezone(timezone))
}

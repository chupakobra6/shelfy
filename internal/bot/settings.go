package bot

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

var allowedTimezones = map[string]struct{}{
	"Europe/Moscow":    {},
	"UTC":              {},
	"Europe/Berlin":    {},
	"America/New_York": {},
}

func isAllowedTimezone(value string) bool {
	_, ok := allowedTimezones[value]
	return ok
}

func adjustDigestTime(current string, deltaToken string) (string, error) {
	base, err := time.Parse("15:04", current)
	if err != nil {
		return "", err
	}
	deltaMinutes, err := strconv.Atoi(strings.TrimSpace(deltaToken))
	if err != nil {
		return "", err
	}
	updated := base.Add(time.Duration(deltaMinutes) * time.Minute)
	return fmt.Sprintf("%02d:%02d", updated.Hour(), updated.Minute()), nil
}

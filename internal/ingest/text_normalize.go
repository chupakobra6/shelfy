package ingest

import (
	"strings"
	"unicode/utf8"
)

func normalizeFreeText(input string) string {
	value := strings.TrimSpace(input)
	if value == "" {
		return ""
	}
	value = strings.NewReplacer("\r", " ", "\n", " ", "\t", " ", "\u00a0", " ").Replace(value)
	return strings.Join(strings.Fields(value), " ")
}

func excerptForLog(value string, limit int) string {
	value = strings.TrimSpace(value)
	if limit <= 0 || value == "" {
		return value
	}
	if utf8.RuneCountInString(value) <= limit {
		return value
	}
	runes := []rune(value)
	return strings.TrimSpace(string(runes[:limit])) + "…"
}

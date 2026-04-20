package ingest

import "strings"

func normalizeFreeText(input string) string {
	fields := strings.Fields(strings.TrimSpace(input))
	return strings.Join(fields, " ")
}

func excerptForLog(input string, limit int) string {
	value := normalizeFreeText(input)
	if limit <= 0 || len(value) <= limit {
		return value
	}
	return value[:limit]
}

func int64Ptr(v int64) *int64 {
	if v == 0 {
		return nil
	}
	return &v
}

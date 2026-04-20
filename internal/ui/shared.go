package ui

import "strings"

func escapeHTML(v string) string {
	replacer := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	return replacer.Replace(v)
}

func short(v string, limit int) string {
	runes := []rune(v)
	if len(runes) <= limit {
		return v
	}
	return string(runes[:limit-1]) + "…"
}

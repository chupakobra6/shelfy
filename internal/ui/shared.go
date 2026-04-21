package ui

import (
	"fmt"
	"strings"
)

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

func dashboardPageCallback(mode string, page int) string {
	switch strings.TrimSpace(mode) {
	case "soon":
		if page <= 0 {
			return "dashboard:soon"
		}
		return fmt.Sprintf("dashboard:soon:page:%d", page)
	default:
		if page <= 0 {
			return "dashboard:list"
		}
		return fmt.Sprintf("dashboard:list:page:%d", page)
	}
}

func productOpenCallback(productID int64, originMode string, originPage int) string {
	return fmt.Sprintf("product:open:%d:%s:%d", productID, normalizeOriginMode(originMode), maxPage(originPage))
}

func productSetCallback(productID int64, status, originMode string, originPage int) string {
	return fmt.Sprintf("product:set:%d:%s:%s:%d", productID, status, normalizeOriginMode(originMode), maxPage(originPage))
}

func normalizeOriginMode(mode string) string {
	switch strings.TrimSpace(mode) {
	case "soon":
		return "soon"
	default:
		return "list"
	}
}

func maxPage(page int) int {
	if page < 0 {
		return 0
	}
	return page
}

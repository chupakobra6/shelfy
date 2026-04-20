package ui

import (
	"strings"
	"time"
)

func (r *Renderer) DigestMessage(lines []string) (string, error) {
	return r.copy.Render("digest.morning", map[string]any{
		"digest_lines": strings.Join(lines, "\n"),
	})
}

func (r *Renderer) DigestLine(kind, name string, expiresOn time.Time) (string, error) {
	messageID := "digest.line.soon"
	if kind == "expired" {
		messageID = "digest.line.expired"
	}
	return r.copy.Render(messageID, map[string]any{
		"name":       escapeHTML(name),
		"expires_on": expiresOn.Format("2006-01-02"),
	})
}

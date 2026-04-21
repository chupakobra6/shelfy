package ingest

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/igor/shelfy/internal/domain"
	"github.com/igor/shelfy/internal/observability"
)

type parsedDraft struct {
	Name              string
	ExpiresOn         *time.Time
	RawDeadlinePhrase string
	Confidence        string
	Source            string
}

func (s *Service) parseTextDraft(ctx context.Context, text string, now time.Time) (parsedDraft, error) {
	cleaned := normalizeFreeText(text)
	s.logger.InfoContext(ctx, "draft_text_input_prepared", observability.LogAttrs(ctx,
		"text_len", len(cleaned),
		"text_excerpt", excerptForLog(cleaned, 280),
	)...)
	result := heuristicParse(cleaned, now)
	if result.Name != "" && result.ExpiresOn != nil {
		s.logger.InfoContext(ctx, "draft_parse_completed", observability.LogAttrs(ctx, "source", result.Source, "confidence", result.Confidence, "has_name", true, "has_expiry", true)...)
		return result, nil
	}
	if !shouldTryTextModel(cleaned, result) {
		if result.Name == "" && result.ExpiresOn == nil {
			return parsedDraft{}, fmt.Errorf("unable to extract any draft fields")
		}
		s.logger.InfoContext(ctx, "draft_parse_completed", observability.LogAttrs(ctx,
			"source", result.Source,
			"confidence", result.Confidence,
			"has_name", result.Name != "",
			"has_expiry", result.ExpiresOn != nil,
			"fast_path", true,
		)...)
		return result, nil
	}
	if structured, err := s.callOllamaText(ctx, cleaned); err == nil {
		if structured.Name != "" && strings.TrimSpace(result.Name) == "" {
			result.Name = structured.Name
		}
		if structured.RawDeadlinePhrase != "" && strings.TrimSpace(result.RawDeadlinePhrase) == "" {
			result.RawDeadlinePhrase = structured.RawDeadlinePhrase
		}
		if result.ExpiresOn == nil && structured.RawDeadlinePhrase != "" {
			resolved := domain.ResolveRelativeDate(structured.RawDeadlinePhrase, now)
			result.ExpiresOn = resolved.Value
		}
		if result.Name != "" || result.ExpiresOn != nil {
			result.Confidence = "model"
			result.Source = "ollama-text"
		}
	} else {
		s.logger.WarnContext(ctx, "ollama_text_failed", observability.LogAttrs(ctx, "error", err)...)
	}
	if result.Name == "" && result.ExpiresOn == nil {
		return parsedDraft{}, fmt.Errorf("unable to extract any draft fields")
	}
	s.logger.InfoContext(ctx, "draft_parse_completed", observability.LogAttrs(ctx,
		"source", result.Source,
		"confidence", result.Confidence,
		"has_name", result.Name != "",
		"has_expiry", result.ExpiresOn != nil,
	)...)
	return result, nil
}

func heuristicParse(text string, now time.Time) parsedDraft {
	cleaned := strings.TrimSpace(text)
	if cleaned == "" {
		return parsedDraft{Confidence: "missing", Source: "empty"}
	}
	lower := strings.ToLower(cleaned)
	candidates := []string{" до ", " by ", " expires ", " exp "}
	for _, marker := range candidates {
		if idx := strings.Index(lower, marker); idx > 0 {
			name := strings.TrimSpace(cleaned[:idx])
			phrase := strings.TrimSpace(cleaned[idx+len(marker):])
			resolved := domain.ResolveRelativeDate(phrase, now)
			return parsedDraft{
				Name:              name,
				ExpiresOn:         resolved.Value,
				RawDeadlinePhrase: phrase,
				Confidence:        resolved.Confidence,
				Source:            "heuristic_marker",
			}
		}
	}
	if name, phrase, resolved, ok := splitTrailingDatePhrase(cleaned, now); ok {
		return parsedDraft{
			Name:              name,
			ExpiresOn:         resolved.Value,
			RawDeadlinePhrase: phrase,
			Confidence:        resolved.Confidence,
			Source:            "heuristic_suffix",
		}
	}
	if name, phrase, resolved, ok := extractNaturalDateFromText(cleaned, now); ok {
		return parsedDraft{
			Name:              name,
			ExpiresOn:         resolved.Value,
			RawDeadlinePhrase: phrase,
			Confidence:        resolved.Confidence,
			Source:            "heuristic_natural",
		}
	}
	resolved := domain.ResolveRelativeDate(cleaned, now)
	if resolved.Value != nil {
		return parsedDraft{
			ExpiresOn:         resolved.Value,
			RawDeadlinePhrase: cleaned,
			Confidence:        resolved.Confidence,
			Source:            "heuristic_date_only",
		}
	}
	return parsedDraft{
		Name:       cleaned,
		Confidence: "low",
		Source:     "heuristic_name_only",
	}
}

func shouldTryTextModel(cleaned string, current parsedDraft) bool {
	if strings.TrimSpace(cleaned) == "" {
		return false
	}
	if current.Name != "" && current.ExpiresOn != nil {
		return false
	}
	if current.Name != "" && current.ExpiresOn == nil && looksLikePlainProductName(cleaned) {
		return false
	}
	if current.Name == "" && current.ExpiresOn != nil {
		return false
	}
	return true
}

func looksLikePlainProductName(cleaned string) bool {
	lower := strings.ToLower(strings.TrimSpace(cleaned))
	if lower == "" {
		return false
	}
	for _, marker := range []string{
		"до ", " by ", "expires", "exp ", "сегодня", "завтра",
		"янв", "фев", "мар", "апр", "мая", "июн", "июл", "авг", "сен", "окт", "ноя", "дек",
		"пн", "вт", "ср", "чт", "пт", "сб", "вс",
	} {
		if strings.Contains(lower, marker) {
			return false
		}
	}
	if strings.ContainsAny(lower, "0123456789./-") {
		return false
	}
	return true
}

func splitTrailingDatePhrase(cleaned string, now time.Time) (name string, phrase string, resolved domain.ResolvedDate, ok bool) {
	parts := strings.Fields(cleaned)
	if len(parts) < 2 {
		return "", "", domain.ResolvedDate{}, false
	}
	maxTail := 3
	if len(parts)-1 < maxTail {
		maxTail = len(parts) - 1
	}
	for tailSize := maxTail; tailSize >= 1; tailSize-- {
		phrase = strings.Join(parts[len(parts)-tailSize:], " ")
		resolved = domain.ResolveRelativeDate(phrase, now)
		if resolved.Value == nil {
			continue
		}
		name = strings.TrimSpace(strings.Join(parts[:len(parts)-tailSize], " "))
		if name == "" || isDatePrefixOnly(name) {
			continue
		}
		return name, phrase, resolved, true
	}
	return "", "", domain.ResolvedDate{}, false
}

func extractNaturalDateFromText(cleaned string, now time.Time) (name string, phrase string, resolved domain.ResolvedDate, ok bool) {
	extracted, ok := domain.ExtractDateFromText(cleaned, now)
	if !ok || extracted.Value == nil {
		return "", "", domain.ResolvedDate{}, false
	}
	name = removeDatePhrase(cleaned, extracted.Phrase)
	if name == "" || name == cleaned || isDatePrefixOnly(name) {
		return "", "", domain.ResolvedDate{}, false
	}
	return name, extracted.Phrase, domain.ResolvedDate{
		Value:      extracted.Value,
		Confidence: extracted.Confidence,
	}, true
}

func removeDatePhrase(cleaned, phrase string) string {
	idx := strings.Index(cleaned, phrase)
	if idx < 0 {
		return cleaned
	}
	value := strings.TrimSpace(cleaned[:idx] + " " + cleaned[idx+len(phrase):])
	value = strings.TrimSpace(strings.Trim(value, "-—,:;"))
	for _, suffix := range []string{" до", " к", " на", " в", " во", " by"} {
		value = strings.TrimSuffix(value, suffix)
	}
	return strings.TrimSpace(value)
}

func isDatePrefixOnly(value string) bool {
	switch normalizeFreeText(strings.ToLower(value)) {
	case "до", "к", "на", "в", "во", "by":
		return true
	default:
		return false
	}
}

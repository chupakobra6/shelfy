package ingest

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/igor/shelfy/internal/domain"
	"github.com/igor/shelfy/internal/observability"
)

func (s *Service) parsePhotoDraft(ctx context.Context, ocrText string, now time.Time, imagePath string) (parsedDraft, error) {
	cleaned := cleanOCRText(ocrText)
	s.logger.InfoContext(ctx, "photo_ocr_text_prepared", observability.LogAttrs(ctx,
		"raw_text_len", len(strings.TrimSpace(ocrText)),
		"clean_text_len", len(cleaned),
		"clean_text_excerpt", excerptForLog(cleaned, 320),
	)...)
	result := parsedDraft{}
	if cleaned != "" {
		if structured, err := s.callOllamaText(ctx, cleaned); err == nil {
			result = mergeStructuredDraft(result, structured, now)
			if result.Name != "" || result.ExpiresOn != nil {
				result.Confidence = "model"
				result.Source = "ollama-text"
			}
		} else {
			s.logger.WarnContext(ctx, "ollama_text_failed", observability.LogAttrs(ctx, "error", err)...)
		}
	}
	if imagePath != "" && (result.Name == "" || result.ExpiresOn == nil) {
		if vision, err := s.callOllamaVision(ctx, imagePath, cleaned); err == nil {
			result = mergeStructuredDraft(result, vision, now)
			if result.Name != "" || result.ExpiresOn != nil {
				result.Confidence = "model_vision"
				result.Source = "ollama-vision"
			}
		} else {
			s.logger.WarnContext(ctx, "ollama_vision_failed", observability.LogAttrs(ctx, "error", err)...)
		}
	}
	if shouldUseOCRHeuristicFallback(cleaned, result) {
		heuristic := heuristicParse(cleaned, now)
		if result.Name == "" && heuristic.Name != "" {
			result.Name = heuristic.Name
		}
		if result.RawDeadlinePhrase == "" && heuristic.RawDeadlinePhrase != "" {
			result.RawDeadlinePhrase = heuristic.RawDeadlinePhrase
		}
		if result.ExpiresOn == nil && heuristic.ExpiresOn != nil {
			result.ExpiresOn = heuristic.ExpiresOn
		}
		if result.Source == "" && (heuristic.Name != "" || heuristic.ExpiresOn != nil) {
			result.Source = "heuristic_ocr_fallback"
			result.Confidence = heuristic.Confidence
		}
	}
	if result.Name == "" && result.ExpiresOn == nil {
		return parsedDraft{}, fmt.Errorf("unable to extract any draft fields from OCR/image")
	}
	s.logger.InfoContext(ctx, "draft_parse_completed", observability.LogAttrs(ctx,
		"source", result.Source,
		"confidence", result.Confidence,
		"has_name", result.Name != "",
		"has_expiry", result.ExpiresOn != nil,
		"name_excerpt", excerptForLog(result.Name, 120),
		"deadline_excerpt", excerptForLog(result.RawDeadlinePhrase, 120),
	)...)
	return result, nil
}

func mergeStructuredDraft(base parsedDraft, structured ollamaDraft, now time.Time) parsedDraft {
	if base.Name == "" && strings.TrimSpace(structured.Name) != "" {
		base.Name = normalizeFreeText(structured.Name)
	}
	if base.RawDeadlinePhrase == "" && strings.TrimSpace(structured.RawDeadlinePhrase) != "" {
		base.RawDeadlinePhrase = normalizeFreeText(structured.RawDeadlinePhrase)
	}
	if base.ExpiresOn == nil && base.RawDeadlinePhrase != "" {
		resolved := domain.ResolveRelativeDate(base.RawDeadlinePhrase, now)
		base.ExpiresOn = resolved.Value
	}
	return base
}

func shouldUseOCRHeuristicFallback(cleaned string, current parsedDraft) bool {
	if strings.TrimSpace(cleaned) == "" {
		return false
	}
	if current.Name != "" && current.ExpiresOn != nil {
		return false
	}
	if looksLikeDiagnosticText(cleaned) {
		return false
	}
	if len(cleaned) > 240 {
		return false
	}
	return true
}

func cleanOCRText(input string) string {
	lines := strings.Split(input, "\n")
	filtered := make([]string, 0, len(lines))
	for _, line := range lines {
		line = normalizeFreeText(line)
		if line == "" {
			continue
		}
		if looksLikeDiagnosticText(line) {
			continue
		}
		filtered = append(filtered, line)
	}
	return strings.Join(filtered, "\n")
}

func looksLikeDiagnosticText(input string) bool {
	lower := strings.ToLower(strings.TrimSpace(input))
	if lower == "" {
		return false
	}
	diagnostics := []string{
		"error opening data file",
		"please make sure the tessdata_prefix environment variable is set",
		"failed loading language",
		"tesseract couldn't load any languages",
		"could not initialize tesseract",
		"estimating resolution as",
		"read_params_file",
		"tesseract open source ocr engine",
	}
	for _, marker := range diagnostics {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

package ingest

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/igor/shelfy/internal/domain"
	"github.com/igor/shelfy/internal/observability"
)

var ocrSignalTokenPattern = regexp.MustCompile(`[\p{L}\p{N}]+`)
var receiptPriceLikePattern = regexp.MustCompile(`\d+[.,]\d{2}`)

var ocrDateSignalTokens = map[string]struct{}{
	"today":       {},
	"tomorrow":    {},
	"сегодня":     {},
	"завтра":      {},
	"послезавтра": {},
	"mon":         {},
	"tue":         {},
	"wed":         {},
	"thu":         {},
	"fri":         {},
	"sat":         {},
	"sun":         {},
	"пн":          {},
	"вт":          {},
	"ср":          {},
	"чт":          {},
	"пт":          {},
	"сб":          {},
	"вс":          {},
	"янв":         {},
	"фев":         {},
	"мар":         {},
	"апр":         {},
	"май":         {},
	"мая":         {},
	"июн":         {},
	"июл":         {},
	"авг":         {},
	"сен":         {},
	"сент":        {},
	"окт":         {},
	"ноя":         {},
	"дек":         {},
	"jan":         {},
	"feb":         {},
	"mar":         {},
	"apr":         {},
	"may":         {},
	"jun":         {},
	"jul":         {},
	"aug":         {},
	"sep":         {},
	"oct":         {},
	"nov":         {},
	"dec":         {},
}

type photoVisionContribution struct {
	addedName              bool
	addedRawDeadlinePhrase bool
	addedExpiresOn         bool
}

func (c photoVisionContribution) hasAny() bool {
	return c.addedName || c.addedRawDeadlinePhrase || c.addedExpiresOn
}

func (s *Service) parsePhotoDraft(ctx context.Context, caption string, ocrText string, now time.Time, imagePath string) (parsedDraft, error) {
	caption = normalizeFreeText(caption)
	cleaned := cleanOCRText(ocrText)
	hasSignal := hasMeaningfulOCRSignal(cleaned)
	receiptLike := looksLikeReceiptOCR(cleaned)
	s.logger.InfoContext(ctx, "photo_ocr_text_prepared", observability.LogAttrs(ctx,
		"caption_len", len(caption),
		"raw_text_len", len(strings.TrimSpace(ocrText)),
		"clean_text_len", len(cleaned),
		"has_signal", hasSignal,
		"receipt_like", receiptLike,
		"caption_excerpt", excerptForLog(caption, 160),
		"clean_text_excerpt", excerptForLog(cleaned, 320),
	)...)

	captionDraft := s.bestEffortCaptionDraft(ctx, caption, now)
	result := mergeParsedDraft(parsedDraft{}, captionDraft)
	captionHasName := captionDraft.Name != ""
	if draftIsComplete(result) {
		s.logger.InfoContext(ctx, "photo_caption_satisfied_draft", observability.LogAttrs(ctx,
			"source", result.Source,
			"confidence", result.Confidence,
			"has_name", result.Name != "",
			"has_expiry", result.ExpiresOn != nil,
		)...)
		return result, nil
	}

	ocrDraft := parsedDraft{}
	switch {
	case receiptLike && result.Name != "":
		ocrDraft = parseReceiptOCRDraft(cleaned, now)
	case receiptLike && cleaned != "":
		// Receipt-like OCR without a caption-provided product anchor must fail closed.
	case hasSignal && cleaned != "":
		ocrDraft = s.parseOCRAssistDraft(ctx, cleaned, now)
	case cleaned != "":
		s.logger.InfoContext(ctx, "photo_ocr_signal_rejected", observability.LogAttrs(ctx,
			"clean_text_excerpt", excerptForLog(cleaned, 160),
		)...)
	}
	if receiptLike && result.Name == "" && cleaned != "" {
		s.logger.InfoContext(ctx, "photo_receipt_requires_caption_anchor", observability.LogAttrs(ctx,
			"clean_text_excerpt", excerptForLog(cleaned, 160),
		)...)
	}
	result = mergeParsedDraft(result, ocrDraft)

	if imagePath != "" && result.Name == "" && result.ExpiresOn != nil {
		if vision, err := s.callOllamaVision(ctx, imagePath, cleaned, photoVisionModeAnchoredNameAssist, visionAnchorHint(result)); err == nil {
			before := result
			result = mergeStructuredDraft(result, vision, now)
			result = applyPhotoVisionGuardrails(cleaned, result, diffPhotoVisionContribution(before, result), photoVisionModeAnchoredNameAssist)
			if effective := diffPhotoVisionContribution(before, result); effective.hasAny() && (result.Name != "" || result.ExpiresOn != nil) {
				result.Source = "multimodal"
				if result.Confidence == "" {
					result.Confidence = "model_vision"
				}
			}
		} else {
			s.logger.WarnContext(ctx, "ollama_vision_failed", observability.LogAttrs(ctx, "error", err)...)
		}
	} else if hasSignal && imagePath != "" && (result.Name == "" || result.ExpiresOn == nil) {
		if vision, err := s.callOllamaVision(ctx, imagePath, cleaned, photoVisionModeStrictExtract, ""); err == nil {
			before := result
			result = mergeStructuredDraft(result, vision, now)
			result = applyPhotoVisionGuardrails(cleaned, result, diffPhotoVisionContribution(before, result), photoVisionModeStrictExtract)
			if effective := diffPhotoVisionContribution(before, result); effective.hasAny() && (result.Name != "" || result.ExpiresOn != nil) {
				result.Source = "multimodal"
				if result.Confidence == "" {
					result.Confidence = "model_vision"
				}
			}
		} else {
			s.logger.WarnContext(ctx, "ollama_vision_failed", observability.LogAttrs(ctx, "error", err)...)
		}
	} else if imagePath != "" && !hasSignal && (result.Name == "" || result.ExpiresOn == nil) {
		s.logger.InfoContext(ctx, "photo_vision_skipped_low_signal", observability.LogAttrs(ctx,
			"has_caption_anchor", result.Name != "" || result.ExpiresOn != nil,
			"clean_text_excerpt", excerptForLog(cleaned, 160),
		)...)
	}

	if result.ExpiresOn == nil && !captionHasName && looksLikePhotoBoilerplateName(result.Name) {
		result.Name = ""
	}

	if result.Name == "" && result.ExpiresOn == nil {
		return parsedDraft{}, fmt.Errorf("unable to extract any draft fields from caption/OCR/image")
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

func diffPhotoVisionContribution(before, after parsedDraft) photoVisionContribution {
	return photoVisionContribution{
		addedName:              before.Name == "" && after.Name != "",
		addedRawDeadlinePhrase: before.RawDeadlinePhrase == "" && after.RawDeadlinePhrase != "",
		addedExpiresOn:         before.ExpiresOn == nil && after.ExpiresOn != nil,
	}
}

func applyPhotoVisionGuardrails(cleaned string, result parsedDraft, vision photoVisionContribution, mode photoVisionMode) parsedDraft {
	if !vision.hasAny() {
		return result
	}
	switch mode {
	case photoVisionModeAnchoredNameAssist:
		if vision.addedRawDeadlinePhrase {
			result.RawDeadlinePhrase = ""
		}
		if vision.addedExpiresOn {
			result.ExpiresOn = nil
		}
		if vision.addedName && (result.ExpiresOn == nil || looksLikePhotoBoilerplateName(result.Name)) {
			result.Name = ""
		}
	default:
		if vision.addedRawDeadlinePhrase && result.ExpiresOn == nil {
			result.RawDeadlinePhrase = ""
		}
		if vision.addedName && isLatinOnlyName(result.Name) && !hasLatinTokenOverlap(cleaned, result.Name) {
			result.Name = ""
		}
		if vision.addedName && result.ExpiresOn == nil {
			result.Name = ""
		}
	}
	return result
}

func (s *Service) bestEffortCaptionDraft(ctx context.Context, caption string, now time.Time) parsedDraft {
	if strings.TrimSpace(caption) == "" {
		return parsedDraft{}
	}
	draft, err := s.parseTextDraft(ctx, caption, now)
	if err != nil {
		s.logger.InfoContext(ctx, "photo_caption_parse_skipped", observability.LogAttrs(ctx,
			"caption_excerpt", excerptForLog(caption, 160),
			"error", err,
		)...)
		return parsedDraft{}
	}
	return tagParsedDraftSource(draft, "caption")
}

func (s *Service) parseOCRAssistDraft(ctx context.Context, cleaned string, now time.Time) parsedDraft {
	result := parsedDraft{}
	if strings.TrimSpace(cleaned) == "" {
		return result
	}
	if structured, err := s.callOllamaText(ctx, cleaned); err == nil {
		result = mergeStructuredDraft(result, structured, now)
		if result.Name != "" || result.ExpiresOn != nil {
			result.Confidence = "model"
			result.Source = "ollama-text"
		}
	} else {
		s.logger.WarnContext(ctx, "ollama_text_failed", observability.LogAttrs(ctx, "error", err)...)
	}
	if shouldUseOCRHeuristicFallback(cleaned, result) {
		heuristic := heuristicParse(cleaned, now)
		if result.Name == "" && heuristic.Name != "" && !looksLikePhotoBoilerplateName(heuristic.Name) {
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
	if result.ExpiresOn == nil && looksLikePhotoBoilerplateName(result.Name) {
		result.Name = ""
	}
	return tagParsedDraftSource(result, "ocr")
}

func parseReceiptOCRDraft(cleaned string, now time.Time) parsedDraft {
	for _, line := range strings.Split(cleaned, "\n") {
		line = normalizeFreeText(line)
		if line == "" {
			continue
		}
		if extracted, ok := domain.ExtractDateFromText(line, now); ok && extracted.Value != nil {
			return parsedDraft{
				ExpiresOn:         extracted.Value,
				RawDeadlinePhrase: extracted.Phrase,
				Confidence:        extracted.Confidence,
				Source:            "receipt_ocr_date",
			}
		}
	}
	normalized := normalizeFreeText(cleaned)
	if extracted, ok := domain.ExtractDateFromText(normalized, now); ok && extracted.Value != nil {
		return parsedDraft{
			ExpiresOn:         extracted.Value,
			RawDeadlinePhrase: extracted.Phrase,
			Confidence:        extracted.Confidence,
			Source:            "receipt_ocr_date",
		}
	}
	return parsedDraft{}
}

func mergeParsedDraft(base, extra parsedDraft) parsedDraft {
	contributed := false
	if base.Name == "" && extra.Name != "" {
		base.Name = extra.Name
		contributed = true
	}
	if base.RawDeadlinePhrase == "" && extra.RawDeadlinePhrase != "" {
		base.RawDeadlinePhrase = extra.RawDeadlinePhrase
		contributed = true
	}
	if base.ExpiresOn == nil && extra.ExpiresOn != nil {
		base.ExpiresOn = extra.ExpiresOn
		contributed = true
	}
	if !contributed {
		return base
	}
	if base.Source == "" {
		base.Source = extra.Source
		base.Confidence = extra.Confidence
		return base
	}
	if extra.Source != "" && base.Source != extra.Source {
		base.Source = "multimodal"
		if extra.Confidence != "" && base.Confidence != extra.Confidence {
			base.Confidence = "mixed"
		}
	}
	if base.Confidence == "" {
		base.Confidence = extra.Confidence
	}
	return base
}

func tagParsedDraftSource(draft parsedDraft, prefix string) parsedDraft {
	if !draftHasFields(draft) {
		return draft
	}
	if strings.TrimSpace(draft.Source) == "" {
		draft.Source = prefix
		return draft
	}
	draft.Source = prefix + "_" + draft.Source
	return draft
}

func draftHasFields(draft parsedDraft) bool {
	return draft.Name != "" || draft.ExpiresOn != nil
}

func draftIsComplete(draft parsedDraft) bool {
	return draft.Name != "" && draft.ExpiresOn != nil
}

func visionAnchorHint(draft parsedDraft) string {
	if strings.TrimSpace(draft.RawDeadlinePhrase) != "" {
		return draft.RawDeadlinePhrase
	}
	if draft.ExpiresOn != nil {
		return draft.ExpiresOn.Format("2006-01-02")
	}
	return ""
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

func hasMeaningfulOCRSignal(cleaned string) bool {
	normalized := normalizeFreeText(cleaned)
	if normalized == "" || looksLikeDiagnosticText(normalized) {
		return false
	}
	tokens := ocrSignalTokenPattern.FindAllString(normalized, -1)
	if len(tokens) == 0 {
		return false
	}
	wordTokens := 0
	hasDateSignal := false
	for _, raw := range tokens {
		token := strings.ToLower(strings.TrimSpace(raw))
		if token == "" {
			continue
		}
		if looksLikeOCRDateSignalToken(token) {
			hasDateSignal = true
		}
		if isMeaningfulOCRWordToken(token) {
			wordTokens++
		}
	}
	if wordTokens >= 2 {
		return true
	}
	if hasDateSignal && wordTokens >= 1 {
		return true
	}
	return false
}

func looksLikeOCRDateSignalToken(token string) bool {
	if token == "" {
		return false
	}
	if strings.IndexFunc(token, unicode.IsDigit) >= 0 {
		return true
	}
	_, ok := ocrDateSignalTokens[strings.ToLower(token)]
	return ok
}

func isMeaningfulOCRWordToken(token string) bool {
	if token == "" || looksLikeOCRDateSignalToken(token) || !tokenHasLetters(token) {
		return false
	}
	letters := letterRuneCount(token)
	if letters < 2 {
		return false
	}
	if isLatinOnlyName(token) {
		return letters >= 3
	}
	return true
}

func hasLatinTokenOverlap(cleaned, name string) bool {
	ocrTokens := latinTokenSet(cleaned)
	if len(ocrTokens) == 0 {
		return false
	}
	for token := range latinTokenSet(name) {
		if _, ok := ocrTokens[token]; ok {
			return true
		}
	}
	return false
}

func latinTokenSet(input string) map[string]struct{} {
	tokens := ocrSignalTokenPattern.FindAllString(strings.ToLower(input), -1)
	set := make(map[string]struct{}, len(tokens))
	for _, token := range tokens {
		if !isLatinOnlyName(token) || letterRuneCount(token) < 2 {
			continue
		}
		set[token] = struct{}{}
	}
	return set
}

func isLatinOnlyName(value string) bool {
	hasLetters := false
	for _, r := range value {
		if !unicode.IsLetter(r) {
			continue
		}
		hasLetters = true
		if !unicode.In(r, unicode.Latin) {
			return false
		}
	}
	return hasLetters
}

func tokenHasLetters(value string) bool {
	for _, r := range value {
		if unicode.IsLetter(r) {
			return true
		}
	}
	return false
}

func letterRuneCount(value string) int {
	count := 0
	for _, r := range value {
		if unicode.IsLetter(r) {
			count++
		}
	}
	return count
}

func looksLikePhotoBoilerplateName(value string) bool {
	lower := strings.ToLower(normalizeFreeText(value))
	if lower == "" {
		return false
	}
	markers := []string{
		"срок год",
		"годен",
		"годна",
		"годно",
		"употреб",
		"best before",
		"use by",
		"sell by",
		"expires",
		"expiry",
		"exp",
	}
	for _, marker := range markers {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func looksLikeReceiptOCR(cleaned string) bool {
	if strings.TrimSpace(cleaned) == "" {
		return false
	}
	lower := strings.ToLower(cleaned)
	lines := 0
	priceLikeLines := 0
	for _, line := range strings.Split(cleaned, "\n") {
		line = normalizeFreeText(line)
		if line == "" {
			continue
		}
		lines++
		if strings.Contains(line, "₽") || strings.Contains(line, "руб") {
			priceLikeLines++
			continue
		}
		if matches := receiptPriceLikePattern.FindAllString(line, -1); len(matches) > 0 {
			priceLikeLines++
		}
	}
	markers := 0
	for _, marker := range []string{
		"чек",
		"кассовый",
		"итог",
		"сумма",
		"безнал",
		"налич",
		"руб",
		"₽",
	} {
		if strings.Contains(lower, marker) {
			markers++
		}
	}
	if strings.Contains(lower, "кассовый чек") {
		return true
	}
	if lines >= 3 && markers >= 2 {
		return true
	}
	return markers >= 1 && priceLikeLines >= 2
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

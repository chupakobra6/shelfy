package ingest

import (
	"fmt"
	"strings"
	"time"

	"github.com/igor/shelfy/internal/domain"
)

type parsedDraft struct {
	Name              string
	ExpiresOn         *time.Time
	LockedExpiry      bool
	RawDeadlinePhrase string
	Confidence        string
	Source            string
}

func finalizeParsedTextDraft(cleaned string, result parsedDraft) (parsedDraft, error) {
	if result.Name == "" && result.ExpiresOn == nil {
		return parsedDraft{}, fmt.Errorf("unable to extract any draft fields")
	}
	result.Name = normalizeDraftName(result.Name)
	if result.Name == "" && result.ExpiresOn == nil {
		return parsedDraft{}, fmt.Errorf("unable to extract any draft fields")
	}
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
			resolvedPhrase, resolved := resolveDraftDeadlinePhrase(phrase, now)
			if resolved.Value == nil {
				continue
			}
			if normalizedName := normalizeDraftName(name); normalizedName == "" || isDateOnlyResidualText(normalizedName, now) {
				rawPhrase := strings.TrimSpace(strings.TrimSpace(marker) + " " + resolvedPhrase)
				return parsedDraft{
					ExpiresOn:         resolved.Value,
					LockedExpiry:      resolved.Absolute,
					RawDeadlinePhrase: rawPhrase,
					Confidence:        resolved.Confidence,
					Source:            "heuristic_marker_date_only",
				}
			}
			return withNormalizedDraftName(parsedDraft{
				Name:              name,
				ExpiresOn:         resolved.Value,
				LockedExpiry:      resolved.Absolute,
				RawDeadlinePhrase: resolvedPhrase,
				Confidence:        resolved.Confidence,
				Source:            "heuristic_marker",
			})
		}
	}
	if name, phrase, resolved, ok := splitTrailingDatePhrase(cleaned, now); ok {
		return withNormalizedDraftName(parsedDraft{
			Name:              name,
			ExpiresOn:         resolved.Value,
			LockedExpiry:      resolved.Absolute,
			RawDeadlinePhrase: phrase,
			Confidence:        resolved.Confidence,
			Source:            "heuristic_suffix",
		})
	}
	if name, phrase, resolved, ok := extractNaturalDateFromText(cleaned, now); ok {
		return withNormalizedDraftName(parsedDraft{
			Name:              name,
			ExpiresOn:         resolved.Value,
			LockedExpiry:      resolved.Absolute,
			RawDeadlinePhrase: phrase,
			Confidence:        resolved.Confidence,
			Source:            "heuristic_natural",
		})
	}
	if phrase, resolved, ok := extractNaturalDateOnlyFromText(cleaned, now); ok {
		return parsedDraft{
			ExpiresOn:         resolved.Value,
			LockedExpiry:      resolved.Absolute,
			RawDeadlinePhrase: phrase,
			Confidence:        resolved.Confidence,
			Source:            "heuristic_date_only_natural",
		}
	}
	resolved := domain.ResolveRelativeDate(cleaned, now)
	if resolved.Value != nil {
		phrase, validated := validateResolvedDeadlinePhrase(cleaned, resolved, now)
		if validated.Value == nil {
			goto nameOnly
		}
		return parsedDraft{
			ExpiresOn:         validated.Value,
			LockedExpiry:      validated.Absolute,
			RawDeadlinePhrase: phrase,
			Confidence:        validated.Confidence,
			Source:            "heuristic_date_only",
		}
	}
nameOnly:
	return withNormalizedDraftName(parsedDraft{
		Name:       normalizeDraftName(cleaned),
		Confidence: "low",
		Source:     "heuristic_name_only",
	})
}

func resolveDraftDeadlinePhrase(raw string, now time.Time) (string, domain.ResolvedDate) {
	phrase := strings.TrimSpace(normalizeFreeText(raw))
	if phrase == "" {
		return "", domain.ResolvedDate{Confidence: "missing"}
	}
	resolved := domain.ResolveRelativeDate(phrase, now)
	if resolved.Value != nil {
		return validateResolvedDeadlinePhrase(phrase, resolved, now)
	}
	if extracted, ok := domain.ExtractDateFromText(phrase, now); ok && extracted.Value != nil {
		return validateResolvedDeadlinePhrase(extracted.Phrase, domain.ResolvedDate{
			Value:      extracted.Value,
			Confidence: extracted.Confidence,
			Absolute:   extracted.Absolute,
		}, now)
	}
	return phrase, resolved
}

func normalizeDraftName(input string) string {
	name := strings.TrimSpace(strings.Trim(normalizeFreeText(input), ".,;:!?"))
	if name == "" {
		return ""
	}
	name = rewriteNormalizedPhrases(name, draftNamePhraseRewrites)
	for {
		lower := strings.ToLower(name)
		changed := false
		for _, prefix := range draftFillerPrefixes {
			if lower != prefix && !strings.HasPrefix(lower, prefix+" ") {
				continue
			}
			candidate := strings.TrimSpace(name[len(prefix):])
			if candidate == "" || candidate == name {
				continue
			}
			name = strings.TrimSpace(candidate)
			changed = true
			break
		}
		if changed {
			continue
		}
		for _, prefix := range draftActionPrefixes {
			if lower != prefix && !strings.HasPrefix(lower, prefix+" ") {
				continue
			}
			candidate := strings.TrimSpace(name[len(prefix):])
			if candidate == "" || candidate == name {
				continue
			}
			name = strings.TrimSpace(candidate)
			changed = true
			break
		}
		if !changed {
			break
		}
	}
	if canonical, ok := canonicalProductPhrase(name); ok {
		return canonical
	}
	name = trimDraftLeadingNoise(name)
	if canonical, ok := canonicalProductPhrase(name); ok {
		return canonical
	}
	name = trimDraftTrailingNoise(name)
	name = trimDraftLeadingNoise(name)
	name = rewriteNormalizedPhrases(name, draftNamePhraseRewrites)
	if canonical, ok := canonicalProductPhrase(name); ok {
		return canonical
	}
	name = canonicalizeDraftLeadToken(name)
	if canonical, ok := canonicalProductPhrase(name); ok {
		return canonical
	}
	return strings.TrimSpace(name)
}

func withNormalizedDraftName(draft parsedDraft) parsedDraft {
	draft.Name = normalizeDraftName(draft.Name)
	return draft
}

func splitTrailingDatePhrase(cleaned string, now time.Time) (name string, phrase string, resolved domain.ResolvedDate, ok bool) {
	parts := strings.Fields(cleaned)
	if len(parts) < 2 {
		return "", "", domain.ResolvedDate{}, false
	}
	maxTail := 5
	if len(parts)-1 < maxTail {
		maxTail = len(parts) - 1
	}
	for tailSize := maxTail; tailSize >= 1; tailSize-- {
		phrase = strings.Join(parts[len(parts)-tailSize:], " ")
		resolved = domain.ResolveRelativeDate(phrase, now)
		if resolved.Value == nil {
			continue
		}
		phrase, resolved = validateResolvedDeadlinePhrase(phrase, resolved, now)
		if resolved.Value == nil {
			continue
		}
		if tailSize > 1 && !looksLikeDatePhraseLead(parts[len(parts)-tailSize], now) {
			continue
		}
		name = strings.TrimSpace(strings.Join(parts[:len(parts)-tailSize], " "))
		if name == "" || isDatePrefixOnly(name) {
			return "", phrase, resolved, true
		}
		if isDateOnlyResidualText(normalizeDraftName(name), now) {
			return "", phrase, resolved, true
		}
		return name, phrase, resolved, true
	}
	return "", "", domain.ResolvedDate{}, false
}

func looksLikeDatePhraseLead(token string, now time.Time) bool {
	normalized := normalizedPolicyText(token)
	if normalized == "" {
		return false
	}
	switch normalized {
	case "до", "к", "на", "в", "во", "через":
		return true
	}
	return hasDateSignal(normalized, now)
}

func extractNaturalDateFromText(cleaned string, now time.Time) (name string, phrase string, resolved domain.ResolvedDate, ok bool) {
	extracted, ok := domain.ExtractDateFromText(cleaned, now)
	if !ok || extracted.Value == nil {
		return "", "", domain.ResolvedDate{}, false
	}
	phrase, resolved = validateResolvedDeadlinePhrase(extracted.Phrase, domain.ResolvedDate{
		Value:      extracted.Value,
		Confidence: extracted.Confidence,
		Absolute:   extracted.Absolute,
	}, now)
	if resolved.Value == nil {
		return "", "", domain.ResolvedDate{}, false
	}
	name = removeDatePhrase(cleaned, extracted.Phrase)
	if name == "" || name == cleaned || isDatePrefixOnly(name) {
		return "", phrase, resolved, true
	}
	if isDateOnlyResidualText(normalizeDraftName(name), now) {
		return "", phrase, resolved, true
	}
	return name, phrase, resolved, true
}

func extractNaturalDateOnlyFromText(cleaned string, now time.Time) (string, domain.ResolvedDate, bool) {
	extracted, ok := domain.ExtractDateFromText(cleaned, now)
	if !ok || extracted.Value == nil {
		return "", domain.ResolvedDate{}, false
	}
	phrase, resolved := validateResolvedDeadlinePhrase(extracted.Phrase, domain.ResolvedDate{
		Value:      extracted.Value,
		Confidence: extracted.Confidence,
		Absolute:   extracted.Absolute,
	}, now)
	if resolved.Value == nil {
		return "", domain.ResolvedDate{}, false
	}
	remainder := normalizeDraftName(removeDatePhrase(cleaned, extracted.Phrase))
	if remainder == "" || isDateOnlyResidualText(remainder, now) {
		return phrase, resolved, true
	}
	return "", domain.ResolvedDate{}, false
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

func isDateOnlyResidualText(value string, now time.Time) bool {
	normalized := normalizedPolicyText(value)
	if normalized == "" {
		return true
	}
	if containsFoodLexiconSignal(normalized) {
		return false
	}
	if looksLikeRejectTaskInput(normalized, parsedDraft{}) {
		return false
	}
	allowed := map[string]struct{}{
		"слушай": {}, "надо": {}, "нужно": {}, "пожалуйста": {}, "чтобы": {}, "было": {},
		"мне": {}, "бы": {}, "короче": {}, "это": {}, "давай": {}, "успеть": {},
	}
	for _, token := range strings.Fields(normalized) {
		if _, ok := allowed[token]; ok {
			continue
		}
		if hasDateSignal(token, now) {
			continue
		}
		return false
	}
	return true
}

func trimDraftTrailingNoise(name string) string {
	if canonical, ok := canonicalProductPhrase(name); ok {
		return canonical
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	lower := " " + strings.ToLower(name) + " "
	for _, phrase := range draftTrailingCutPhrases {
		if idx := strings.Index(lower, phrase); idx >= 0 {
			start := idx - 1
			if start < 0 {
				start = 0
			}
			name = strings.TrimSpace(name[:start])
			lower = " " + strings.ToLower(name) + " "
		}
	}
	tokens := strings.Fields(name)
	cut := len(tokens)
	for i, token := range tokens {
		normalized := normalizedPolicyText(token)
		if normalized == "" {
			continue
		}
		if protected, ok := canonicalProductPhrase(strings.Join(tokens[:i+1], " ")); ok {
			return protected
		}
		if _, ok := draftTrailingCutTokens[normalized]; ok && i > 0 {
			cut = i
			break
		}
		if (normalized == "из" || normalized == "с") && i+1 < len(tokens) {
			if _, ok := storeNoiseTokens[normalizedPolicyText(tokens[i+1])]; ok {
				cut = i
				break
			}
		}
	}
	if cut == 0 {
		return ""
	}
	trimmed := strings.Fields(strings.TrimSpace(strings.Join(tokens[:cut], " ")))
	for len(trimmed) > 1 {
		last := normalizedPolicyText(trimmed[len(trimmed)-1])
		if last != "и" && last != "или" && last != "с" && last != "со" && last != "из" {
			break
		}
		trimmed = trimmed[:len(trimmed)-1]
	}
	return strings.TrimSpace(strings.Join(trimmed, " "))
}

func trimDraftLeadingNoise(name string) string {
	trimmed := strings.TrimSpace(name)
	if canonical, ok := canonicalProductPhrase(trimmed); ok {
		return canonical
	}
	for _, phrase := range draftLeadingNoisePhrases {
		prefix := phrase + " "
		if strings.HasPrefix(normalizedPolicyText(trimmed), prefix) {
			trimmed = strings.TrimSpace(trimmed[len(phrase):])
		}
	}
	tokens := strings.Fields(trimmed)
	start := 0
	for start < len(tokens) {
		normalized := normalizedPolicyText(tokens[start])
		if normalized == "" {
			start++
			continue
		}
		if normalized == "свежий" || normalized == "свежая" || normalized == "свежие" || normalized == "свежих" {
			start++
			continue
		}
		if _, ok := storeNoiseTokens[normalized]; ok {
			start++
			continue
		}
		if _, ok := draftTrailingCutTokens[normalized]; !ok {
			break
		}
		start++
	}
	if start >= len(tokens) {
		return ""
	}
	return strings.TrimSpace(strings.Join(tokens[start:], " "))
}

func canonicalizeDraftLeadToken(name string) string {
	tokens := strings.Fields(strings.TrimSpace(name))
	if len(tokens) == 0 {
		return ""
	}
	if canonical, ok := productCanonicalLeadToken[normalizedPolicyText(tokens[0])]; ok {
		tokens[0] = canonical
	}
	return strings.Join(tokens, " ")
}

func validateResolvedDeadlinePhrase(phrase string, resolved domain.ResolvedDate, now time.Time) (string, domain.ResolvedDate) {
	normalized := normalizedPolicyText(phrase)
	if normalized == "" {
		return "", domain.ResolvedDate{Confidence: "missing"}
	}
	if isLikelyFalseDatePhrase(normalized) {
		return "", domain.ResolvedDate{Confidence: "unknown"}
	}
	hasDateSignal := hasDateSignalWithoutWhen(normalized, now)
	if containsFoodLexiconSignal(normalized) {
		return "", domain.ResolvedDate{Confidence: "unknown"}
	}
	if containsAnyToken(normalized, packagingNoiseTokens) || containsAnyToken(normalized, deliveryNoiseTokens) {
		return "", domain.ResolvedDate{Confidence: "unknown"}
	}
	if containsAnyToken(normalized, quantityNoiseTokens) && !hasDateSignal {
		return "", domain.ResolvedDate{Confidence: "unknown"}
	}
	if containsAnyToken(normalized, timeOnlyNoiseTokens) && !hasDateSignal {
		return "", domain.ResolvedDate{Confidence: "unknown"}
	}
	if !hasDateSignal && resolved.Value != nil {
		return "", domain.ResolvedDate{Confidence: "unknown"}
	}
	return strings.TrimSpace(phrase), resolved
}

func hasDateSignalWithoutWhen(input string, now time.Time) bool {
	normalized := normalizedPolicyText(input)
	if normalized == "" {
		return false
	}
	if isLikelyFalseDatePhrase(normalized) {
		return false
	}
	if strings.IndexFunc(normalized, func(r rune) bool { return r >= '0' && r <= '9' }) >= 0 {
		return true
	}
	padded := " " + normalized + " "
	for _, token := range voiceDateSignalTokens {
		if strings.Contains(padded, " "+token+" ") {
			return true
		}
	}
	if resolved := domain.ResolveRelativeDate(normalized, now); resolved.Value != nil {
		return resolved.Value != nil
	}
	if extracted, ok := domain.ExtractDateFromText(normalized, now); ok && extracted.Value != nil {
		return true
	}
	return false
}

func isLikelyFalseDatePhrase(normalized string) bool {
	padded := " " + normalizedPolicyText(normalized) + " "
	for _, marker := range []string{" десяток ", " завтрак ", " к завтраку ", " полчаса ", " через полчаса ", " час ", " часа ", " часов "} {
		if strings.Contains(padded, marker) {
			return true
		}
	}
	return false
}

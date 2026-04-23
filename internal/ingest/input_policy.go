package ingest

import (
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/igor/shelfy/internal/domain"
)

func normalizeVoiceTranscript(input string) string {
	normalized := normalizedPolicyText(input)
	if normalized == "" {
		return ""
	}
	normalized = rewriteNormalizedPhrases(normalized, voiceTranscriptPhraseRewrites)
	tokens := strings.Fields(normalized)
	for i, token := range tokens {
		tokens[i] = correctVoiceToken(token)
	}
	tokens = normalizeVoiceDateGrammar(tokens)
	return rewriteNormalizedPhrases(strings.Join(tokens, " "), voiceTranscriptPhraseRewrites)
}

func normalizeVoiceDateGrammar(tokens []string) []string {
	if len(tokens) < 2 {
		return tokens
	}
	out := append([]string(nil), tokens...)
	for i := 1; i < len(out); i++ {
		if out[i] == "число" && looksLikeOrdinalGenitive(out[i-1]) {
			out[i] = "числа"
		}
	}
	return out
}

func looksLikeOrdinalGenitive(token string) bool {
	token = normalizedPolicyText(token)
	if token == "" {
		return false
	}
	return strings.HasSuffix(token, "ого") || strings.HasSuffix(token, "его")
}

func normalizeIntentInput(input string) string {
	normalized := normalizedPolicyText(input)
	if normalized == "" {
		return ""
	}
	for _, rewrite := range []phraseRewrite{
		{From: "со вкусом сыра", To: ""},
		{From: "со вкусом", To: ""},
		{From: "с саусеп", To: ""},
	} {
		normalized = rewriteNormalizedPhrases(normalized, []phraseRewrite{rewrite})
	}
	return strings.Join(strings.Fields(normalized), " ")
}

func normalizedPolicyText(input string) string {
	value := strings.ToLower(normalizeFreeText(input))
	value = strings.ReplaceAll(value, "ё", "е")
	value = strings.Map(func(r rune) rune {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r):
			return r
		default:
			return ' '
		}
	}, value)
	return strings.Join(strings.Fields(value), " ")
}

func containsFoodLexiconSignal(input string) bool {
	normalized := normalizedPolicyText(input)
	if normalized == "" {
		return false
	}
	return len(foodLexiconMatchesNormalized(normalized)) > 0
}

func foodLexiconMatchesNormalized(normalized string) []string {
	tokens := strings.Fields(normalized)
	if len(tokens) == 0 {
		return nil
	}
	matches := make([]string, 0, 4)
	for i := 0; i < len(tokens); {
		bestLen := 0
		bestPhrase := ""
		for _, phrase := range foodLexiconPhrases {
			parts := strings.Fields(phrase)
			if len(parts) <= bestLen || i+len(parts) > len(tokens) {
				continue
			}
			matched := true
			for j := range parts {
				if tokens[i+j] != parts[j] {
					matched = false
					break
				}
			}
			if matched {
				bestLen = len(parts)
				bestPhrase = phrase
			}
		}
		if bestLen == 0 {
			i++
			continue
		}
		matches = append(matches, bestPhrase)
		i += bestLen
	}
	return uniqueStrings(matches)
}

func looksLikeRejectTaskInput(normalized string, draft parsedDraft) bool {
	normalized = normalizeIntentInput(normalized)
	if strings.TrimSpace(normalized) == "" {
		return false
	}
	if isGenericContainerName(draft.Name) {
		return true
	}
	if containsRejectIntentPhrase(normalized) {
		return true
	}
	if containsFoodLexiconSignal(normalized) {
		return false
	}
	if containsFoodLexiconSignal(draft.Name) {
		return false
	}
	if draft.Name == "" && draft.ExpiresOn != nil {
		return false
	}
	padded := " " + normalized + " "
	if draft.Name != "" && letterRuneCount(draft.Name) > 0 && len(strings.Fields(normalized)) <= 2 {
		return false
	}
	if strings.Contains(padded, " на покрывало ") {
		return true
	}
	return len(strings.Fields(normalized)) >= 4
}

func containsRejectIntentPhrase(normalized string) bool {
	padded := " " + normalizeIntentInput(normalized) + " "
	for _, phrase := range rejectIntentPhrases {
		if strings.Contains(padded, " "+phrase+" ") {
			return true
		}
	}
	return false
}

func hasDateSignal(input string, now time.Time) bool {
	normalized := normalizedPolicyText(input)
	if normalized == "" {
		return false
	}
	if strings.IndexFunc(normalized, unicode.IsDigit) >= 0 {
		return true
	}
	padded := " " + normalized + " "
	for _, token := range voiceDateSignalTokens {
		if strings.Contains(padded, " "+token+" ") {
			return true
		}
	}
	if resolved := domain.ResolveRelativeDate(normalized, now); resolved.Value != nil {
		return true
	}
	if extracted, ok := domain.ExtractDateFromText(normalized, now); ok && extracted.Value != nil {
		return true
	}
	return false
}

func correctVoiceToken(token string) string {
	if token == "" || strings.IndexFunc(token, unicode.IsDigit) >= 0 || !tokenHasLetters(token) {
		return token
	}
	if _, ok := canonicalProductPhrase(token); ok {
		return token
	}
	if shouldSkipVoiceTokenCorrection(token) {
		return token
	}
	if containsFoodLexiconSignal(token) {
		return token
	}
	best := token
	bestDistance := correctionDistanceLimit(token) + 1
	for _, candidate := range foodLexiconTokens {
		if utf8.RuneCountInString(candidate) != utf8.RuneCountInString(token) && absInt(utf8.RuneCountInString(candidate)-utf8.RuneCountInString(token)) > 2 {
			continue
		}
		if firstTokenRune(token) != firstTokenRune(candidate) {
			continue
		}
		distance := tokenLevenshtein(token, candidate)
		if distance < bestDistance {
			best = candidate
			bestDistance = distance
		}
	}
	return best
}

func correctionDistanceLimit(token string) int {
	switch n := utf8.RuneCountInString(token); {
	case n <= 3:
		return 0
	case n <= 6:
		return 1
	default:
		return 2
	}
}

func shouldSkipVoiceTokenCorrection(token string) bool {
	normalized := normalizedPolicyText(token)
	if normalized == "" {
		return true
	}
	for _, value := range voiceNoCorrectionTokens {
		if normalized == value {
			return true
		}
	}
	return false
}

func firstTokenRune(value string) rune {
	r, _ := utf8.DecodeRuneInString(value)
	return r
}

func tokenLevenshtein(left, right string) int {
	if left == right {
		return 0
	}
	a := []rune(left)
	b := []rune(right)
	if absInt(len(a)-len(b)) > 2 {
		return 3
	}
	column := make([]int, len(b)+1)
	for i := range column {
		column[i] = i
	}
	for i, ra := range a {
		prevDiagonal := column[0]
		column[0] = i + 1
		for j, rb := range b {
			insertCost := column[j+1] + 1
			deleteCost := column[j] + 1
			replaceCost := prevDiagonal
			if ra != rb {
				replaceCost++
			}
			prevDiagonal = column[j+1]
			column[j+1] = minInt(insertCost, deleteCost, replaceCost)
		}
	}
	return column[len(b)]
}

func minInt(values ...int) int {
	best := values[0]
	for _, value := range values[1:] {
		if value < best {
			best = value
		}
	}
	return best
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

package ingest

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/igor/shelfy/internal/domain"
	"github.com/igor/shelfy/internal/jobs"
)

type reviewCleaner struct {
	CleanedInput string
	ReasonCode   string
}

type reviewRun struct {
	Cleaner     reviewCleaner
	CleanedText string
	Candidate   parsedDraft
	CandidateOK bool
}

func cloneDraftPayload(input map[string]any) map[string]any {
	if len(input) == 0 {
		return map[string]any{}
	}
	out := make(map[string]any, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func mergeDraftPayload(base map[string]any, updates map[string]any) map[string]any {
	out := cloneDraftPayload(base)
	for key, value := range updates {
		out[key] = value
	}
	return out
}

func buildDraftPayload(base map[string]any, parsed parsedDraft, payload jobs.IngestPayload, rawTranscript, normalizedTranscript string) map[string]any {
	meta := cloneDraftPayload(base)
	resetReviewPayload(meta)
	meta[domain.DraftPayloadKeyFastSource] = parsed.Source
	meta[domain.DraftPayloadKeyFastConfidence] = parsed.Confidence
	meta[domain.DraftPayloadKeySmartReviewAttempted] = false
	meta[domain.DraftPayloadKeyReviewApplied] = false
	if strings.TrimSpace(payload.Text) != "" {
		meta[domain.DraftPayloadKeyOriginalText] = strings.TrimSpace(payload.Text)
	}
	if strings.TrimSpace(rawTranscript) != "" {
		meta[domain.DraftPayloadKeyRawTranscript] = strings.TrimSpace(rawTranscript)
	}
	if strings.TrimSpace(normalizedTranscript) != "" {
		meta[domain.DraftPayloadKeyNormalizedTranscript] = strings.TrimSpace(normalizedTranscript)
	}
	return meta
}

func buildReviewPayload(payload jobs.IngestPayload, rawTranscript, normalizedTranscript string) jobs.RefineDraftAIPayload {
	return jobs.RefineDraftAIPayload{
		TraceID:              payload.TraceID,
		UserID:               payload.UserID,
		ChatID:               payload.ChatID,
		SourceKind:           payload.Kind,
		OriginalText:         strings.TrimSpace(payload.Text),
		RawTranscript:        strings.TrimSpace(rawTranscript),
		NormalizedTranscript: strings.TrimSpace(normalizedTranscript),
	}
}

func formatOptionalDraftDate(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.Format("2006-01-02")
}

func draftStateFromSession(draft domain.DraftSession) string {
	hasName := strings.TrimSpace(draft.DraftName) != ""
	hasDate := draft.DraftExpiresOn != nil
	switch {
	case hasName && hasDate:
		return "ready"
	case hasName:
		return "needs_expiry"
	case hasDate:
		return "needs_name"
	default:
		return "reject"
	}
}

func parseFastDraft(text string, now time.Time) (parsedDraft, error) {
	cleaned := normalizeFreeText(text)
	result := heuristicParse(cleaned, now)
	return finalizeParsedTextDraft(cleaned, result)
}

func buildCleanerCandidate(now time.Time, cleaner reviewCleaner, fallbackText string) (parsedDraft, string, error) {
	cleanedInput := reviewCleanedInput(cleaner, fallbackText)
	candidate := heuristicParse(cleanedInput, now)
	if !reviewCandidateSupportedByInput(candidate.Name, cleanedInput) {
		return parsedDraft{}, cleanedInput, fmt.Errorf("review candidate name is not supported by input")
	}
	if candidate.Name != "" || candidate.ExpiresOn != nil {
		candidate.Source = "ollama-cleaner"
		candidate.Confidence = "review"
	}
	finalized, err := finalizeParsedTextDraft(cleanedInput, candidate)
	if err != nil {
		return parsedDraft{}, cleanedInput, err
	}
	return finalized, cleanedInput, nil
}

func draftCompletenessRank(draft parsedDraft) int {
	switch {
	case strings.TrimSpace(draft.Name) != "" && draft.ExpiresOn != nil:
		return 3
	case strings.TrimSpace(draft.Name) != "" || draft.ExpiresOn != nil:
		return 2
	default:
		return 0
	}
}

func reviewApplyDecision(current domain.DraftSession, candidate parsedDraft) (bool, string) {
	if current.Status != domain.DraftStatusReady {
		return false, "draft_not_ready"
	}
	currentHasName := strings.TrimSpace(current.DraftName) != ""
	currentHasDate := current.DraftExpiresOn != nil
	candidateHasName := strings.TrimSpace(candidate.Name) != ""
	candidateHasDate := candidate.ExpiresOn != nil
	if !candidateHasName && !candidateHasDate {
		return false, "candidate_empty"
	}
	if currentHasName && !candidateHasName {
		return false, "candidate_drops_name"
	}
	if currentHasDate && !candidateHasDate {
		return false, "candidate_drops_date"
	}
	currentName := normalizeDraftName(current.DraftName)
	candidateName := normalizeDraftName(candidate.Name)
	currentRank := 0
	switch {
	case currentHasName && currentHasDate:
		currentRank = 3
	case currentHasName || currentHasDate:
		currentRank = 2
	}
	candidateRank := draftCompletenessRank(candidate)
	if candidateRank > currentRank {
		return true, "adds_missing_field"
	}
	if candidateRank < currentRank {
		return false, "candidate_less_complete"
	}
	if currentHasDate && candidateHasDate && !sameDraftDate(current.DraftExpiresOn, candidate.ExpiresOn) {
		return false, "candidate_changes_date"
	}
	if currentHasName && candidateHasName {
		if currentName == candidateName {
			return false, "same_candidate"
		}
		if candidateRemovesOnlyNoise(currentName, candidateName) {
			return true, "removes_noise"
		}
		if nameLooksBetter(currentName, candidateName) {
			return true, "better_name"
		}
		return false, "name_not_better"
	}
	if !currentHasDate && candidateHasDate {
		return true, "adds_date"
	}
	if !currentHasName && candidateHasName {
		return true, "adds_name"
	}
	return false, "no_improvement"
}

func sameDraftDate(current *time.Time, candidate *time.Time) bool {
	if current == nil && candidate == nil {
		return true
	}
	if current == nil || candidate == nil {
		return false
	}
	return current.Format("2006-01-02") == candidate.Format("2006-01-02")
}

func nameLooksBetter(current, candidate string) bool {
	if strings.TrimSpace(candidate) == "" || current == candidate {
		return false
	}
	if candidateRemovesOnlyNoise(current, candidate) {
		return true
	}
	if currentCanonical, ok := canonicalProductPhrase(current); ok && currentCanonical == candidate {
		return true
	}
	if len(candidate) <= len(current) {
		return false
	}
	return strings.Contains(candidate, current) || containsFoodLexiconSignal(candidate)
}

var reviewNameNoiseTokens = map[string]struct{}{
	"я": {}, "хочу": {}, "заказать": {}, "закажи": {}, "мне": {}, "нужна": {}, "нужен": {}, "нужно": {},
	"пожалуйста": {}, "алиса": {}, "салют": {}, "джой": {}, "сири": {},
	"ты": {}, "экс": {}, "окей": {}, "ок": {}, "такс": {}, "тэк": {}, "ум": {}, "бессилием": {},
	"а": {}, "по": {},
	"в": {}, "пакетиках": {}, "доставкой": {}, "доставка": {}, "на": {}, "дом": {},
	"силе": {}, "записать": {}, "запиши": {}, "тут": {}, "меня": {},
	"срок": {}, "годности": {},
	"как": {}, "можно": {}, "быстрее": {}, "из": {}, "магазина": {}, "фирмы": {},
	"объемом": {}, "объем": {}, "емкостью": {}, "литр": {}, "литра": {}, "литров": {},
	"полкилограмма": {}, "килограмм": {}, "килограмма": {}, "килограммов": {},
	"штука": {}, "штуки": {}, "штук": {}, "ультрапастеризованное": {}, "ультрапастеризованный": {},
	"процента": {}, "процент": {}, "процентов": {}, "жирностью": {}, "жирность": {},
}

func candidateRemovesOnlyNoise(current, candidate string) bool {
	currentTokens := strings.Fields(current)
	candidateTokens := strings.Fields(candidate)
	if len(candidateTokens) == 0 || len(candidateTokens) >= len(currentTokens) {
		return false
	}
	removed := make([]string, 0, len(currentTokens)-len(candidateTokens))
	index := 0
	for _, token := range currentTokens {
		if index < len(candidateTokens) && token == candidateTokens[index] {
			index++
			continue
		}
		removed = append(removed, token)
	}
	if index != len(candidateTokens) || len(removed) == 0 {
		return false
	}
	for _, token := range removed {
		if _, ok := reviewNameNoiseTokens[token]; !ok {
			return false
		}
	}
	return true
}

func reviewCandidateSupportedByInput(name, cleanedInput string) bool {
	name = normalizeDraftName(name)
	if strings.TrimSpace(name) == "" {
		return true
	}
	if canonical, ok := canonicalProductPhrase(cleanedInput); ok && normalizeDraftName(canonical) == name {
		return true
	}
	inputSet := map[string]struct{}{}
	for _, token := range strings.Fields(normalizedPolicyText(cleanedInput)) {
		inputSet[token] = struct{}{}
		if canonical, ok := productCanonicalLeadToken[token]; ok {
			inputSet[canonical] = struct{}{}
		}
	}
	for _, token := range strings.Fields(normalizedPolicyText(name)) {
		if _, ok := inputSet[token]; ok {
			continue
		}
		return false
	}
	return true
}

func shouldRunSmartReview(kind domain.MessageKind) bool {
	return kind == domain.MessageKindText || kind == domain.MessageKindVoice || kind == domain.MessageKindAudio
}

func reviewModelInput(payload jobs.RefineDraftAIPayload) string {
	if payload.SourceKind == domain.MessageKindText {
		return normalizeFreeText(payload.OriginalText)
	}
	return normalizeFreeText(payload.NormalizedTranscript)
}

func reviewCleanedInput(cleaner reviewCleaner, fallbackText string) string {
	cleaned := strings.TrimSpace(cleaner.CleanedInput)
	if cleaned == "" {
		cleaned = strings.TrimSpace(fallbackText)
	}
	return normalizeFreeText(cleaned)
}

func shouldSkipReviewForDraft(draft domain.DraftSession) bool {
	switch draft.Status {
	case domain.DraftStatusReady:
		return draft.DraftMessageID == nil
	default:
		return true
	}
}

func shouldAttemptReviewRescue(kind domain.MessageKind, input string, parseErr error) bool {
	if parseErr == nil || !shouldRunSmartReview(kind) {
		return false
	}
	normalized := normalizeIntentInput(input)
	if normalized == "" {
		return false
	}
	message := strings.ToLower(strings.TrimSpace(parseErr.Error()))
	switch {
	case strings.Contains(message, "text rejected by intent gate"):
		return true
	case strings.Contains(message, "unable to extract any draft fields"):
		return len(strings.Fields(normalized)) >= 2
	default:
		return false
	}
}

func finalizeReviewRescueDraft(cleanedInput string, draft parsedDraft) (parsedDraft, bool) {
	draft.Name = normalizeDraftName(draft.Name)
	if draft.Name == "" && draft.ExpiresOn == nil {
		return parsedDraft{}, false
	}
	if !reviewCandidateSupportedByInput(draft.Name, cleanedInput) {
		return parsedDraft{}, false
	}
	return draft, true
}

func renderReviewStatus(ctx context.Context, s *Service, draftID int64, status string, cleanedText string) error {
	draft, err := s.store.GetDraftSession(ctx, draftID)
	if err != nil {
		return err
	}
	if draft.Status != domain.DraftStatusReady || draft.DraftMessageID == nil {
		return nil
	}
	meta := cloneDraftPayload(draft.DraftPayload)
	if status == "" {
		delete(meta, domain.DraftPayloadKeyAIReviewStatus)
	} else {
		meta[domain.DraftPayloadKeyAIReviewStatus] = status
	}
	if strings.TrimSpace(cleanedText) != "" {
		meta[domain.DraftPayloadKeyReviewCleanedText] = strings.TrimSpace(cleanedText)
	}
	meta[domain.DraftPayloadKeySmartReviewAttempted] = true
	if err := s.store.UpdateDraftPayload(ctx, draftID, meta); err != nil {
		return err
	}
	return s.publishDraftCard(ctx, draftID)
}

func reviewMetadata(base map[string]any, cleaner reviewCleaner, cleanedText string, applied bool, applyReason string) map[string]any {
	meta := cloneDraftPayload(base)
	resetReviewPayload(meta)
	meta[domain.DraftPayloadKeySmartReviewAttempted] = true
	meta[domain.DraftPayloadKeyReviewReasonCode] = strings.TrimSpace(cleaner.ReasonCode)
	meta[domain.DraftPayloadKeyReviewApplyReason] = strings.TrimSpace(applyReason)
	meta[domain.DraftPayloadKeyReviewApplied] = applied
	if strings.TrimSpace(cleanedText) != "" {
		meta[domain.DraftPayloadKeyReviewCleanedText] = strings.TrimSpace(cleanedText)
	}
	return meta
}

func resetReviewPayload(meta map[string]any) {
	for _, key := range []string{
		domain.DraftPayloadKeyAIReviewStatus,
		domain.DraftPayloadKeyReviewCleanedText,
		domain.DraftPayloadKeyReviewReasonCode,
		domain.DraftPayloadKeyReviewApplyReason,
		domain.DraftPayloadKeyReviewApplied,
		"review_mode",
		"review_routing_reason",
		"review_cleaner_state",
		"review_cleaner_name",
		"review_cleaner_date",
		"review_cleaner_reason",
		"review_judge_confidence",
		"review_judge_decision",
		"review_judge_reason",
	} {
		delete(meta, key)
	}
}

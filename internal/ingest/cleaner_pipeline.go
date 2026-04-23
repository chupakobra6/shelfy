package ingest

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/igor/shelfy/internal/domain"
	"github.com/igor/shelfy/internal/observability"
)

type cleanerOutput struct {
	CleanedInput string
	ReasonCode   string
}

type draftBuildTrace struct {
	NormalizedInput     string
	CleanerCalled       bool
	CleanerChangedInput bool
	CandidateValid      bool
	CleanedInput        string
	CleanerReasonCode   string
	SelectionReason     string
	ChosenSource        string
}

type draftBuildResult struct {
	Draft parsedDraft
	Trace draftBuildTrace
}

func buildDraftTracePayload(trace draftBuildTrace) map[string]any {
	meta := map[string]any{
		domain.DraftPayloadKeyNormalizedInput: trace.NormalizedInput,
		domain.DraftPayloadKeyCleanerCalled:   trace.CleanerCalled,
		domain.DraftPayloadKeySelectionReason: trace.SelectionReason,
		domain.DraftPayloadKeyChosenSource:    trace.ChosenSource,
	}
	if trace.CleanerCalled && strings.TrimSpace(trace.CleanedInput) != "" {
		meta[domain.DraftPayloadKeyCleanedInput] = trace.CleanedInput
	}
	return meta
}

func withCleanerPending(meta map[string]any, pending bool) map[string]any {
	if meta == nil {
		meta = map[string]any{}
	}
	meta[domain.DraftPayloadKeyCleanerPending] = pending
	return meta
}

func mergeDraftPayload(base, overlay map[string]any) map[string]any {
	if len(base) == 0 && len(overlay) == 0 {
		return map[string]any{}
	}
	merged := make(map[string]any, len(base)+len(overlay))
	for key, value := range base {
		merged[key] = value
	}
	for key, value := range overlay {
		merged[key] = value
	}
	return merged
}

func buildInitialDraft(input string, now time.Time) (draftBuildResult, error) {
	normalizedInput := normalizeFreeText(input)
	trace := draftBuildTrace{
		NormalizedInput: normalizedInput,
		SelectionReason: "initial_fast_parse",
		ChosenSource:    "baseline",
	}
	draft, err := parseFastDraft(normalizedInput, now)
	if err != nil {
		return draftBuildResult{Trace: trace}, err
	}
	return draftBuildResult{Draft: draft, Trace: trace}, nil
}

func parseFastDraft(text string, now time.Time) (parsedDraft, error) {
	cleaned := normalizeFreeText(text)
	result := heuristicParse(cleaned, now)
	return finalizeParsedTextDraft(cleaned, result)
}

func (s *Service) runCleanerPass(ctx context.Context, kind domain.MessageKind, normalizedInput string, current domain.DraftSession, now time.Time) (draftBuildTrace, parsedDraft, bool) {
	trace := draftBuildTrace{
		NormalizedInput: strings.TrimSpace(normalizedInput),
		CleanerCalled:   true,
		SelectionReason: "same_candidate",
		ChosenSource:    "baseline",
	}

	cleaner, err := s.callCleanerForKind(ctx, kind, normalizedInput)
	if err != nil {
		trace.SelectionReason = "cleaner_failed"
		s.logger.WarnContext(ctx, "cleaner_failed", observability.LogAttrs(ctx,
			"kind", kind,
			"error", err,
		)...)
		return trace, parsedDraft{}, false
	}

	trace.CleanerReasonCode = strings.TrimSpace(cleaner.ReasonCode)
	trace.CleanedInput = normalizeCleanerOutput(cleaner, normalizedInput)
	trace.CleanerChangedInput = trace.CleanedInput != strings.TrimSpace(normalizedInput)

	candidate, _, err := parseCleanerCandidate(now, cleaner, normalizedInput)
	if err != nil {
		trace.SelectionReason = "candidate_invalid"
		s.logCleanerDecision(ctx, kind, "cleaner_kept_baseline", trace)
		return trace, parsedDraft{}, false
	}
	trace.CandidateValid = true

	apply, reason := shouldApplyBackgroundCleaner(current, candidate)
	trace.SelectionReason = reason
	if !apply {
		s.logCleanerDecision(ctx, kind, "cleaner_kept_baseline", trace)
		return trace, parsedDraft{}, false
	}

	trace.ChosenSource = "cleaner"
	s.logCleanerDecision(ctx, kind, "cleaner_selected_candidate", trace)
	return trace, candidate, true
}

func (s *Service) logCleanerDecision(ctx context.Context, kind domain.MessageKind, event string, trace draftBuildTrace) {
	s.logger.InfoContext(ctx, event, observability.LogAttrs(ctx,
		"kind", kind,
		"chosen_source", trace.ChosenSource,
		"selection_reason", trace.SelectionReason,
		"cleaner_reason_code", trace.CleanerReasonCode,
		"cleaner_changed_input", trace.CleanerChangedInput,
	)...)
}

func (s *Service) callCleanerForKind(ctx context.Context, kind domain.MessageKind, normalizedInput string) (cleanerOutput, error) {
	switch kind {
	case domain.MessageKindVoice, domain.MessageKindAudio:
		return s.callOllamaVoiceCleaner(ctx, normalizedInput)
	default:
		return s.callOllamaTextCleaner(ctx, normalizedInput)
	}
}

func parseCleanerCandidate(now time.Time, cleaner cleanerOutput, fallbackText string) (parsedDraft, string, error) {
	cleanedInput := normalizeCleanerOutput(cleaner, fallbackText)
	candidate := heuristicParse(cleanedInput, now)
	if !cleanerCandidateSupportedByInput(candidate.Name, cleanedInput) {
		return parsedDraft{}, cleanedInput, errUnsupportedCleanerCandidate
	}
	if candidate.Name != "" || candidate.ExpiresOn != nil {
		candidate.Source = "cleaner"
		candidate.Confidence = "review"
	}
	finalized, err := finalizeParsedTextDraft(cleanedInput, candidate)
	if err != nil {
		return parsedDraft{}, cleanedInput, err
	}
	return finalized, cleanedInput, nil
}

var errUnsupportedCleanerCandidate = errors.New("cleaner candidate not supported by input")

func shouldApplyBackgroundCleaner(current domain.DraftSession, candidate parsedDraft) (bool, string) {
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
	if currentHasDate && !candidateHasDate {
		return false, "candidate_drops_date"
	}
	if currentHasDate && candidateHasDate && !sameDraftDate(current.DraftExpiresOn, candidate.ExpiresOn) {
		return false, "candidate_changes_date"
	}
	if !currentHasName && candidateHasName {
		return true, "adds_name"
	}
	if !currentHasDate && candidateHasDate {
		return true, "adds_date"
	}
	if !currentHasName || !candidateHasName {
		return false, "no_improvement"
	}

	currentName := normalizeDraftName(current.DraftName)
	candidateName := normalizeDraftName(candidate.Name)
	if currentName == candidateName && sameDraftDate(current.DraftExpiresOn, candidate.ExpiresOn) {
		return false, "same_candidate"
	}
	if !cleanerCandidateMatchesCurrentEntity(currentName, candidateName) {
		return false, "unrelated_entity"
	}
	if candidateRemovesOnlyNoise(currentName, candidateName) {
		return true, "removes_noise"
	}

	currentNoise := nameNoiseScore(currentName)
	candidateNoise := nameNoiseScore(candidateName)
	if candidateNoise > currentNoise {
		return false, "adds_noise"
	}
	if candidateHasDisallowedExpansion(currentName, candidateName) {
		return false, "adds_noise"
	}
	if candidateNoise < currentNoise {
		return true, "cleaner_name"
	}
	if nameTokenCount(candidateName) < nameTokenCount(currentName) {
		return true, "shorter_name"
	}
	if strings.Contains(currentName, candidateName) && candidateName != "" {
		return true, "shorter_name"
	}
	return false, "name_not_better"
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

var cleanerNameNoiseTokens = map[string]struct{}{
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
		if !isCleanerNoiseToken(token) {
			return false
		}
	}
	return true
}

func cleanerCandidateSupportedByInput(name, cleanedInput string) bool {
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

func cleanerCandidateMatchesCurrentEntity(current, candidate string) bool {
	current = normalizeDraftName(current)
	candidate = normalizeDraftName(candidate)
	if current == "" || candidate == "" {
		return false
	}
	if current == candidate {
		return true
	}
	currentSet := cleanerNameTokenSet(current)
	for _, token := range strings.Fields(normalizedPolicyText(candidate)) {
		token = cleanerCanonicalNameToken(token)
		if token == "" {
			continue
		}
		if _, ok := currentSet[token]; ok {
			continue
		}
		return false
	}
	return true
}

func cleanerNameTokenSet(name string) map[string]struct{} {
	set := map[string]struct{}{}
	for _, token := range strings.Fields(normalizedPolicyText(name)) {
		token = cleanerCanonicalNameToken(token)
		if token == "" {
			continue
		}
		set[token] = struct{}{}
	}
	return set
}

func cleanerCanonicalNameToken(token string) string {
	token = normalizedPolicyText(token)
	if token == "" {
		return ""
	}
	if canonical, ok := productCanonicalLeadToken[token]; ok {
		return canonical
	}
	return token
}

func normalizeCleanerOutput(cleaner cleanerOutput, fallbackText string) string {
	cleaned := strings.TrimSpace(cleaner.CleanedInput)
	if cleaned == "" {
		cleaned = strings.TrimSpace(fallbackText)
	}
	return normalizeFreeText(cleaned)
}

func nameNoiseScore(name string) int {
	score := 0
	for _, token := range strings.Fields(normalizedPolicyText(name)) {
		if isCleanerNoiseToken(token) {
			score++
		}
	}
	return score
}

func nameTokenCount(name string) int {
	return len(strings.Fields(normalizedPolicyText(name)))
}

func isCleanerNoiseToken(token string) bool {
	token = normalizedPolicyText(token)
	if token == "" {
		return false
	}
	if _, ok := cleanerNameNoiseTokens[token]; ok {
		return true
	}
	if _, ok := packagingNoiseTokens[token]; ok {
		return true
	}
	if _, ok := deliveryNoiseTokens[token]; ok {
		return true
	}
	if _, ok := storeNoiseTokens[token]; ok {
		return true
	}
	if _, ok := quantityNoiseTokens[token]; ok {
		return true
	}
	return false
}

func candidateHasDisallowedExpansion(current, candidate string) bool {
	currentNoise := nameNoiseScore(current)
	candidateNoise := nameNoiseScore(candidate)
	if candidateNoise > currentNoise {
		return true
	}
	if nameTokenCount(candidate) > nameTokenCount(current) && !strings.Contains(current, candidate) {
		for _, token := range strings.Fields(normalizedPolicyText(candidate)) {
			if isCleanerNoiseToken(token) {
				return true
			}
		}
	}
	return false
}

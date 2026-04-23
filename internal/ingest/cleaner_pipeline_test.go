package ingest

import (
	"testing"
	"time"

	"github.com/igor/shelfy/internal/domain"
)

func TestShouldApplyBackgroundCleanerImprovesPartialDraft(t *testing.T) {
	current := domain.DraftSession{
		Status:         domain.DraftStatusReady,
		DraftName:      "молоко",
		DraftExpiresOn: nil,
	}
	date := time.Date(2026, time.April, 24, 0, 0, 0, 0, time.UTC)
	candidate := parsedDraft{
		Name:      "молоко",
		ExpiresOn: &date,
	}
	apply, reason := shouldApplyBackgroundCleaner(current, candidate)
	if !apply {
		t.Fatalf("expected apply, got reason %q", reason)
	}
	if reason != "adds_date" {
		t.Fatalf("reason = %q, want adds_date", reason)
	}
}

func TestShouldApplyBackgroundCleanerAppliesShorterEntity(t *testing.T) {
	date := time.Date(2026, time.April, 24, 0, 0, 0, 0, time.UTC)
	current := domain.DraftSession{
		Status:         domain.DraftStatusReady,
		DraftName:      "молоко простоквашино",
		DraftExpiresOn: &date,
	}
	candidate := parsedDraft{
		Name:      "молоко",
		ExpiresOn: &date,
	}
	apply, reason := shouldApplyBackgroundCleaner(current, candidate)
	if !apply {
		t.Fatalf("expected shorter entity to apply, got reason %q", reason)
	}
	if reason != "shorter_name" {
		t.Fatalf("reason = %q, want shorter_name", reason)
	}
}

func TestShouldApplyBackgroundCleanerRejectsNoisyExpansion(t *testing.T) {
	date := time.Date(2026, time.April, 24, 0, 0, 0, 0, time.UTC)
	current := domain.DraftSession{
		Status:         domain.DraftStatusReady,
		DraftName:      "чай ахмад ти инглиш брекфаст",
		DraftExpiresOn: &date,
	}
	candidate := parsedDraft{
		Name:      "чай ахмад инглиш брекфаст сто пакетиков",
		ExpiresOn: &date,
	}
	apply, reason := shouldApplyBackgroundCleaner(current, candidate)
	if apply {
		t.Fatal("expected noisy expansion to be rejected")
	}
	if reason != "adds_noise" && reason != "name_not_better" && reason != "unrelated_entity" {
		t.Fatalf("reason = %q, want adds_noise, name_not_better, or unrelated_entity", reason)
	}
}

func TestShouldApplyBackgroundCleanerRejectsUnrelatedShorterEntity(t *testing.T) {
	current := domain.DraftSession{
		Status:    domain.DraftStatusReady,
		DraftName: "черный уральский хлеб",
	}
	candidate := parsedDraft{
		Name: "монетки",
	}
	apply, reason := shouldApplyBackgroundCleaner(current, candidate)
	if apply {
		t.Fatal("expected unrelated entity to be rejected")
	}
	if reason != "unrelated_entity" {
		t.Fatalf("reason = %q, want unrelated_entity", reason)
	}
}

func TestShouldApplyBackgroundCleanerAppliesObviousNoiseCleanup(t *testing.T) {
	date := time.Date(2026, time.April, 24, 0, 0, 0, 0, time.UTC)
	current := domain.DraftSession{
		Status:         domain.DraftStatusReady,
		DraftName:      "по фарш",
		DraftExpiresOn: &date,
	}
	candidate := parsedDraft{
		Name:      "фарш",
		ExpiresOn: &date,
	}
	apply, reason := shouldApplyBackgroundCleaner(current, candidate)
	if !apply {
		t.Fatalf("expected noise cleanup to apply, got reason %q", reason)
	}
	if reason != "removes_noise" {
		t.Fatalf("reason = %q, want removes_noise", reason)
	}
}

func TestShouldApplyBackgroundCleanerRejectsSameCandidate(t *testing.T) {
	date := time.Date(2026, time.April, 24, 0, 0, 0, 0, time.UTC)
	current := domain.DraftSession{
		Status:         domain.DraftStatusReady,
		DraftName:      "шоколад",
		DraftExpiresOn: &date,
	}
	candidate := parsedDraft{
		Name:      "шоколад",
		ExpiresOn: &date,
	}
	apply, reason := shouldApplyBackgroundCleaner(current, candidate)
	if apply {
		t.Fatal("expected identical candidate to be rejected")
	}
	if reason != "same_candidate" {
		t.Fatalf("reason = %q, want same_candidate", reason)
	}
}

func TestParseCleanerCandidateUsesStrictDatePhrase(t *testing.T) {
	now := time.Date(2026, time.April, 20, 10, 0, 0, 0, time.UTC)
	candidate, cleaned, err := parseCleanerCandidate(now, cleanerOutput{
		CleanedInput: "молоко до 26 апреля",
	}, "молоко до 26 апреля")
	if err != nil {
		t.Fatalf("parseCleanerCandidate() error = %v", err)
	}
	if cleaned != "молоко до 26 апреля" {
		t.Fatalf("cleaned = %q, want %q", cleaned, "молоко до 26 апреля")
	}
	if candidate.Name != "молоко" {
		t.Fatalf("candidate name = %q, want молоко", candidate.Name)
	}
	if candidate.ExpiresOn == nil || candidate.ExpiresOn.Format("2006-01-02") != "2026-04-26" {
		t.Fatalf("candidate date = %#v, want 2026-04-26", candidate.ExpiresOn)
	}
}

func TestBuildDraftPayloadStoresCleanerMetadata(t *testing.T) {
	meta := buildDraftTracePayload(draftBuildTrace{
		NormalizedInput: "по фарш послезавтра",
		CleanerCalled:   true,
		CleanedInput:    "фарш послезавтра",
		SelectionReason: "removes_noise",
		ChosenSource:    "cleaner",
	})
	if got, _ := meta[domain.DraftPayloadKeyNormalizedInput].(string); got != "по фарш послезавтра" {
		t.Fatalf("normalized_input = %q, want %q", got, "по фарш послезавтра")
	}
	if got, _ := meta[domain.DraftPayloadKeyCleanerCalled].(bool); !got {
		t.Fatal("expected cleaner_called=true")
	}
	if got, _ := meta[domain.DraftPayloadKeyCleanedInput].(string); got != "фарш послезавтра" {
		t.Fatalf("cleaned_input = %q, want %q", got, "фарш послезавтра")
	}
	if got, _ := meta[domain.DraftPayloadKeySelectionReason].(string); got != "removes_noise" {
		t.Fatalf("selection_reason = %q, want removes_noise", got)
	}
	if got, _ := meta[domain.DraftPayloadKeyChosenSource].(string); got != "cleaner" {
		t.Fatalf("chosen_source = %q, want cleaner", got)
	}
}

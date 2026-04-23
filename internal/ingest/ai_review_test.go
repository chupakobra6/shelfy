package ingest

import (
	"errors"
	"testing"
	"time"

	"github.com/igor/shelfy/internal/domain"
)

func TestReviewApplyDecisionImprovesPartialDraft(t *testing.T) {
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
	apply, reason := reviewApplyDecision(current, candidate)
	if !apply {
		t.Fatalf("expected apply, got reason %q", reason)
	}
	if reason != "adds_missing_field" {
		t.Fatalf("reason = %q, want adds_missing_field", reason)
	}
}

func TestReviewApplyDecisionRejectsMoreGenericName(t *testing.T) {
	date := time.Date(2026, time.April, 24, 0, 0, 0, 0, time.UTC)
	current := domain.DraftSession{
		Status:         domain.DraftStatusReady,
		DraftName:      "йогурт питьевой",
		DraftExpiresOn: &date,
	}
	candidate := parsedDraft{
		Name:      "йогурт",
		ExpiresOn: &date,
	}
	apply, reason := reviewApplyDecision(current, candidate)
	if apply {
		t.Fatal("expected generic candidate to be rejected")
	}
	if reason != "name_not_better" {
		t.Fatalf("reason = %q, want name_not_better", reason)
	}
}

func TestReviewApplyDecisionRejectsCanonicalButShorterBrandName(t *testing.T) {
	date := time.Date(2026, time.April, 24, 0, 0, 0, 0, time.UTC)
	current := domain.DraftSession{
		Status:         domain.DraftStatusReady,
		DraftName:      "масло подсолнечное золотая семечка",
		DraftExpiresOn: &date,
	}
	candidate := parsedDraft{
		Name:      "масло",
		ExpiresOn: &date,
	}
	apply, reason := reviewApplyDecision(current, candidate)
	if apply {
		t.Fatal("expected shortened canonical candidate to be rejected")
	}
	if reason != "name_not_better" {
		t.Fatalf("reason = %q, want name_not_better", reason)
	}
}

func TestReviewApplyDecisionAppliesObviousNoiseCleanup(t *testing.T) {
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
	apply, reason := reviewApplyDecision(current, candidate)
	if !apply {
		t.Fatalf("expected noise cleanup to apply, got reason %q", reason)
	}
	if reason != "removes_noise" {
		t.Fatalf("reason = %q, want removes_noise", reason)
	}
}

func TestReviewApplyDecisionRejectsSameCandidate(t *testing.T) {
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
	apply, reason := reviewApplyDecision(current, candidate)
	if apply {
		t.Fatal("expected identical candidate to be rejected")
	}
	if reason != "same_candidate" {
		t.Fatalf("reason = %q, want same_candidate", reason)
	}
}

func TestBuildCleanerCandidateUsesStrictDatePhrase(t *testing.T) {
	now := time.Date(2026, time.April, 20, 10, 0, 0, 0, time.UTC)
	candidate, cleaned, err := buildCleanerCandidate(now, reviewCleaner{
		CleanedInput: "молоко до 26 апреля",
	}, "молоко до 26 апреля")
	if err != nil {
		t.Fatalf("buildCleanerCandidate() error = %v", err)
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

func TestShouldAttemptReviewRescueForIntentGateFailure(t *testing.T) {
	if !shouldAttemptReviewRescue(domain.MessageKindVoice, "фарш до второго числа", errors.New("text rejected by intent gate: non_food")) {
		t.Fatal("expected review rescue for intent-gate failure")
	}
	if shouldAttemptReviewRescue(domain.MessageKindVoice, "", errors.New("text rejected by intent gate: non_food")) {
		t.Fatal("expected empty input to skip review rescue")
	}
}

func TestFinalizeReviewRescueDraftRejectsUnsupportedName(t *testing.T) {
	draft, ok := finalizeReviewRescueDraft("молоко до завтра", parsedDraft{Name: "кефир"})
	if ok || draft.Name != "" {
		t.Fatal("expected unsupported rescue candidate to be rejected")
	}
}

func TestReviewMetadataDropsLegacyKeys(t *testing.T) {
	meta := reviewMetadata(map[string]any{
		"review_mode":             "name_cleanup",
		"review_routing_reason":   "legacy",
		"review_cleaner_state":    "ready",
		"review_cleaner_name":     "молоко",
		"review_cleaner_date":     "до завтра",
		"review_cleaner_reason":   "legacy_cleanup",
		"review_judge_confidence": "high",
		"review_judge_decision":   "apply",
		"review_judge_reason":     "legacy_judge",
	}, reviewCleaner{
		CleanedInput: "молоко до завтра",
		ReasonCode:   "filler_cleanup",
	}, "молоко до завтра", true, "adds_missing_field")
	for _, key := range []string{
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
		if _, ok := meta[key]; ok {
			t.Fatalf("expected legacy key %q to be removed", key)
		}
	}
	if got, _ := meta[domain.DraftPayloadKeyReviewReasonCode].(string); got != "filler_cleanup" {
		t.Fatalf("review reason code = %q, want filler_cleanup", got)
	}
	if got, _ := meta[domain.DraftPayloadKeyReviewApplyReason].(string); got != "adds_missing_field" {
		t.Fatalf("review apply reason = %q, want adds_missing_field", got)
	}
}

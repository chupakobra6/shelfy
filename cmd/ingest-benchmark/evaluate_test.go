package main

import (
	"testing"

	"github.com/igor/shelfy/internal/ingest"
)

func TestAssessProductOutcomeTreatsIntentRejectAsSuccess(t *testing.T) {
	t.Parallel()

	exact, failureKind, wantSummary, gotSummary, errText := assessProductOutcome(
		"text",
		"reject",
		"",
		"",
		ingest.EvalResult{Name: "фэри"},
		nil,
	)
	if !exact {
		t.Fatalf("expected reject outcome to be exact, got failure kind %q error %q", failureKind, errText)
	}
	if failureKind != "" {
		t.Fatalf("expected empty failure kind, got %q", failureKind)
	}
	if wantSummary != "state=reject" || gotSummary != "state=needs_expiry, name=фэри" {
		t.Fatalf("unexpected summaries: want=%q got=%q", wantSummary, gotSummary)
	}
	if errText != "" {
		t.Fatalf("expected empty error text, got %q", errText)
	}
}

func TestProductNamesEquivalentAllowsSimpleRussianCaseShift(t *testing.T) {
	t.Parallel()

	if !productNamesEquivalent("курицу", "курица") {
		t.Fatalf("expected курицу and курица to match")
	}
	if productNamesEquivalent("кефир", "молоко") {
		t.Fatalf("expected different product names to differ")
	}
}

func TestProductNameAcceptableAllowsShorterCoreEntity(t *testing.T) {
	t.Parallel()

	if !productNameAcceptable("молоко домик в деревне", "молоко") {
		t.Fatalf("expected shorter core entity to be acceptable")
	}
	if !productNameAcceptable("мороженое чистая линия ванильное", "ванильное мороженое") {
		t.Fatalf("expected reordered subset entity to be acceptable")
	}
	if productNameAcceptable("черный уральский хлеб", "монетки") {
		t.Fatalf("expected unrelated entity to be rejected")
	}
}

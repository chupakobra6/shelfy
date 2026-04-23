package main

import (
	"fmt"
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
		ingest.EvalResult{},
		fmt.Errorf("text rejected by intent gate: non_food"),
	)
	if !exact {
		t.Fatalf("expected reject outcome to be exact, got failure kind %q error %q", failureKind, errText)
	}
	if failureKind != "" {
		t.Fatalf("expected empty failure kind, got %q", failureKind)
	}
	if wantSummary != "state=reject" || gotSummary != "state=reject" {
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

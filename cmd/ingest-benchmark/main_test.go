package main

import (
	"testing"
)

func TestValidateCorpusPublicAssets(t *testing.T) {
	suite, err := loadCorpus("../../internal/ingest/testdata/benchmark_corpus.json")
	if err != nil {
		t.Fatalf("loadCorpus error = %v", err)
	}
	if err := validateCorpus(suite); err != nil {
		t.Fatalf("validateCorpus error = %v", err)
	}
	if got := len(suite.Text); got != 100 {
		t.Fatalf("text cases = %d, want 100", got)
	}
	if got := len(suite.Voice); got != 100 {
		t.Fatalf("voice cases = %d, want 100", got)
	}
	if got := len(suite.ReviewHard.TextIDs); got == 0 {
		t.Fatal("review_hard.text_ids should not be empty")
	}
	if got := len(suite.ReviewHard.VoiceIDs); got == 0 {
		t.Fatal("review_hard.voice_ids should not be empty")
	}
	for _, voice := range suite.Voice {
		if voice.SourceID == "" {
			t.Fatalf("voice case %s is missing source_id", voice.ID)
		}
		if !hasVoiceAssetLocator(voice) {
			t.Fatalf("voice case %s is missing supported asset locator", voice.ID)
		}
		if voice.Difficulty == "" {
			t.Fatalf("voice case %s is missing difficulty", voice.ID)
		}
	}
	for _, tc := range suite.Text {
		if tc.Difficulty == "" {
			t.Fatalf("text case %s is missing difficulty", tc.ID)
		}
	}
}

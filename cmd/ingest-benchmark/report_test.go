package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWriteReportCopiesArtifacts(t *testing.T) {
	root := t.TempDir()
	audioPath := filepath.Join(root, "sample.wav")
	if err := os.WriteFile(audioPath, []byte("artifact"), 0o644); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}

	cfg := runConfig{
		corpusPath:    "internal/ingest/testdata/benchmark_corpus.json",
		ollamaModel:   "gemma3:4b",
		reportDir:     filepath.Join(root, "report"),
		copyArtifacts: true,
		include:       map[string]bool{"voice": true},
		limit:         1,
		caseFilter:    "voice-case",
	}
	summaries := []variantSummary{
		{
			Family:       "voice",
			Variant:      "voice_vosk_plain",
			Total:        1,
			Exact:        1,
			FailureKinds: map[string]int{"asset_error": 0},
			Cases: []caseResult{
				{
					ID:             "voice-case",
					Family:         "voice",
					Variant:        "voice_vosk_plain",
					Exact:          true,
					AudioPath:      audioPath,
					DurationMillis: 10,
				},
			},
		},
	}

	if err := writeReport(cfg, time.Date(2026, 4, 20, 12, 0, 0, 0, time.FixedZone("MSK", 3*3600)), summaries); err != nil {
		t.Fatalf("writeReport error = %v", err)
	}

	indexPath := filepath.Join(cfg.reportDir, "index.html")
	content, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatalf("ReadFile error = %v", err)
	}
	if !strings.Contains(string(content), "Shelfy Ingest Benchmark") {
		t.Fatalf("index.html missing benchmark title")
	}
	if !strings.Contains(string(content), "artifacts/") {
		t.Fatalf("index.html missing copied artifact path")
	}
	if !strings.Contains(string(content), "Run mode") {
		t.Fatalf("index.html missing run metadata")
	}
	if !strings.Contains(string(content), "Scenario Matrix") {
		t.Fatalf("index.html missing scenario matrix")
	}
	if !strings.Contains(string(content), "Tag Matrix") {
		t.Fatalf("index.html missing tag matrix")
	}
	if !strings.Contains(string(content), "Difficulty Matrix") {
		t.Fatalf("index.html missing difficulty matrix")
	}
	if !strings.Contains(string(content), "Audit Queue") {
		t.Fatalf("index.html missing audit queue")
	}
	if _, err := os.Stat(filepath.Join(cfg.reportDir, "audit.md")); err != nil {
		t.Fatalf("audit.md missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cfg.reportDir, "audit.json")); err != nil {
		t.Fatalf("audit.json missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cfg.reportDir, "audit-refined.md")); err != nil {
		t.Fatalf("audit-refined.md missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cfg.reportDir, "audit-refined.json")); err != nil {
		t.Fatalf("audit-refined.json missing: %v", err)
	}
}

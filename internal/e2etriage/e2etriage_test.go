package e2etriage

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMatchScenarioLabel(t *testing.T) {
	row := ArtifactRow{
		ScenarioPath:    "/tmp/02-dashboard-navigation-and-settings.jsonl",
		TranscriptLabel: "02-dashboard-navigation-and-settings",
	}

	cases := []string{
		"02-dashboard-navigation-and-settings",
		"02-dashboard-navigation-and-settings.jsonl",
		"/tmp/02-dashboard-navigation-and-settings.jsonl",
	}
	for _, value := range cases {
		if !MatchScenarioLabel(value, row) {
			t.Fatalf("expected %q to match %+v", value, row)
		}
	}
}

func TestResolveWindowFromArtifactsPrefersFailureReport(t *testing.T) {
	root := t.TempDir()
	transcriptsDir := filepath.Join(root, "artifacts", "transcripts")
	if err := os.MkdirAll(transcriptsDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	failureAt := time.Date(2026, 4, 22, 10, 11, 12, 0, time.UTC)
	failureJSON := `{
		"scenario_path":"/tmp/15-dashboard-pagination.jsonl",
		"transcript_label":"15-dashboard-pagination",
		"failure_at":"` + failureAt.Format(time.RFC3339) + `"
	}`
	if err := os.WriteFile(filepath.Join(transcriptsDir, "last-failure.json"), []byte(failureJSON), 0o644); err != nil {
		t.Fatalf("WriteFile(last-failure.json) error = %v", err)
	}

	since, until, err := ResolveWindowFromArtifacts(root, "15-dashboard-pagination", time.Now().UTC())
	if err != nil {
		t.Fatalf("ResolveWindowFromArtifacts() error = %v", err)
	}
	if want := failureAt.Add(-WindowPadding); !since.Equal(want) {
		t.Fatalf("since = %s, want %s", since, want)
	}
	if want := failureAt.Add(WindowPadding); !until.Equal(want) {
		t.Fatalf("until = %s, want %s", until, want)
	}
}

func TestResolveWindowFromArtifactsFallsBackToSummary(t *testing.T) {
	root := t.TempDir()
	transcriptsDir := filepath.Join(root, "artifacts", "transcripts")
	if err := os.MkdirAll(transcriptsDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	finishedAt := time.Date(2026, 4, 22, 10, 11, 12, 0, time.UTC)
	summaryJSON := `[
		{
			"scenario_path":"/tmp/05-voice-date-phrases.jsonl",
			"transcript_label":"05-voice-date-phrases",
			"finished_at":"` + finishedAt.Format(time.RFC3339) + `"
		}
	]`
	if err := os.WriteFile(filepath.Join(transcriptsDir, "last-run-summary.json"), []byte(summaryJSON), 0o644); err != nil {
		t.Fatalf("WriteFile(last-run-summary.json) error = %v", err)
	}

	since, until, err := ResolveWindowFromArtifacts(root, "05-voice-date-phrases", time.Now().UTC())
	if err != nil {
		t.Fatalf("ResolveWindowFromArtifacts() error = %v", err)
	}
	if want := finishedAt.Add(-WindowPadding); !since.Equal(want) {
		t.Fatalf("since = %s, want %s", since, want)
	}
	if want := finishedAt.Add(WindowPadding); !until.Equal(want) {
		t.Fatalf("until = %s, want %s", until, want)
	}
}

func TestNormalizeLogLine(t *testing.T) {
	raw := `{"time":"2026-04-22T12:00:00Z","level":"info","msg":"digest created","trace_id":"abc","job_id":"123","payload":"ignored"}`
	got, ok := NormalizeLogLine(raw, Filters{})
	if !ok {
		t.Fatalf("expected JSON log to be included")
	}
	if strings.Contains(got, "payload=") {
		t.Fatalf("unexpected payload leak: %q", got)
	}
	if !strings.Contains(got, "trace_id=abc") || !strings.Contains(got, "job_id=123") {
		t.Fatalf("missing compacted fields: %q", got)
	}
}

func TestNormalizeLogLineFiltersJSONRecords(t *testing.T) {
	raw := `{"time":"2026-04-22T12:00:00Z","level":"info","msg":"digest created","trace_id":"abc","update_id":"999"}`
	if _, ok := NormalizeLogLine(raw, Filters{TraceID: "nope"}); ok {
		t.Fatalf("expected unmatched record to be filtered out")
	}
	if got, ok := NormalizeLogLine(raw, Filters{UpdateID: "999"}); !ok || !strings.Contains(got, "update_id=999") {
		t.Fatalf("expected matching record, got %q ok=%t", got, ok)
	}
}

func TestNormalizeLogLineCompactsPlainLinesWithoutFilters(t *testing.T) {
	got, ok := NormalizeLogLine(" service | plain    text   line ", Filters{})
	if !ok {
		t.Fatalf("expected plain line to be included without filters")
	}
	if got != "plain text line" {
		t.Fatalf("got %q", got)
	}
}

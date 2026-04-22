package e2etriage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const WindowPadding = 45 * time.Second

type ArtifactRow struct {
	ScenarioPath    string    `json:"scenario_path"`
	TranscriptLabel string    `json:"transcript_label"`
	FailureAt       time.Time `json:"failure_at,omitempty"`
	FinishedAt      time.Time `json:"finished_at,omitempty"`
}

type FailureReport struct {
	ArtifactRow
}

type Filters struct {
	TraceID  string
	UpdateID string
	JobID    string
}

var (
	ansiRE       = regexp.MustCompile(`\x1b\[[0-9;]*m`)
	whitespaceRE = regexp.MustCompile(`\s+`)
)

func NormalizeLabel(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimSuffix(value, ".jsonl")
	return value
}

func MatchScenarioLabel(label string, row ArtifactRow) bool {
	inputs := []string{
		NormalizeLabel(label),
		NormalizeLabel(filepath.Base(label)),
		NormalizeLabel(strings.TrimSuffix(filepath.Base(label), filepath.Ext(label))),
	}
	candidates := []string{
		NormalizeLabel(row.TranscriptLabel),
		NormalizeLabel(strings.TrimSuffix(filepath.Base(row.ScenarioPath), filepath.Ext(row.ScenarioPath))),
		NormalizeLabel(filepath.Base(row.ScenarioPath)),
	}
	for _, input := range inputs {
		if input == "" {
			continue
		}
		for _, candidate := range candidates {
			if candidate == input {
				return true
			}
		}
	}
	return false
}

func ResolveWindowFromArtifacts(toolRoot, scenarioLabel string, now time.Time) (time.Time, time.Time, error) {
	failurePath := filepath.Join(toolRoot, "artifacts", "transcripts", "last-failure.json")
	if failure, err := readFailureReport(failurePath); err == nil {
		if MatchScenarioLabel(scenarioLabel, failure.ArtifactRow) {
			if anchor := firstNonZero(failure.FailureAt, failure.FinishedAt); !anchor.IsZero() {
				return anchor.Add(-WindowPadding), anchor.Add(WindowPadding), nil
			}
			return time.Time{}, time.Time{}, fmt.Errorf("scenario metadata has no failure_at or finished_at timestamp")
		}
	}

	summaryPath := filepath.Join(toolRoot, "artifacts", "transcripts", "last-run-summary.json")
	rows, err := readSummaryRows(summaryPath)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("scenario label not found in tool artifacts: %s", scenarioLabel)
	}
	for _, row := range rows {
		if MatchScenarioLabel(scenarioLabel, row) {
			if anchor := firstNonZero(row.FailureAt, row.FinishedAt); !anchor.IsZero() {
				return anchor.Add(-WindowPadding), anchor.Add(WindowPadding), nil
			}
			return time.Time{}, time.Time{}, fmt.Errorf("scenario metadata has no failure_at or finished_at timestamp")
		}
	}
	return time.Time{}, time.Time{}, fmt.Errorf("scenario label not found in tool artifacts: %s", scenarioLabel)
}

func SafeLabel(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "failure"
	}
	var b strings.Builder
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || strings.ContainsRune("-._", r) {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	if b.Len() == 0 {
		return "failure"
	}
	return b.String()
}

func NormalizeLogLine(raw string, filters Filters) (string, bool) {
	line := strings.TrimSpace(stripANSI(raw))
	if line == "" {
		return "", false
	}
	if idx := strings.Index(line, "|"); idx >= 0 {
		line = strings.TrimSpace(line[idx+1:])
	}
	if line == "" {
		return "", false
	}

	var record map[string]any
	if err := json.Unmarshal([]byte(line), &record); err != nil {
		if filters.Active() {
			return "", false
		}
		return compact(line, 200), true
	}

	if filters.Active() && !filters.MatchRecord(record) {
		return "", false
	}

	keys := []string{"time", "level", "msg", "trace_id", "update_id", "job_id", "error"}
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		value, ok := record[key]
		if !ok || value == nil {
			continue
		}
		text := strings.TrimSpace(fmt.Sprint(value))
		if text == "" {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s=%s", key, compact(text, 120)))
	}
	if len(parts) == 0 {
		return "", false
	}
	return strings.Join(parts, " "), true
}

func (f Filters) Active() bool {
	return strings.TrimSpace(f.TraceID) != "" || strings.TrimSpace(f.UpdateID) != "" || strings.TrimSpace(f.JobID) != ""
}

func (f Filters) MatchRecord(record map[string]any) bool {
	expectations := map[string]string{
		"trace_id":  strings.TrimSpace(f.TraceID),
		"update_id": strings.TrimSpace(f.UpdateID),
		"job_id":    strings.TrimSpace(f.JobID),
	}
	for key, expected := range expectations {
		if expected == "" {
			continue
		}
		if value, ok := record[key]; ok && value != nil && fmt.Sprint(value) == expected {
			return true
		}
	}
	return false
}

func readFailureReport(path string) (FailureReport, error) {
	var report FailureReport
	body, err := os.ReadFile(path)
	if err != nil {
		return report, err
	}
	if err := json.Unmarshal(body, &report); err != nil {
		return report, err
	}
	return report, nil
}

func readSummaryRows(path string) ([]ArtifactRow, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var rows []ArtifactRow
	if err := json.Unmarshal(body, &rows); err != nil {
		return nil, err
	}
	return rows, nil
}

func compact(value string, limit int) string {
	value = stripANSI(value)
	value = whitespaceRE.ReplaceAllString(value, " ")
	value = strings.TrimSpace(value)
	if limit <= 0 || len(value) <= limit {
		return value
	}
	if limit == 1 {
		return "…"
	}
	return value[:limit-1] + "…"
}

func stripANSI(value string) string {
	return ansiRE.ReplaceAllString(value, "")
}

func firstNonZero(values ...time.Time) time.Time {
	for _, value := range values {
		if !value.IsZero() {
			return value
		}
	}
	return time.Time{}
}

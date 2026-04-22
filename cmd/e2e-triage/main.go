package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/igor/shelfy/internal/e2etriage"
)

const defaultMaxLines = 200

func main() {
	if len(os.Args) < 2 {
		printUsage(os.Stderr)
		os.Exit(2)
	}

	repoRoot := detectRepoRoot()
	switch os.Args[1] {
	case "trace-logs":
		if err := runTraceLogs(repoRoot, os.Args[2:], os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "last-failure-pack":
		if err := runLastFailurePack(repoRoot, os.Args[2:], os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "help", "--help", "-h":
		printUsage(os.Stdout)
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", os.Args[1])
		printUsage(os.Stderr)
		os.Exit(2)
	}
}

func printUsage(out io.Writer) {
	fmt.Fprintln(out, "usage: e2e-triage <trace-logs|last-failure-pack> [flags]")
}

func runTraceLogs(repoRoot string, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("trace-logs", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	traceID := fs.String("trace-id", "", "filter by trace_id")
	updateID := fs.String("update-id", "", "filter by update_id")
	jobID := fs.String("job-id", "", "filter by job_id")
	scenarioLabel := fs.String("scenario-label", "", "derive time window from tool artifacts")
	sinceText := fs.String("since", "", "start of log window (RFC3339)")
	untilText := fs.String("until", "", "end of log window (RFC3339)")
	service := fs.String("service", "", "limit output to one service")
	toolRoot := fs.String("tool-root", defaultToolRoot(repoRoot), "path to telegram-bot-e2e-test-tool")
	maxLines := fs.Int("max-lines", defaultEnvInt("MAX_LINES", defaultMaxLines), "maximum normalized lines per service")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := ensureCommand("docker"); err != nil {
		return err
	}
	if _, err := os.Stat(*toolRoot); err != nil {
		return fmt.Errorf("tool root not found: %s", *toolRoot)
	}

	now := time.Now().UTC()
	since, until, err := resolveWindow(*toolRoot, strings.TrimSpace(*scenarioLabel), strings.TrimSpace(*sinceText), strings.TrimSpace(*untilText), now)
	if err != nil {
		return err
	}

	services := []string{"telegram-api", "pipeline-worker", "scheduler-worker"}
	if trimmed := strings.TrimSpace(*service); trimmed != "" {
		services = []string{trimmed}
	}

	filters := e2etriage.Filters{
		TraceID:  strings.TrimSpace(*traceID),
		UpdateID: strings.TrimSpace(*updateID),
		JobID:    strings.TrimSpace(*jobID),
	}

	for index, svc := range services {
		if strings.TrimSpace(*service) == "" {
			if index > 0 {
				fmt.Fprintln(out)
			}
			fmt.Fprintf(out, "== %s ==\n", svc)
		}
		lines, err := collectServiceLogs(repoRoot, svc, since, until, filters, *maxLines)
		if err != nil {
			return err
		}
		for _, line := range lines {
			fmt.Fprintln(out, line)
		}
	}
	return nil
}

func runLastFailurePack(repoRoot string, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("last-failure-pack", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	toolRoot := fs.String("tool-root", defaultToolRoot(repoRoot), "path to telegram-bot-e2e-test-tool")
	packRoot := fs.String("pack-root", filepath.Join(repoRoot, "tmp", "e2e-failure-pack"), "directory for generated failure packs")
	maxLinesPerService := fs.Int("max-lines-per-service", defaultEnvInt("MAX_LINES_PER_SERVICE", 133), "maximum normalized lines per service")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := ensureCommand("docker"); err != nil {
		return err
	}
	if _, err := os.Stat(*toolRoot); err != nil {
		return fmt.Errorf("tool root not found: %s", *toolRoot)
	}

	failureJSON := filepath.Join(*toolRoot, "artifacts", "transcripts", "last-failure.json")
	failureTXT := filepath.Join(*toolRoot, "artifacts", "transcripts", "last-failure.txt")
	report, err := readFailureReport(failureJSON)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("last failure artifact not found: %s", failureJSON)
		}
		return err
	}

	label := strings.TrimSpace(report.TranscriptLabel)
	if label == "" {
		label = strings.TrimSpace(report.ScenarioPath)
	}
	packDir := filepath.Join(*packRoot, fmt.Sprintf("%s-%s", time.Now().UTC().Format("20060102T150405Z"), e2etriage.SafeLabel(label)))
	if err := os.MkdirAll(packDir, 0o755); err != nil {
		return err
	}

	if err := copyFile(filepath.Join(packDir, "tool-last-failure.json"), failureJSON); err != nil {
		return err
	}
	if fileExists(failureTXT) {
		if err := copyFile(filepath.Join(packDir, "tool-last-failure.txt"), failureTXT); err != nil {
			return err
		}
	}

	svcFiles := map[string]string{}
	for _, svc := range []string{"telegram-api", "pipeline-worker", "scheduler-worker"} {
		lines, err := collectServiceLogsForScenario(repoRoot, *toolRoot, label, svc, *maxLinesPerService)
		if err != nil {
			return err
		}
		target := filepath.Join(packDir, fmt.Sprintf("trace-logs.%s.txt", svc))
		if err := os.WriteFile(target, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
			return err
		}
		svcFiles[svc] = target
	}

	manifest := map[string]any{
		"generated_at":     time.Now().UTC().Format(time.RFC3339),
		"failure_at":       formatOptionalTime(report.FailureAt),
		"transcript_label": report.TranscriptLabel,
		"scenario_path":    report.ScenarioPath,
		"files": map[string]string{
			"tool_last_failure_json": filepath.Join(packDir, "tool-last-failure.json"),
			"tool_last_failure_txt":  filepath.Join(packDir, "tool-last-failure.txt"),
			"telegram_api_logs":      svcFiles["telegram-api"],
			"pipeline_worker_logs":   svcFiles["pipeline-worker"],
			"scheduler_worker_logs":  svcFiles["scheduler-worker"],
		},
	}
	body, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(packDir, "manifest.json"), body, 0o644); err != nil {
		return err
	}

	fmt.Fprintln(out, packDir)
	return nil
}

func resolveWindow(toolRoot, scenarioLabel, sinceText, untilText string, now time.Time) (time.Time, time.Time, error) {
	var (
		since time.Time
		until time.Time
		err   error
	)
	if strings.TrimSpace(sinceText) != "" {
		since, err = time.Parse(time.RFC3339, sinceText)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("parse --since: %w", err)
		}
	}
	if strings.TrimSpace(untilText) != "" {
		until, err = time.Parse(time.RFC3339, untilText)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("parse --until: %w", err)
		}
	}
	if scenarioLabel != "" && (since.IsZero() || until.IsZero()) {
		resolvedSince, resolvedUntil, err := e2etriage.ResolveWindowFromArtifacts(toolRoot, scenarioLabel, now)
		if err != nil {
			return time.Time{}, time.Time{}, err
		}
		if since.IsZero() {
			since = resolvedSince
		}
		if until.IsZero() {
			until = resolvedUntil
		}
	}
	if since.IsZero() {
		since = now.Add(-e2etriage.WindowPadding)
	}
	if until.IsZero() {
		until = now.Add(e2etriage.WindowPadding)
	}
	return since.UTC(), until.UTC(), nil
}

func collectServiceLogsForScenario(repoRoot, toolRoot, label, service string, maxLines int) ([]string, error) {
	since, until, err := e2etriage.ResolveWindowFromArtifacts(toolRoot, label, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	return collectServiceLogs(repoRoot, service, since, until, e2etriage.Filters{}, maxLines)
}

func collectServiceLogs(repoRoot, service string, since, until time.Time, filters e2etriage.Filters, maxLines int) ([]string, error) {
	cmd := exec.Command(
		"docker", "compose", "logs", "--no-color",
		"--since", since.Format(time.RFC3339),
		"--until", until.Format(time.RFC3339),
		service,
	)
	cmd.Dir = repoRoot
	stderr := &strings.Builder{}
	cmd.Stderr = stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	lines := make([]string, 0, maxLines)
	buf := make([]byte, 0, 64*1024)
	reader := io.Reader(stdout)
	scanner := newLineScanner(reader, buf)
	for scanner.Scan() {
		normalized, ok := e2etriage.NormalizeLogLine(scanner.Text(), filters)
		if !ok {
			continue
		}
		if maxLines <= 0 || len(lines) < maxLines {
			lines = append(lines, normalized)
		}
	}
	if err := scanner.Err(); err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return nil, err
	}
	if err := cmd.Wait(); err != nil {
		if strings.TrimSpace(stderr.String()) != "" {
			return nil, fmt.Errorf("docker compose logs %s: %w: %s", service, err, strings.TrimSpace(stderr.String()))
		}
		return nil, fmt.Errorf("docker compose logs %s: %w", service, err)
	}
	return lines, nil
}

func newLineScanner(reader io.Reader, buf []byte) *bufio.Scanner {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(buf, 1024*1024)
	return scanner
}

func readFailureReport(path string) (e2etriage.FailureReport, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return e2etriage.FailureReport{}, err
	}
	var report e2etriage.FailureReport
	if err := json.Unmarshal(body, &report); err != nil {
		return e2etriage.FailureReport{}, err
	}
	return report, nil
}

func copyFile(dst, src string) error {
	body, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, body, 0o644)
}

func ensureCommand(name string) error {
	if _, err := exec.LookPath(name); err != nil {
		return fmt.Errorf("required command not found: %s", name)
	}
	return nil
}

func defaultToolRoot(repoRoot string) string {
	if value := strings.TrimSpace(os.Getenv("TOOL_ROOT")); value != "" {
		return value
	}
	return filepath.Join(repoRoot, "..", "telegram-bot-e2e-test-tool")
}

func defaultEnvInt(name string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	var parsed int
	if _, err := fmt.Sscanf(value, "%d", &parsed); err != nil {
		return fallback
	}
	return parsed
}

func detectRepoRoot() string {
	if _, file, _, ok := runtime.Caller(0); ok {
		root := filepath.Clean(filepath.Join(filepath.Dir(file), "../.."))
		if fileExists(filepath.Join(root, "go.mod")) {
			return root
		}
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return cwd
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func formatOptionalTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}

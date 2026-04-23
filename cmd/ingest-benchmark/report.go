package main

import (
	"encoding/json"
	"html/template"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type reportPayload struct {
	GeneratedAt   string           `json:"generated_at"`
	ReferenceTime string           `json:"reference_time"`
	Model         string           `json:"model"`
	Corpus        string           `json:"corpus"`
	RunMode       string           `json:"run_mode"`
	Include       []string         `json:"include"`
	Limit         int              `json:"limit,omitempty"`
	CaseFilter    string           `json:"case_filter,omitempty"`
	TagFilter     []string         `json:"tag_filter,omitempty"`
	Summaries     []variantSummary `json:"summaries"`
}

type reportSummary struct {
	Family               string         `json:"family"`
	Variant              string         `json:"variant"`
	Total                int            `json:"total"`
	Exact                int            `json:"exact"`
	FirstExact           int            `json:"first_exact,omitempty"`
	Failed               int            `json:"failed"`
	TextCalls            int            `json:"text_calls"`
	DurationMs           int64          `json:"duration_ms"`
	Timeouts             int            `json:"timeouts"`
	AssetErrors          int            `json:"asset_errors"`
	CleanerEligible      int            `json:"cleaner_eligible,omitempty"`
	CleanerCalled        int            `json:"cleaner_called,omitempty"`
	CleanerChangedInput  int            `json:"cleaner_changed_input,omitempty"`
	CandidateValid       int            `json:"candidate_valid,omitempty"`
	CleanerApplied       int            `json:"cleaner_applied,omitempty"`
	CleanerHelped        int            `json:"cleaner_helped,omitempty"`
	CleanerHurt          int            `json:"cleaner_hurt,omitempty"`
	CleanerNoop          int            `json:"cleaner_noop,omitempty"`
	CleanerSameCandidate int            `json:"cleaner_same_candidate,omitempty"`
	FailureKinds         map[string]int `json:"failure_kinds,omitempty"`
}

type auditCase struct {
	ID           string   `json:"id"`
	Family       string   `json:"family"`
	Variant      string   `json:"variant"`
	Difficulty   string   `json:"difficulty,omitempty"`
	FailureKind  string   `json:"failure_kind,omitempty"`
	WantSummary  string   `json:"want_summary,omitempty"`
	FirstSummary string   `json:"first_summary,omitempty"`
	GotSummary   string   `json:"got_summary,omitempty"`
	Tags         []string `json:"tags,omitempty"`
	Note         string   `json:"note,omitempty"`
}

type auditPayload struct {
	GeneratedAt string      `json:"generated_at"`
	FailCases   []auditCase `json:"fail_cases"`
	Suspicious  []auditCase `json:"suspicious_cases"`
}

func writeReport(cfg runConfig, referenceTime time.Time, summaries []variantSummary) error {
	if err := os.MkdirAll(cfg.reportDir, 0o755); err != nil {
		return err
	}
	payload := reportPayload{
		GeneratedAt:   time.Now().Format(time.RFC3339),
		ReferenceTime: referenceTime.Format(time.RFC3339),
		Model:         cfg.ollamaModel,
		Corpus:        cfg.corpusPath,
		RunMode:       benchmarkRunMode(cfg),
		Include:       sortedFilterKeys(cfg.include),
		Limit:         cfg.limit,
		CaseFilter:    cfg.caseFilter,
		TagFilter:     sortedFilterKeys(cfg.tagFilter),
		Summaries:     cloneSummaries(summaries),
	}
	if err := materializeArtifacts(cfg, &payload); err != nil {
		return err
	}
	resultsJSON, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(cfg.reportDir, "results.json"), resultsJSON, 0o644); err != nil {
		return err
	}
	flatSummary := make([]reportSummary, 0, len(payload.Summaries))
	for _, summary := range payload.Summaries {
		flatSummary = append(flatSummary, reportSummary{
			Family:               summary.Family,
			Variant:              summary.Variant,
			Total:                summary.Total,
			Exact:                summary.Exact,
			FirstExact:           summary.FirstExact,
			Failed:               summary.Failed,
			TextCalls:            summary.TextCalls,
			DurationMs:           summary.Duration.Milliseconds(),
			Timeouts:             summary.Timeouts,
			AssetErrors:          summary.AssetErrors,
			CleanerEligible:      summary.CleanerEligible,
			CleanerCalled:        summary.CleanerCalled,
			CleanerChangedInput:  summary.CleanerChangedInput,
			CandidateValid:       summary.CandidateValid,
			CleanerApplied:       summary.CleanerApplied,
			CleanerHelped:        summary.CleanerHelped,
			CleanerHurt:          summary.CleanerHurt,
			CleanerNoop:          summary.CleanerNoop,
			CleanerSameCandidate: summary.CleanerSameCandidate,
			FailureKinds:         summary.FailureKinds,
		})
	}
	summaryJSON, err := json.MarshalIndent(flatSummary, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(cfg.reportDir, "summary.json"), summaryJSON, 0o644); err != nil {
		return err
	}
	if err := writeAuditFiles(cfg.reportDir, payload); err != nil {
		return err
	}
	return writeHTMLReport(filepath.Join(cfg.reportDir, "index.html"), payload)
}

func writeAuditFiles(reportDir string, payload reportPayload) error {
	audit := buildAuditPayload(payload)
	data, err := json.MarshalIndent(audit, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(reportDir, "audit.json"), data, 0o644); err != nil {
		return err
	}
	var md strings.Builder
	md.WriteString("# Benchmark Audit\n\n")
	md.WriteString("Generated: " + audit.GeneratedAt + "\n\n")
	md.WriteString("## Fail Cases\n")
	for _, item := range audit.FailCases {
		md.WriteString("- " + item.Family + "/" + item.Variant + "/" + item.ID + " [" + item.FailureKind + "]")
		if item.Difficulty != "" {
			md.WriteString(" difficulty=" + item.Difficulty)
		}
		if item.WantSummary != "" || item.GotSummary != "" {
			md.WriteString(" | want: " + item.WantSummary + " | got: " + item.GotSummary)
		}
		md.WriteString("\n")
	}
	md.WriteString("\n## Suspicious Cases Requiring Manual Audit\n")
	for _, item := range audit.Suspicious {
		md.WriteString("- " + item.Family + "/" + item.Variant + "/" + item.ID)
		if item.FailureKind != "" {
			md.WriteString(" [" + item.FailureKind + "]")
		}
		if item.Note != "" {
			md.WriteString(" | " + item.Note)
		}
		md.WriteString("\n")
	}
	if err := os.WriteFile(filepath.Join(reportDir, "audit.md"), []byte(md.String()), 0o644); err != nil {
		return err
	}

	refined := buildRefinedAuditPayload(payload)
	refinedData, err := json.MarshalIndent(refined, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(reportDir, "audit-refined.json"), refinedData, 0o644); err != nil {
		return err
	}
	var refinedMD strings.Builder
	refinedMD.WriteString("# Refined Benchmark Audit\n\n")
	refinedMD.WriteString("Generated: " + refined.GeneratedAt + "\n\n")
	refinedMD.WriteString("## Fail Cases\n")
	for _, item := range refined.FailCases {
		refinedMD.WriteString("- " + item.Family + "/" + item.Variant + "/" + item.ID)
		if item.FailureKind != "" {
			refinedMD.WriteString(" [" + item.FailureKind + "]")
		}
		if item.Difficulty != "" {
			refinedMD.WriteString(" difficulty=" + item.Difficulty)
		}
		if item.WantSummary != "" || item.GotSummary != "" {
			refinedMD.WriteString(" | want: " + item.WantSummary + " | got: " + item.GotSummary)
		}
		refinedMD.WriteString("\n")
	}
	refinedMD.WriteString("\n## Suspicious Cases Requiring Manual Audit\n")
	for _, item := range refined.Suspicious {
		refinedMD.WriteString("- " + item.Family + "/" + item.Variant + "/" + item.ID)
		if item.FailureKind != "" {
			refinedMD.WriteString(" [" + item.FailureKind + "]")
		}
		if item.Note != "" {
			refinedMD.WriteString(" | " + item.Note)
		}
		refinedMD.WriteString("\n")
	}
	return os.WriteFile(filepath.Join(reportDir, "audit-refined.md"), []byte(refinedMD.String()), 0o644)
}

func buildAuditPayload(payload reportPayload) auditPayload {
	audit := auditPayload{
		GeneratedAt: payload.GeneratedAt,
	}
	for _, summary := range payload.Summaries {
		for _, record := range summary.Cases {
			item := auditCase{
				ID:           record.ID,
				Family:       record.Family,
				Variant:      record.Variant,
				Difficulty:   record.Difficulty,
				FailureKind:  record.FailureKind,
				WantSummary:  record.WantSummary,
				FirstSummary: record.FirstSummary,
				GotSummary:   record.GotSummary,
				Tags:         append([]string(nil), record.Tags...),
				Note:         record.Note,
			}
			if !record.Exact {
				audit.FailCases = append(audit.FailCases, item)
			}
			if !record.Exact || record.CleanerApplied || record.CleanerHurt || record.CleanerHelped || (record.FirstSummary != "" && record.GotSummary != "" && record.FirstSummary != record.GotSummary) {
				if record.SelectionReason != "" {
					item.Note = strings.TrimSpace(strings.Join([]string{
						strings.TrimSpace(item.Note),
						"selection_reason=" + strings.TrimSpace(record.SelectionReason),
					}, " | "))
				}
				audit.Suspicious = append(audit.Suspicious, item)
			}
		}
	}
	return audit
}

func buildRefinedAuditPayload(payload reportPayload) auditPayload {
	refined := auditPayload{
		GeneratedAt: payload.GeneratedAt,
	}
	for _, summary := range payload.Summaries {
		for _, record := range summary.Cases {
			item := auditCase{
				ID:           record.ID,
				Family:       record.Family,
				Variant:      record.Variant,
				Difficulty:   record.Difficulty,
				FailureKind:  record.FailureKind,
				WantSummary:  record.WantSummary,
				FirstSummary: record.FirstSummary,
				GotSummary:   record.GotSummary,
				Tags:         append([]string(nil), record.Tags...),
				Note:         record.Note,
			}
			if !record.Exact {
				refined.FailCases = append(refined.FailCases, item)
			}
			if !record.Exact || record.CleanerApplied || record.CleanerHelped || record.CleanerHurt {
				noteParts := make([]string, 0, 4)
				if item.Note != "" {
					noteParts = append(noteParts, strings.TrimSpace(item.Note))
				}
				if record.SelectionReason != "" {
					noteParts = append(noteParts, "selection_reason="+strings.TrimSpace(record.SelectionReason))
				}
				if record.CleanerApplied {
					noteParts = append(noteParts, "applied=true")
				}
				item.Note = strings.Join(noteParts, " | ")
				refined.Suspicious = append(refined.Suspicious, item)
			}
		}
	}
	return refined
}

func benchmarkRunMode(cfg runConfig) string {
	switch {
	case cfg.limit > 0:
		return "smoke"
	case cfg.caseFilter != "" || len(cfg.tagFilter) > 0 || len(cfg.include) < 2:
		return "filtered"
	default:
		return "full"
	}
}

func sortedFilterKeys(values map[string]bool) []string {
	keys := make([]string, 0, len(values))
	for key, enabled := range values {
		if enabled {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return keys
}

func cloneSummaries(summaries []variantSummary) []variantSummary {
	out := make([]variantSummary, len(summaries))
	for i, summary := range summaries {
		out[i] = summary
		if summary.FailureKinds != nil {
			out[i].FailureKinds = make(map[string]int, len(summary.FailureKinds))
			for key, value := range summary.FailureKinds {
				out[i].FailureKinds[key] = value
			}
		}
		out[i].Cases = append([]caseResult(nil), summary.Cases...)
		for j, record := range summary.Cases {
			out[i].Cases[j] = record
			out[i].Cases[j].Tags = append([]string(nil), record.Tags...)
			out[i].Cases[j].ModelCalls = append([]modelCall(nil), record.ModelCalls...)
		}
	}
	return out
}

func materializeArtifacts(cfg runConfig, payload *reportPayload) error {
	if !cfg.copyArtifacts {
		for si := range payload.Summaries {
			for ci := range payload.Summaries[si].Cases {
				rewriteCasePaths(&payload.Summaries[si].Cases[ci], false, cfg.reportDir, nil)
			}
		}
		return nil
	}
	artifactRoot := filepath.Join(cfg.reportDir, "artifacts")
	if err := os.MkdirAll(artifactRoot, 0o755); err != nil {
		return err
	}
	copied := map[string]string{}
	for si := range payload.Summaries {
		for ci := range payload.Summaries[si].Cases {
			rewriteCasePaths(&payload.Summaries[si].Cases[ci], true, artifactRoot, copied)
		}
	}
	return nil
}

func rewriteCasePaths(record *caseResult, copyArtifacts bool, baseDir string, copied map[string]string) {
	record.AudioPath = materializePath(record.AudioPath, copyArtifacts, baseDir, copied)
}

func materializePath(path string, copyArtifacts bool, baseDir string, copied map[string]string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if !copyArtifacts {
		return "file://" + path
	}
	if rel, ok := copied[path]; ok {
		return rel
	}
	name := sanitizeArtifactName(filepath.Base(path))
	relPath := filepath.Join("artifacts", shortHash(path), name)
	destPath := filepath.Join(baseDir, shortHash(path), name)
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err == nil {
		if err := copyFile(path, destPath); err == nil {
			copied[path] = filepath.ToSlash(relPath)
			return filepath.ToSlash(relPath)
		}
	}
	return "file://" + path
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

func sanitizeArtifactName(name string) string {
	name = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z':
			return r
		case r >= 'A' && r <= 'Z':
			return r
		case r >= '0' && r <= '9':
			return r
		case r == '.' || r == '-' || r == '_':
			return r
		default:
			return '_'
		}
	}, name)
	if name == "" {
		return "artifact.bin"
	}
	return name
}

func writeHTMLReport(path string, payload reportPayload) error {
	dataJSON, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	tpl := template.Must(template.New("report").Parse(reportTemplate))
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	return tpl.Execute(file, map[string]any{
		"DataJSON": template.JS(dataJSON),
	})
}

const reportTemplate = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Shelfy ingest benchmark report</title>
  <style>
    :root { color-scheme: light; --bg:#f5f1ea; --panel:#fffdf8; --text:#1e1b18; --muted:#70665d; --line:#d8d0c6; --accent:#a74f18; --bad:#b42318; --good:#0a7a33; }
    body { margin:0; font-family: "Iowan Old Style", Georgia, serif; background: radial-gradient(circle at top, #fff4de, var(--bg) 40%); color:var(--text); }
    .wrap { max-width: 1280px; margin:0 auto; padding: 24px; }
    .hero { background: linear-gradient(135deg, rgba(167,79,24,0.10), rgba(167,79,24,0.02)); border:1px solid var(--line); border-radius:24px; padding:24px; margin-bottom:20px; }
    .hero h1 { margin:0 0 8px; font-size: 34px; }
    .hero p { margin:0; color:var(--muted); }
    .hero .hero-meta { margin-top:14px; display:grid; gap:10px; grid-template-columns: repeat(auto-fit, minmax(220px, 1fr)); }
    .hero .meta-card { border:1px solid var(--line); border-radius:16px; padding:12px; background:rgba(255,255,255,0.65); }
    .summary { display:grid; grid-template-columns: repeat(auto-fit, minmax(240px, 1fr)); gap:12px; margin:18px 0 0; }
    .tile, .controls, .group, .variant { background:var(--panel); border:1px solid var(--line); border-radius:18px; }
    .tile { padding:16px; }
    .tile h2 { margin:0 0 12px; font-size:20px; }
    .tile strong { display:block; font-size: 28px; margin-top:8px; }
    .controls { padding:16px; margin-bottom:20px; display:grid; gap:12px; grid-template-columns: repeat(auto-fit, minmax(220px, 1fr)); }
    .controls label { display:block; font-size:12px; text-transform:uppercase; letter-spacing:0.08em; color:var(--muted); margin-bottom:6px; }
    .controls input, .controls select { width:100%; padding:10px 12px; border:1px solid var(--line); border-radius:12px; background:#fff; font:inherit; }
    table { width:100%; border-collapse: collapse; font-size:14px; }
    th, td { text-align:left; padding:8px 10px; border-bottom:1px solid var(--line); vertical-align:top; }
    th { color:var(--muted); font-size:12px; text-transform:uppercase; letter-spacing:0.08em; }
    .group { padding:18px; margin-bottom:20px; }
    .group-head { display:flex; justify-content:space-between; gap:16px; align-items:flex-start; margin-bottom:14px; }
    .group-head h2 { margin:0; font-size:24px; }
    .meta { color:var(--muted); font-size:14px; }
    .tags { display:flex; flex-wrap:wrap; gap:6px; margin-top:10px; }
    .tag { border:1px solid var(--line); border-radius:999px; padding:4px 10px; font-size:12px; color:var(--muted); }
    .variants { display:grid; gap:14px; }
    .variant { padding:16px; }
    .variant.good { border-color: rgba(10,122,51,0.35); }
    .variant.bad { border-color: rgba(180,35,24,0.35); }
    .variant h3 { margin:0 0 8px; font-size:18px; }
    .status { font-size:12px; letter-spacing:0.08em; text-transform:uppercase; color:var(--muted); margin-bottom:10px; }
    .status.pass { color:var(--good); }
    .status.fail { color:var(--bad); }
    .grid { display:grid; gap:12px; grid-template-columns: repeat(auto-fit, minmax(260px, 1fr)); }
    .panel { border:1px solid var(--line); border-radius:14px; padding:12px; background:#fff; }
    .panel h4 { margin:0 0 8px; font-size:14px; text-transform:uppercase; letter-spacing:0.08em; color:var(--muted); }
    .panel img { width:100%; border-radius:12px; display:block; background:#eee; }
    .panel audio { width:100%; }
    pre { white-space:pre-wrap; word-break:break-word; margin:0; font-family: ui-monospace, SFMono-Regular, monospace; font-size: 12px; }
    .review-note { margin-top:14px; display:grid; gap:8px; grid-template-columns: 160px 1fr; }
    .review-note textarea { width:100%; min-height:72px; padding:10px 12px; border:1px solid var(--line); border-radius:12px; resize:vertical; font:inherit; }
    .matrix-stack { display:grid; gap:16px; }
    .subtable h3 { margin:0 0 8px; font-size:16px; }
    .hidden { display:none !important; }
  </style>
</head>
<body>
  <div class="wrap">
    <section class="hero">
      <h1>Shelfy Ingest Benchmark</h1>
      <p>Interactive benchmark audit for public audio assets and parser-only text cases.</p>
      <div class="hero-meta" id="run-meta"></div>
      <div class="summary" id="summary"></div>
    </section>

    <section class="controls">
      <div>
        <label for="search">Search</label>
        <input id="search" placeholder="case id, note, transcript, prompt...">
      </div>
      <div>
        <label for="family">Family</label>
        <select id="family">
          <option value="">All families</option>
        </select>
      </div>
      <div>
        <label for="variant">Variant</label>
        <select id="variant">
          <option value="">All variants</option>
        </select>
      </div>
      <div>
        <label for="status">Status</label>
        <select id="status">
          <option value="">Pass + fail</option>
          <option value="pass">Pass only</option>
          <option value="fail">Fail only</option>
        </select>
      </div>
    </section>

    <section class="tile">
      <table id="summary-table"></table>
    </section>

    <section class="tile">
      <h2>Scenario Matrix</h2>
      <table id="scenario-matrix"></table>
    </section>

    <section class="tile">
      <h2>Tag Matrix</h2>
      <div class="matrix-stack" id="tag-matrix"></div>
    </section>

    <section class="tile">
      <h2>Difficulty Matrix</h2>
      <table id="difficulty-matrix"></table>
    </section>

    <section class="tile">
      <h2>Cleaner Delta</h2>
      <table id="review-delta"></table>
    </section>

    <section class="tile">
      <h2>Hardest Failing Tags</h2>
      <table id="hardest-tags"></table>
    </section>

    <section class="tile">
      <h2>Audit Queue</h2>
      <table id="audit-queue"></table>
    </section>

    <div id="groups"></div>
  </div>
  <script>
    const REPORT = {{.DataJSON}};
    const summaryEl = document.getElementById('summary');
    const runMetaEl = document.getElementById('run-meta');
    const summaryTable = document.getElementById('summary-table');
    const scenarioMatrixEl = document.getElementById('scenario-matrix');
    const tagMatrixEl = document.getElementById('tag-matrix');
    const difficultyMatrixEl = document.getElementById('difficulty-matrix');
    const reviewDeltaEl = document.getElementById('review-delta');
    const hardestTagsEl = document.getElementById('hardest-tags');
    const auditQueueEl = document.getElementById('audit-queue');
    const groupsEl = document.getElementById('groups');
    const searchEl = document.getElementById('search');
    const familyEl = document.getElementById('family');
    const variantEl = document.getElementById('variant');
    const statusEl = document.getElementById('status');

    const variantKeys = new Set();
    const familyKeys = new Set();
    for (const summary of REPORT.summaries) {
      variantKeys.add(summary.variant);
      familyKeys.add(summary.family);
    }
    for (const family of [...familyKeys].sort()) {
      const option = document.createElement('option');
      option.value = family;
      option.textContent = family;
      familyEl.appendChild(option);
    }
    for (const variant of [...variantKeys].sort()) {
      const option = document.createElement('option');
      option.value = variant;
      option.textContent = variant;
      variantEl.appendChild(option);
    }

    function renderSummary() {
      const totalCases = REPORT.summaries.reduce((sum, item) => sum + item.total, 0);
      const uniqueCases = groupedCases().length;
      const totalExact = REPORT.summaries.reduce((sum, item) => sum + item.exact, 0);
      const totalFirstExact = REPORT.summaries.reduce((sum, item) => sum + (item.first_exact || 0), 0);
      const totalFailed = REPORT.summaries.reduce((sum, item) => sum + item.failed, 0);
      const textCalls = REPORT.summaries.reduce((sum, item) => sum + item.text_calls, 0);
      const timeouts = REPORT.summaries.reduce((sum, item) => sum + (item.timeouts || 0), 0);
      const assetErrors = REPORT.summaries.reduce((sum, item) => sum + (item.asset_errors || 0), 0);
      const cleanerEligible = REPORT.summaries.reduce((sum, item) => sum + (item.cleaner_eligible || 0), 0);
      const cleanerCalled = REPORT.summaries.reduce((sum, item) => sum + (item.cleaner_called || 0), 0);
      const cleanerChangedInput = REPORT.summaries.reduce((sum, item) => sum + (item.cleaner_changed_input || 0), 0);
      const candidateValid = REPORT.summaries.reduce((sum, item) => sum + (item.candidate_valid || 0), 0);
      const cleanerApplied = REPORT.summaries.reduce((sum, item) => sum + (item.cleaner_applied || 0), 0);
      const cleanerHelped = REPORT.summaries.reduce((sum, item) => sum + (item.cleaner_helped || 0), 0);
      const cleanerHurt = REPORT.summaries.reduce((sum, item) => sum + (item.cleaner_hurt || 0), 0);
      const cleanerNoop = REPORT.summaries.reduce((sum, item) => sum + (item.cleaner_noop || 0), 0);
      const cleanerSameCandidate = REPORT.summaries.reduce((sum, item) => sum + (item.cleaner_same_candidate || 0), 0);
      const failureKinds = {};
      for (const summary of REPORT.summaries) {
        for (const [kind, count] of Object.entries(summary.failure_kinds || {})) {
          failureKinds[kind] = (failureKinds[kind] || 0) + count;
        }
      }
      const reviewRollup = readReviewRollup();
      runMetaEl.innerHTML = [
        metaCard('Run mode', REPORT.run_mode || 'unknown'),
        metaCard('Include', (REPORT.include || []).join(', ') || 'all'),
        metaCard('Limit', REPORT.limit ? String(REPORT.limit) : 'none'),
        metaCard('Filters', [REPORT.case_filter || '', (REPORT.tag_filter || []).join(', ')].filter(Boolean).join(' | ') || 'none'),
        metaCard('Corpus', REPORT.corpus),
        metaCard('Model', REPORT.model)
      ].join('');
      summaryEl.innerHTML = [
        tile('Run Mode', REPORT.run_mode || 'unknown'),
        tile('Unique Cases', uniqueCases),
        tile('Cases', totalCases),
        tile('First Exact', totalFirstExact),
        tile('Exact', totalExact),
        tile('Fail', totalFailed),
        tile('Timeouts', timeouts),
        tile('Asset Errors', assetErrors),
        tile('LLM Text Calls', textCalls),
        tile('Cleaner Eligible', cleanerEligible),
        tile('Cleaner Called', cleanerCalled),
        tile('Input Changed', cleanerChangedInput),
        tile('Candidate Valid', candidateValid),
        tile('Cleaner Applied', cleanerApplied),
        tile('Cleaner Helped', cleanerHelped),
        tile('Cleaner Hurt', cleanerHurt),
        tile('Cleaner No-op', cleanerNoop),
        tile('Same Candidate', cleanerSameCandidate),
        tile('Manual Verdicts', reviewRollup),
        tile('Generated', REPORT.generated_at.replace('T', ' ').replace('Z', ' UTC'))
      ].join('');
      const rows = REPORT.summaries.map(function(item) {
        return '<tr>'
          + '<td>' + escapeHTML(item.family) + '</td>'
          + '<td>' + escapeHTML(item.variant) + '</td>'
          + '<td>' + item.total + '</td>'
          + '<td>' + (item.first_exact || 0) + '</td>'
          + '<td>' + item.exact + '</td>'
          + '<td>' + item.failed + '</td>'
          + '<td>' + (item.timeouts || 0) + '</td>'
          + '<td>' + (item.asset_errors || 0) + '</td>'
          + '<td>' + item.text_calls + '</td>'
          + '<td>' + (item.cleaner_eligible || 0) + '</td>'
          + '<td>' + (item.cleaner_called || 0) + '</td>'
          + '<td>' + (item.cleaner_changed_input || 0) + '</td>'
          + '<td>' + (item.candidate_valid || 0) + '</td>'
          + '<td>' + (item.cleaner_applied || 0) + '</td>'
          + '<td>' + (item.cleaner_helped || 0) + '</td>'
          + '<td>' + (item.cleaner_hurt || 0) + '</td>'
          + '<td>' + (item.cleaner_noop || 0) + '</td>'
          + '<td>' + (item.cleaner_same_candidate || 0) + '</td>'
          + '<td>' + escapeHTML(formatFailureKinds(item.failure_kinds || {})) + '</td>'
          + '<td>' + Math.round(item.duration_ns / 1e6) + '</td>'
          + '</tr>';
      }).join('');
      summaryTable.innerHTML = '<thead>'
        + '<tr><th>Family</th><th>Variant</th><th>Total</th><th>First exact</th><th>Final exact</th><th>Fail</th><th>Timeout</th><th>Asset</th><th>Text</th><th>Eligible</th><th>Called</th><th>Changed</th><th>Valid</th><th>Applied</th><th>Helped</th><th>Hurt</th><th>No-op</th><th>Same</th><th>Failure kinds</th><th>ms</th></tr>'
        + '</thead><tbody>' + rows + '</tbody>';
      renderScenarioMatrix();
      renderTagMatrix();
      renderDifficultyMatrix();
      renderCleanerDelta();
      renderHardestTags();
      renderAuditQueue();
    }

    function renderScenarioMatrix() {
      const states = ['ready', 'needs_expiry', 'needs_name', 'reject'];
      const rows = REPORT.summaries.map(function(summary) {
        const cells = states.map(function(state) {
          const records = (summary.cases || []).filter(function(record) { return record.want_state === state; });
          const exact = records.filter(function(record) { return record.exact; }).length;
          return '<td>' + escapeHTML(formatRatio(exact, records.length)) + '</td>';
        }).join('');
        return '<tr>'
          + '<td>' + escapeHTML(summary.family) + '</td>'
          + '<td>' + escapeHTML(summary.variant) + '</td>'
          + '<td>' + summary.total + '</td>'
          + cells
          + '</tr>';
      }).join('');
      scenarioMatrixEl.innerHTML = '<thead>'
        + '<tr><th>Family</th><th>Variant</th><th>Total</th><th>Ready</th><th>Needs expiry</th><th>Needs name</th><th>Reject</th></tr>'
        + '</thead><tbody>' + rows + '</tbody>';
    }

    function renderTagMatrix() {
      const ignoredTags = new Set(['parser_only', 'public_audio', 'asr_command']);
      const byFamily = new Map();
      for (const summary of REPORT.summaries) {
        if (!byFamily.has(summary.family)) {
          byFamily.set(summary.family, []);
        }
        byFamily.get(summary.family).push(summary);
      }
      const sections = [];
      for (const family of [...byFamily.keys()].sort()) {
        const summaries = byFamily.get(family);
        const tagCounts = new Map();
        for (const summary of summaries) {
          for (const record of summary.cases || []) {
            for (const tag of record.tags || []) {
              if (ignoredTags.has(tag)) continue;
              tagCounts.set(tag, (tagCounts.get(tag) || 0) + 1);
            }
          }
        }
        const tags = [...tagCounts.entries()]
          .sort(function(a, b) {
            if (b[1] !== a[1]) return b[1] - a[1];
            return a[0].localeCompare(b[0]);
          })
          .map(function(entry) { return entry[0]; });
        if (tags.length === 0) continue;
        const rows = summaries.map(function(summary) {
          const cells = tags.map(function(tag) {
            const records = (summary.cases || []).filter(function(record) { return (record.tags || []).includes(tag); });
            const exact = records.filter(function(record) { return record.exact; }).length;
            return '<td>' + escapeHTML(formatRatio(exact, records.length)) + '</td>';
          }).join('');
          return '<tr>'
            + '<td>' + escapeHTML(summary.variant) + '</td>'
            + '<td>' + summary.total + '</td>'
            + cells
            + '</tr>';
        }).join('');
        sections.push('<section class="subtable">'
          + '<h3>' + escapeHTML(family) + '</h3>'
          + '<table><thead><tr><th>Variant</th><th>Total</th>'
          + tags.map(function(tag) { return '<th>' + escapeHTML(tag) + '</th>'; }).join('')
          + '</tr></thead><tbody>' + rows + '</tbody></table></section>');
      }
      tagMatrixEl.innerHTML = sections.join('');
    }

    function renderDifficultyMatrix() {
      const levels = ['medium', 'hard'];
      const rows = REPORT.summaries.map(function(summary) {
        const cells = levels.map(function(level) {
          const records = (summary.cases || []).filter(function(record) { return record.difficulty === level; });
          const exact = records.filter(function(record) { return record.exact; }).length;
          return '<td>' + escapeHTML(formatRatio(exact, records.length)) + '</td>';
        }).join('');
        return '<tr><td>' + escapeHTML(summary.family) + '</td><td>' + escapeHTML(summary.variant) + '</td><td>' + summary.total + '</td>' + cells + '</tr>';
      }).join('');
      difficultyMatrixEl.innerHTML = '<thead><tr><th>Family</th><th>Variant</th><th>Total</th><th>Medium</th><th>Hard</th></tr></thead><tbody>' + rows + '</tbody>';
    }

    function renderCleanerDelta() {
      const rows = REPORT.summaries.map(function(summary) {
        return '<tr>'
          + '<td>' + escapeHTML(summary.family) + '</td>'
          + '<td>' + escapeHTML(summary.variant) + '</td>'
          + '<td>' + (summary.first_exact || 0) + '</td>'
          + '<td>' + summary.exact + '</td>'
          + '<td>' + (summary.cleaner_called || 0) + '</td>'
          + '<td>' + (summary.cleaner_changed_input || 0) + '</td>'
          + '<td>' + (summary.cleaner_applied || 0) + '</td>'
          + '<td>' + (summary.cleaner_helped || 0) + '</td>'
          + '<td>' + (summary.cleaner_hurt || 0) + '</td>'
          + '<td>' + (summary.cleaner_noop || 0) + '</td>'
          + '</tr>';
      }).join('');
      reviewDeltaEl.innerHTML = '<thead><tr><th>Family</th><th>Variant</th><th>First exact</th><th>Final exact</th><th>Called</th><th>Changed</th><th>Applied</th><th>Helped</th><th>Hurt</th><th>No-op</th></tr></thead><tbody>' + rows + '</tbody>';
    }

    function renderHardestTags() {
      const ignoredTags = new Set(['parser_only', 'public_audio', 'asr_command']);
      const scores = new Map();
      for (const summary of REPORT.summaries) {
        for (const record of summary.cases || []) {
          for (const tag of record.tags || []) {
            if (ignoredTags.has(tag)) continue;
            const key = summary.family + '::' + tag;
            if (!scores.has(key)) scores.set(key, { family: summary.family, tag: tag, total: 0, exact: 0 });
            const item = scores.get(key);
            item.total += 1;
            if (record.exact) item.exact += 1;
          }
        }
      }
      const rows = [...scores.values()]
        .filter(item => item.total >= 2)
        .sort(function(a, b) {
          const ar = a.total ? a.exact / a.total : 0;
          const br = b.total ? b.exact / b.total : 0;
          if (ar !== br) return ar - br;
          return b.total - a.total;
        })
        .slice(0, 20)
        .map(function(item) {
          return '<tr><td>' + escapeHTML(item.family) + '</td><td>' + escapeHTML(item.tag) + '</td><td>' + item.exact + '/' + item.total + '</td><td>' + Math.round((item.exact / item.total) * 100) + '%</td></tr>';
        }).join('');
      hardestTagsEl.innerHTML = '<thead><tr><th>Family</th><th>Tag</th><th>Exact</th><th>Rate</th></tr></thead><tbody>' + rows + '</tbody>';
    }

    function renderAuditQueue() {
      const rows = [];
      for (const summary of REPORT.summaries) {
        for (const record of summary.cases || []) {
          if (!record.exact || record.cleaner_applied || record.cleaner_helped || record.cleaner_hurt || record.selection_reason) {
            rows.push(record);
          }
        }
      }
      rows.sort(function(a, b) {
        if (a.exact !== b.exact) return a.exact ? 1 : -1;
        return (a.family + '/' + a.id + '/' + a.variant).localeCompare(b.family + '/' + b.id + '/' + b.variant);
      });
      auditQueueEl.innerHTML = '<thead><tr><th>Family</th><th>Case</th><th>Variant</th><th>Difficulty</th><th>Failure</th><th>Selection</th><th>Note</th></tr></thead><tbody>'
        + rows.slice(0, 40).map(function(record) {
          return '<tr>'
            + '<td>' + escapeHTML(record.family) + '</td>'
            + '<td>' + escapeHTML(record.id) + '</td>'
            + '<td>' + escapeHTML(record.variant) + '</td>'
            + '<td>' + escapeHTML(record.difficulty || '') + '</td>'
            + '<td>' + escapeHTML(record.failure_kind || '') + '</td>'
            + '<td>' + escapeHTML(record.selection_reason || '') + '</td>'
            + '<td>' + escapeHTML(record.note || '') + '</td>'
            + '</tr>';
        }).join('')
        + '</tbody>';
    }

    function tile(label, value) {
      return '<div class="tile"><div class="meta">' + escapeHTML(label) + '</div><strong>' + escapeHTML(String(value)) + '</strong></div>';
    }

    function metaCard(label, value) {
      return '<div class="meta-card"><div class="meta">' + escapeHTML(label) + '</div><div>' + escapeHTML(String(value)) + '</div></div>';
    }

    function caseKey(record) {
      return record.family + '::' + record.id;
    }

    function variantReviewKey(record) {
      return 'review:' + record.family + ':' + record.id + ':' + record.variant;
    }

    function groupedCases() {
      const groups = new Map();
      for (const summary of REPORT.summaries) {
        for (const record of summary.cases) {
          const key = caseKey(record);
          if (!groups.has(key)) {
            groups.set(key, {
              id: record.id,
              family: record.family,
              dataset: record.dataset || '',
              difficulty: record.difficulty || '',
              source_page: record.source_page || '',
              license: record.license || '',
              note: record.note || '',
              tags: record.tags || [],
              variants: []
            });
          }
          groups.get(key).variants.push(record);
        }
      }
      return [...groups.values()].sort((a, b) => (a.family + '/' + a.id).localeCompare(b.family + '/' + b.id));
    }

    function renderGroups() {
      const search = searchEl.value.trim().toLowerCase();
      const family = familyEl.value;
      const variant = variantEl.value;
      const status = statusEl.value;
      const groups = groupedCases();
      const html = [];
      for (const group of groups) {
        const visibleVariants = group.variants.filter(record => {
          if (family && record.family !== family) return false;
          if (variant && record.variant !== variant) return false;
          if (status === 'pass' && !record.exact) return false;
          if (status === 'fail' && record.exact) return false;
          if (!search) return true;
          const haystack = JSON.stringify(record).toLowerCase();
          return haystack.includes(search);
        });
        if (visibleVariants.length === 0) continue;
        html.push('<section class="group">'
          + '<div class="group-head"><div>'
          + '<div class="meta">' + escapeHTML(group.family) + '</div>'
          + '<h2>' + escapeHTML(group.id) + '</h2>'
          + '<div class="meta">' + escapeHTML(group.dataset) + ' · ' + escapeHTML(group.license || 'no-license-meta') + ' · difficulty=' + escapeHTML(group.difficulty || '') + '</div>'
          + '<div class="tags">' + group.tags.map(function(tag) { return '<span class="tag">' + escapeHTML(tag) + '</span>'; }).join('') + '</div>'
          + '</div>'
          + (group.source_page ? '<div class="meta"><a href="' + escapeAttr(group.source_page) + '">source</a></div>' : '')
          + '</div>'
          + (group.note ? '<p class="meta">' + escapeHTML(group.note) + '</p>' : '')
          + '<div class="variants">' + visibleVariants.map(renderVariant).join('') + '</div>'
          + '</section>');
      }
      groupsEl.innerHTML = html.join('');
      bindReviewControls();
    }

    function renderVariant(record) {
      const statusLabel = record.exact ? 'pass' : 'fail';
      const review = JSON.parse(localStorage.getItem(variantReviewKey(record)) || '{"verdict":"","note":""}');
      const reviewKey = variantReviewKey(record);
      return '<article class="variant ' + (statusLabel === 'pass' ? 'good' : 'bad') + '">'
        + '<div class="status ' + statusLabel + '">' + statusLabel + '</div>'
        + '<h3>' + escapeHTML(record.variant) + '</h3>'
        + (record.failure_kind ? '<p><strong>' + escapeHTML(record.failure_kind) + '</strong></p>' : '')
        + (record.want_summary ? '<p><strong>Want:</strong> ' + escapeHTML(record.want_summary) + '</p>' : '')
        + (record.first_summary ? '<p><strong>First:</strong> ' + escapeHTML(record.first_summary) + '</p>' : '')
        + (record.got_summary ? '<p><strong>Got:</strong> ' + escapeHTML(record.got_summary) + '</p>' : '')
        + ((record.cleaner_called || record.cleaner_applied) ? '<p><strong>Cleaner:</strong> ' + escapeHTML((record.cleaner_applied ? 'applied' : 'kept baseline') + (record.selection_reason ? ' · ' + record.selection_reason : '')) + '</p>' : '')
        + (record.error ? '<p><strong>Error:</strong> ' + escapeHTML(record.error) + '</p>' : '')
        + '<div class="grid">' + renderInputs(record) + renderLLM(record) + '</div>'
        + '<div class="review-note"><div>'
        + '<label class="meta" for="' + escapeAttr(reviewKey) + '">Manual verdict</label>'
        + '<select data-review-select="' + escapeAttr(reviewKey) + '">'
        + '<option value="">unset</option>'
        + '<option value="usable" ' + (review.verdict === 'usable' ? 'selected' : '') + '>usable</option>'
        + '<option value="partial" ' + (review.verdict === 'partial' ? 'selected' : '') + '>partial</option>'
        + '<option value="bad" ' + (review.verdict === 'bad' ? 'selected' : '') + '>bad</option>'
        + '</select></div><div>'
        + '<label class="meta" for="' + escapeAttr(reviewKey) + '-note">Review note</label>'
        + '<textarea data-review-note="' + escapeAttr(reviewKey) + '" placeholder="What looked good or bad?">' + escapeHTML(review.note || '') + '</textarea>'
        + '</div></div></article>';
    }

    function renderInputs(record) {
      const blocks = [];
      if (record.input_text) {
        blocks.push(panel('Text input', '<pre>' + escapeHTML(record.input_text) + '</pre>'));
      }
      if (record.normalized_text) {
        blocks.push(panel('Normalized', '<pre>' + escapeHTML(record.normalized_text) + '</pre>'));
      }
      if (record.audio_path) {
        blocks.push(panel('Audio', '<audio controls src="' + escapeAttr(record.audio_path) + '"></audio>'));
      }
      if (record.transcript_ref || record.transcript || record.normalized_text) {
        blocks.push(panel('Transcript', '<pre>ref: ' + escapeHTML(record.transcript_ref || '')
          + '\nraw: ' + escapeHTML(record.transcript || '')
          + '\nnormalized: ' + escapeHTML(record.normalized_text || '')
          + '\ncer: ' + escapeHTML(String(record.transcript_cer || 0)) + '</pre>'));
      }
      if (record.first_summary || record.got_summary) {
        blocks.push(panel('Pipeline', '<pre>first: ' + escapeHTML(record.first_summary || '')
          + '\nfinal: ' + escapeHTML(record.got_summary || '')
          + '\nfirst_ms: ' + escapeHTML(String(record.time_to_first_ms || 0))
          + '\nfinal_ms: ' + escapeHTML(String(record.time_to_final_ms || 0))
          + '\ncleaner_eligible: ' + escapeHTML(String(!!record.cleaner_eligible))
          + '\ncleaner_called: ' + escapeHTML(String(!!record.cleaner_called))
          + '\ncleaner_changed_input: ' + escapeHTML(String(!!record.cleaner_changed_input))
          + '\ncandidate_valid: ' + escapeHTML(String(!!record.candidate_valid))
          + '\ncleaner_applied: ' + escapeHTML(String(!!record.cleaner_applied))
          + '\ncleaner_helped: ' + escapeHTML(String(!!record.cleaner_helped))
          + '\ncleaner_hurt: ' + escapeHTML(String(!!record.cleaner_hurt))
          + '\ncleaner_noop: ' + escapeHTML(String(!!record.cleaner_noop))
          + '\ncleaner_same_candidate: ' + escapeHTML(String(!!record.cleaner_same_candidate))
          + '\ncleaner_reason_code: ' + escapeHTML(record.cleaner_reason_code || '')
          + '\nselection_reason: ' + escapeHTML(record.selection_reason || '')
          + '\nchosen_source: ' + escapeHTML(record.chosen_source || '')
          + (record.cleaned_input ? '\ncleaned_input: ' + escapeHTML(record.cleaned_input) : '')
          + '</pre>'));
      }
      if (record.parsed_summary) {
        blocks.push(panel('Final parse', '<pre>' + escapeHTML(record.parsed_summary) + '</pre>'));
      }
      return blocks.join('');
    }

    function renderLLM(record) {
      if (!record.model_calls || record.model_calls.length === 0) return '';
      return record.model_calls.map(function(call, index) {
        return panel(
          'LLM ' + (index + 1) + ' · ' + call.mode,
          '<pre>model: ' + escapeHTML(call.model || '')
          + '\nstatus: ' + escapeHTML(String(call.status || ''))
          + '\n\nprompt:\n' + escapeHTML(call.prompt || '')
          + '\n\nresponse:\n' + escapeHTML(call.response || '') + '</pre>'
        );
      }).join('');
    }

    function panel(title, inner) {
      return '<section class="panel"><h4>' + escapeHTML(title) + '</h4>' + inner + '</section>';
    }

    function bindReviewControls() {
      document.querySelectorAll('[data-review-select]').forEach(select => {
        select.addEventListener('change', persistReview);
      });
      document.querySelectorAll('[data-review-note]').forEach(area => {
        area.addEventListener('input', persistReview);
      });
    }

    function persistReview(event) {
      const key = event.target.getAttribute('data-review-select') || event.target.getAttribute('data-review-note');
      if (!key) return;
      const select = document.querySelector('[data-review-select="' + CSS.escape(key) + '"]');
      const note = document.querySelector('[data-review-note="' + CSS.escape(key) + '"]');
      localStorage.setItem(key, JSON.stringify({
        verdict: select ? select.value : '',
        note: note ? note.value : ''
      }));
      renderSummary();
    }

    function readReviewRollup() {
      const counts = { usable: 0, partial: 0, bad: 0 };
      for (const group of groupedCases()) {
        for (const record of group.variants) {
          const raw = localStorage.getItem(variantReviewKey(record));
          if (!raw) continue;
          try {
            const review = JSON.parse(raw);
            if (review.verdict && counts[review.verdict] !== undefined) {
              counts[review.verdict]++;
            }
          } catch (_) {}
        }
      }
      return 'usable ' + counts.usable + ' · partial ' + counts.partial + ' · bad ' + counts.bad;
    }

    function formatFailureKinds(value) {
      const parts = [];
      for (const key of Object.keys(value).sort()) {
        parts.push(key + ':' + value[key]);
      }
      return parts.join(', ');
    }

    function formatRatio(exact, total) {
      if (!total) return '0/0';
      return exact + '/' + total + ' (' + Math.round((exact / total) * 100) + '%)';
    }

    function escapeHTML(value) {
      return String(value)
        .replaceAll('&', '&amp;')
        .replaceAll('<', '&lt;')
        .replaceAll('>', '&gt;')
        .replaceAll('"', '&quot;')
        .replaceAll("'", '&#39;');
    }

    function escapeAttr(value) {
      return escapeHTML(value);
    }

    renderSummary();
    renderGroups();
    [searchEl, familyEl, variantEl, statusEl].forEach(el => el.addEventListener('input', renderGroups));
    [familyEl, variantEl, statusEl].forEach(el => el.addEventListener('change', renderGroups));
  </script>
</body>
</html>`

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/igor/shelfy/internal/ingest"
)

const (
	defaultRuntimeBaseImage    = "shelfy-runtime-base:vosk-lib-0.3.45-small-ru-0.22"
	defaultPipelineWorkerImage = "shelfy-pipeline-worker:latest"
)

type corpus struct {
	ReferenceTime string      `json:"reference_time"`
	Text          []textCase  `json:"text"`
	Voice         []voiceCase `json:"voice"`
	LiveHard      liveHard    `json:"live_hard"`
}

type liveHard struct {
	TextIDs  []string `json:"text_ids"`
	VoiceIDs []string `json:"voice_ids"`
}

type textCase struct {
	ID         string   `json:"id"`
	Input      string   `json:"input"`
	WantState  string   `json:"want_state"`
	WantName   string   `json:"want_name"`
	WantDate   string   `json:"want_date"`
	Difficulty string   `json:"difficulty"`
	Tags       []string `json:"tags"`
	Note       string   `json:"note"`
}

type voiceCase struct {
	ID            string   `json:"id"`
	Dataset       string   `json:"dataset"`
	SourceID      string   `json:"source_id"`
	AssetPath     string   `json:"asset_path"`
	DownloadURL   string   `json:"download_url"`
	SourcePage    string   `json:"source_page"`
	License       string   `json:"license"`
	SHA256        string   `json:"sha256"`
	TranscriptRef string   `json:"transcript_ref"`
	WantState     string   `json:"want_state"`
	WantName      string   `json:"want_name"`
	WantDate      string   `json:"want_date"`
	Difficulty    string   `json:"difficulty"`
	Tags          []string `json:"tags"`
	Note          string   `json:"note"`
}

type runConfig struct {
	corpusPath          string
	ollamaBaseURL       string
	ollamaModel         string
	include             map[string]bool
	limit               int
	runtimeBaseImage    string
	pipelineWorkerImage string
	modelsDir           string
	voskBinaryHostPath  string
	voskModelPath       string
	voskGrammarPath     string
	cacheDir            string
	emitReport          bool
	reportDir           string
	copyArtifacts       bool
	caseFilter          string
	tagFilter           map[string]bool
	datasetSetup        bool
}

func main() {
	var (
		corpusPath          = flag.String("corpus", "internal/ingest/testdata/benchmark_corpus.json", "path to benchmark corpus")
		ollamaBaseURL       = flag.String("ollama-base-url", "http://127.0.0.1:11434", "Ollama base URL")
		ollamaModel         = flag.String("ollama-model", "gemma3:4b", "Ollama model")
		include             = flag.String("include", "text,voice,live_hard", "comma-separated families to run: text,voice,live_hard")
		limit               = flag.Int("limit", 0, "optional per-family case limit for faster smoke runs")
		runtimeBaseImage    = flag.String("runtime-base-image", defaultString(os.Getenv("SHELFY_RUNTIME_BASE_IMAGE"), defaultRuntimeBaseImage), "shared runtime image for docker-backed benchmark helpers")
		pipelineWorkerImage = flag.String("pipeline-worker-image", defaultString(os.Getenv("SHELFY_PIPELINE_WORKER_IMAGE"), defaultPipelineWorkerImage), "docker image with vosk-transcribe installed")
		modelsDir           = flag.String("models-dir", "models", "host path to repo models dir for docker-asr")
		voskBinaryHostPath  = flag.String("vosk-binary-host-path", "", "optional host path to a current-head Linux vosk-transcribe binary mounted into the docker benchmark runtime")
		voskModelPath       = flag.String("vosk-model-path", "/models/vosk-model-small-ru-0.22", "container path to the Vosk model directory")
		voskGrammarPath     = flag.String("vosk-grammar-path", "assets/asr/vosk-grammar.ru.json", "host path to optional Vosk grammar JSON file")
		cacheDir            = flag.String("cache-dir", "tmp/ingest-benchmark", "cache directory for downloaded public assets and generated reports")
		emitReport          = flag.Bool("emit-report", false, "write a local HTML audit report")
		reportDir           = flag.String("report-dir", "", "directory for the HTML benchmark report")
		copyArtifacts       = flag.Bool("copy-artifacts", true, "copy images/audio artifacts into the report directory")
		caseFilter          = flag.String("case-filter", "", "substring filter applied to case ids")
		tagFilter           = flag.String("tag-filter", "", "comma-separated tag filter; any matching tag keeps the case")
		datasetSetup        = flag.Bool("dataset-setup", false, "download and validate selected public assets before running variants")
	)
	flag.Parse()

	root, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}

	cfg := runConfig{
		corpusPath:          resolvePath(root, *corpusPath),
		ollamaBaseURL:       strings.TrimRight(*ollamaBaseURL, "/"),
		ollamaModel:         *ollamaModel,
		include:             parseFilterSet(*include),
		limit:               *limit,
		runtimeBaseImage:    *runtimeBaseImage,
		pipelineWorkerImage: *pipelineWorkerImage,
		modelsDir:           resolvePath(root, *modelsDir),
		voskBinaryHostPath:  resolveExistingPath(root, *voskBinaryHostPath),
		voskModelPath:       *voskModelPath,
		voskGrammarPath:     resolveExistingPath(root, *voskGrammarPath),
		cacheDir:            resolvePath(root, *cacheDir),
		emitReport:          *emitReport,
		reportDir:           resolvePath(root, *reportDir),
		copyArtifacts:       *copyArtifacts,
		caseFilter:          strings.ToLower(strings.TrimSpace(*caseFilter)),
		tagFilter:           parseFilterSet(*tagFilter),
		datasetSetup:        *datasetSetup,
	}

	suite, err := loadCorpus(cfg.corpusPath)
	if err != nil {
		log.Fatal(err)
	}
	if err := validateCorpus(suite); err != nil {
		log.Fatal(err)
	}

	liveHardText := filterTextCases(resolveLiveHardText(suite), cfg.caseFilter, cfg.tagFilter, cfg.limit)
	liveHardVoice := filterVoiceCases(resolveLiveHardVoice(suite), cfg.caseFilter, cfg.tagFilter, cfg.limit)
	suite.Text = filterTextCases(suite.Text, cfg.caseFilter, cfg.tagFilter, cfg.limit)
	suite.Voice = filterVoiceCases(suite.Voice, cfg.caseFilter, cfg.tagFilter, cfg.limit)
	if !cfg.include["voice"] {
		suite.Voice = nil
	}
	if !cfg.include["text"] {
		suite.Text = nil
	}
	if !cfg.include["live_hard"] {
		liveHardText = nil
		liveHardVoice = nil
	}

	referenceTime, err := time.Parse(time.RFC3339, suite.ReferenceTime)
	if err != nil {
		log.Fatalf("parse reference_time: %v", err)
	}

	runtime, err := newBenchRuntime(root, filepath.Dir(cfg.corpusPath), cfg)
	if err != nil {
		log.Fatal(err)
	}
	if err := ensureBenchmarkRuntimeImages(cfg); err != nil {
		log.Fatal(err)
	}
	if cfg.datasetSetup {
		prefetchSuite := corpus{Voice: append(append([]voiceCase(nil), suite.Voice...), liveHardVoice...)}
		prefetchSuite.Voice = dedupeVoiceCases(prefetchSuite.Voice)
		if err := runtime.prefetchAssets(context.Background(), prefetchSuite); err != nil {
			log.Fatal(err)
		}
	}

	proxy := newCountingOllamaProxy(cfg.ollamaBaseURL)
	defer proxy.Close()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	evaluator := ingest.NewEvaluator(proxy.URL(), cfg.ollamaModel, logger)

	var summaries []variantSummary
	if cfg.include["text"] {
		summaries = append(summaries,
			runTextVariant("text", "text_fast_only", suite.Text, proxy, func(ctx context.Context, tc textCase) (ingest.EvalResult, error) {
				return evaluator.FirstTextCard(ctx, tc.Input, referenceTime)
			}),
			runTextCleanerVariant("text", "text_fast_plus_cleaner", suite.Text, proxy, func(ctx context.Context, tc textCase) ingest.PipelineEvalResult {
				return evaluator.TextPipeline(ctx, tc.Input, referenceTime)
			}),
		)
	}
	if cfg.include["voice"] {
		summaries = append(summaries,
			runVoiceVariant(suite.Voice, voiceVariantSpec{
				Family: "voice",
				Name:   "voice_vosk_fast_only",
			}, proxy, runtime, evaluator, cfg, referenceTime),
			runVoiceCleanerVariant(suite.Voice, voiceVariantSpec{
				Family: "voice",
				Name:   "voice_vosk_fast_plus_cleaner",
			}, proxy, runtime, evaluator, cfg, referenceTime),
		)
	}
	if cfg.include["live_hard"] {
		summaries = append(summaries,
			runTextVariant("live_hard_text", "text_fast_only", liveHardText, proxy, func(ctx context.Context, tc textCase) (ingest.EvalResult, error) {
				return evaluator.FirstTextCard(ctx, tc.Input, referenceTime)
			}),
			runTextCleanerVariant("live_hard_text", "text_fast_plus_cleaner", liveHardText, proxy, func(ctx context.Context, tc textCase) ingest.PipelineEvalResult {
				return evaluator.TextPipeline(ctx, tc.Input, referenceTime)
			}),
			runVoiceVariant(liveHardVoice, voiceVariantSpec{
				Family: "live_hard_voice",
				Name:   "voice_vosk_fast_only",
			}, proxy, runtime, evaluator, cfg, referenceTime),
			runVoiceCleanerVariant(liveHardVoice, voiceVariantSpec{
				Family: "live_hard_voice",
				Name:   "voice_vosk_fast_plus_cleaner",
			}, proxy, runtime, evaluator, cfg, referenceTime),
		)
	}
	printSummaries(cfg, referenceTime, summaries)
	if cfg.emitReport {
		if cfg.reportDir == "" {
			cfg.reportDir = filepath.Join(cfg.cacheDir, "reports", time.Now().Format("20060102-150405"))
		}
		if err := writeReport(cfg, referenceTime, summaries); err != nil {
			log.Fatal(err)
		}
		fmt.Printf("\nReport: %s\n", cfg.reportDir)
	}
}

func loadCorpus(path string) (corpus, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return corpus{}, fmt.Errorf("read corpus: %w", err)
	}
	var suite corpus
	if err := json.Unmarshal(data, &suite); err != nil {
		return corpus{}, fmt.Errorf("parse corpus: %w", err)
	}
	return suite, nil
}

func validateCorpus(suite corpus) error {
	if strings.TrimSpace(suite.ReferenceTime) == "" {
		return fmt.Errorf("reference_time is required")
	}
	for _, tc := range suite.Text {
		if strings.TrimSpace(tc.ID) == "" {
			return fmt.Errorf("text case has empty id")
		}
		if strings.TrimSpace(tc.Input) == "" {
			return fmt.Errorf("text case %s has empty input", tc.ID)
		}
		if err := validateDifficulty(tc.Difficulty, tc.ID, "text"); err != nil {
			return err
		}
		if strings.TrimSpace(tc.Note) == "" {
			return fmt.Errorf("text case %s must include note/rationale", tc.ID)
		}
	}
	for _, vc := range suite.Voice {
		if strings.TrimSpace(vc.ID) == "" {
			return fmt.Errorf("voice case has empty id")
		}
		if strings.TrimSpace(vc.Dataset) == "" || strings.Contains(strings.ToLower(vc.Dataset), "synthetic") {
			return fmt.Errorf("voice case %s must declare a non-synthetic dataset", vc.ID)
		}
		if strings.TrimSpace(vc.SourceID) == "" || strings.TrimSpace(vc.SourcePage) == "" || strings.TrimSpace(vc.SHA256) == "" {
			return fmt.Errorf("voice case %s is missing public asset metadata", vc.ID)
		}
		if !hasVoiceAssetLocator(vc) {
			return fmt.Errorf("voice case %s must declare asset_path, download_url, or a supported dataset-backed source", vc.ID)
		}
		if strings.TrimSpace(vc.TranscriptRef) == "" {
			return fmt.Errorf("voice case %s is missing transcript_ref", vc.ID)
		}
		if err := validateDifficulty(vc.Difficulty, vc.ID, "voice"); err != nil {
			return err
		}
		if strings.TrimSpace(vc.Note) == "" {
			return fmt.Errorf("voice case %s must include note/rationale", vc.ID)
		}
	}
	textIndex := map[string]bool{}
	for _, tc := range suite.Text {
		textIndex[tc.ID] = true
	}
	for _, id := range suite.LiveHard.TextIDs {
		if !textIndex[id] {
			return fmt.Errorf("live_hard text id %s not found in text corpus", id)
		}
	}
	voiceIndex := map[string]bool{}
	for _, vc := range suite.Voice {
		voiceIndex[vc.ID] = true
	}
	for _, id := range suite.LiveHard.VoiceIDs {
		if !voiceIndex[id] {
			return fmt.Errorf("live_hard voice id %s not found in voice corpus", id)
		}
	}
	return nil
}

func validateDifficulty(value, id, family string) error {
	switch strings.TrimSpace(value) {
	case "medium", "hard":
		return nil
	case "":
		return fmt.Errorf("%s case %s is missing difficulty", family, id)
	default:
		return fmt.Errorf("%s case %s has unsupported difficulty %q", family, id, value)
	}
}

func hasVoiceAssetLocator(vc voiceCase) bool {
	if strings.TrimSpace(vc.AssetPath) != "" || strings.TrimSpace(vc.DownloadURL) != "" {
		return true
	}
	switch strings.TrimSpace(vc.Dataset) {
	case "golos_crowd_commands":
		return strings.TrimSpace(vc.SourceID) != ""
	default:
		return false
	}
}

func printSummaries(cfg runConfig, referenceTime time.Time, summaries []variantSummary) {
	fmt.Printf("Shelfy ingest benchmark\n")
	fmt.Printf("Reference time: %s\n", referenceTime.Format(time.RFC3339))
	fmt.Printf("Corpus: %s\n", cfg.corpusPath)
	fmt.Printf("Model: %s via %s\n\n", cfg.ollamaModel, cfg.ollamaBaseURL)
	fmt.Printf("%-8s %-30s %5s %5s %5s %8s %8s %8s %9s\n", "family", "variant", "total", "exact", "fail", "llm_txt", "applied", "helped", "avg_ms")
	for _, summary := range summaries {
		avgMs := 0.0
		if summary.Total > 0 {
			avgMs = float64(summary.Duration.Milliseconds()) / float64(summary.Total)
		}
		fmt.Printf("%-8s %-30s %5d %5d %5d %8d %8d %8d %9.1f\n",
			summary.Family,
			summary.Variant,
			summary.Total,
			summary.Exact,
			summary.Failed,
			summary.TextCalls,
			summary.CleanerApplied,
			summary.CleanerHelped,
			avgMs,
		)
	}
	for _, summary := range summaries {
		failures := collectFailures(summary.Cases)
		if len(failures) == 0 {
			continue
		}
		fmt.Printf("\n[%s / %s] failures: %d\n", summary.Family, summary.Variant, len(failures))
		for _, failure := range failures {
			fmt.Printf("- %s: %s\n", failure.ID, failure.FailureKind)
			if failure.WantSummary != "" {
				fmt.Printf("  want: %s\n", failure.WantSummary)
			}
			if failure.GotSummary != "" {
				fmt.Printf("  got:  %s\n", failure.GotSummary)
			}
			if failure.Note != "" {
				fmt.Printf("  note: %s\n", failure.Note)
			}
		}
	}
}

func collectFailures(cases []caseResult) []caseResult {
	out := make([]caseResult, 0, len(cases))
	for _, result := range cases {
		if !result.Exact {
			out = append(out, result)
		}
	}
	return out
}

func filterTextCases(cases []textCase, caseFilter string, tagFilter map[string]bool, limit int) []textCase {
	out := make([]textCase, 0, len(cases))
	for _, tc := range cases {
		if !matchesCase(tc.ID, tc.Tags, caseFilter, tagFilter) {
			continue
		}
		out = append(out, tc)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

func filterVoiceCases(cases []voiceCase, caseFilter string, tagFilter map[string]bool, limit int) []voiceCase {
	out := make([]voiceCase, 0, len(cases))
	for _, vc := range cases {
		if !matchesCase(vc.ID, vc.Tags, caseFilter, tagFilter) {
			continue
		}
		out = append(out, vc)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

func resolveLiveHardText(suite corpus) []textCase {
	if len(suite.LiveHard.TextIDs) == 0 {
		return nil
	}
	index := make(map[string]textCase, len(suite.Text))
	for _, tc := range suite.Text {
		index[tc.ID] = tc
	}
	out := make([]textCase, 0, len(suite.LiveHard.TextIDs))
	for _, id := range suite.LiveHard.TextIDs {
		if tc, ok := index[id]; ok {
			out = append(out, tc)
		}
	}
	return out
}

func resolveLiveHardVoice(suite corpus) []voiceCase {
	if len(suite.LiveHard.VoiceIDs) == 0 {
		return nil
	}
	index := make(map[string]voiceCase, len(suite.Voice))
	for _, vc := range suite.Voice {
		index[vc.ID] = vc
	}
	out := make([]voiceCase, 0, len(suite.LiveHard.VoiceIDs))
	for _, id := range suite.LiveHard.VoiceIDs {
		if vc, ok := index[id]; ok {
			out = append(out, vc)
		}
	}
	return out
}

func dedupeVoiceCases(cases []voiceCase) []voiceCase {
	if len(cases) == 0 {
		return nil
	}
	seen := map[string]bool{}
	out := make([]voiceCase, 0, len(cases))
	for _, vc := range cases {
		if seen[vc.ID] {
			continue
		}
		seen[vc.ID] = true
		out = append(out, vc)
	}
	return out
}

func matchesCase(id string, tags []string, caseFilter string, tagFilter map[string]bool) bool {
	if caseFilter != "" && !strings.Contains(strings.ToLower(id), caseFilter) {
		return false
	}
	if len(tagFilter) == 0 {
		return true
	}
	for _, tag := range tags {
		if tagFilter[strings.ToLower(strings.TrimSpace(tag))] {
			return true
		}
	}
	return false
}

func parseFilterSet(raw string) map[string]bool {
	out := map[string]bool{}
	for _, token := range strings.Split(raw, ",") {
		token = strings.ToLower(strings.TrimSpace(token))
		if token == "" {
			continue
		}
		out[token] = true
	}
	return out
}

func ensureBenchmarkRuntimeImages(cfg runConfig) error {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	required := map[string]bool{}
	if cfg.include["voice"] {
		if cfg.voskBinaryHostPath != "" {
			required[cfg.runtimeBaseImage] = true
		} else {
			required[cfg.pipelineWorkerImage] = true
		}
	}
	for image := range required {
		if err := ensureLocalDockerImage(ctx, image); err != nil {
			switch image {
			case cfg.runtimeBaseImage:
				return fmt.Errorf("%w\nbuild it with: make runtime-base", err)
			case cfg.pipelineWorkerImage:
				return fmt.Errorf("%w\nbuild it with: docker build -t %s .", err, image)
			default:
				return err
			}
		}
	}
	return nil
}

func resolvePath(root, value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	if filepath.IsAbs(value) {
		return value
	}
	return filepath.Join(root, value)
}

func resolveExistingPath(root, value string) string {
	resolved := resolvePath(root, value)
	if fileExists(resolved) {
		return resolved
	}
	return ""
}

func fileExists(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

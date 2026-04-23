package ingest

import (
	"context"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/igor/shelfy/internal/domain"
)

func TestHeuristicParseProductDateScenarios(t *testing.T) {
	now := time.Date(2026, time.April, 20, 10, 0, 0, 0, time.UTC)
	cases := []struct {
		name       string
		input      string
		wantName   string
		wantDate   string
		wantSource string
	}{
		{name: "weekday marker", input: "молоко до пятницы", wantName: "молоко", wantDate: "2026-04-24", wantSource: "heuristic_marker"},
		{name: "action prefix before marker", input: "нужно добавить молоко до пятницы", wantName: "молоко", wantDate: "2026-04-24", wantSource: "heuristic_marker"},
		{name: "short weekday marker", input: "молоко до пт", wantName: "молоко", wantDate: "2026-04-24", wantSource: "heuristic_marker"},
		{name: "tomorrow suffix", input: "зефир завтра", wantName: "зефир", wantDate: "2026-04-21", wantSource: "heuristic_suffix"},
		{name: "named month suffix", input: "молоко 1 мая", wantName: "молоко", wantDate: "2026-05-01", wantSource: "heuristic_suffix"},
		{name: "relative days suffix", input: "йогурт через 3 дня", wantName: "йогурт", wantDate: "2026-04-23", wantSource: "heuristic_suffix"},
		{name: "weekday typo marker", input: "ряженка до пятницаы", wantName: "ряженка", wantDate: "2026-04-24", wantSource: "heuristic_marker"},
		{name: "spoken ordinal day marker", input: "молоко до двадцать шестого", wantName: "молоко", wantDate: "2026-04-26", wantSource: "heuristic_marker"},
		{name: "spoken ordinal day month numeric marker", input: "молоко до десятого ноль третьего", wantName: "молоко", wantDate: "2027-03-10", wantSource: "heuristic_marker"},
		{name: "spoken ordinal day month numeric marker with noisy tail", input: "молоко до десятого ноль третьего двадцать шестого", wantName: "молоко", wantDate: "2027-03-10", wantSource: "heuristic_marker"},
		{name: "spoken ordinal day month words marker", input: "молоко до двадцать шестого апреля", wantName: "молоко", wantDate: "2026-04-26", wantSource: "heuristic_marker"},
		{name: "spoken ordinal day month words marker with repeated tail", input: "молоко до двадцать шестого апреля еще молоко до двадцать девятого апреля", wantName: "молоко", wantDate: "2026-04-26", wantSource: "heuristic_marker"},
		{name: "weekday typo suffix", input: "кефир субота", wantName: "кефир", wantDate: "2026-04-25", wantSource: "heuristic_suffix"},
		{name: "natural phrase via when", input: "сыр до следующей пятницы", wantName: "сыр", wantDate: "2026-04-24", wantSource: "heuristic_marker"},
		{name: "dative weekday", input: "творог к субботе", wantName: "творог", wantDate: "2026-04-25", wantSource: "heuristic_suffix"},
		{name: "numeric day suffix", input: "сметана 26", wantName: "сметана", wantDate: "2026-04-26", wantSource: "heuristic_suffix"},
		{name: "day dot month suffix", input: "соус песто 14.05", wantName: "соус песто", wantDate: "2026-05-14", wantSource: "heuristic_suffix"},
		{name: "action prefix with day dot month", input: "нужно купить курицу 26.04", wantName: "курица", wantDate: "2026-04-26", wantSource: "heuristic_suffix"},
		{name: "single word cheese stays product", input: "сыр", wantName: "сыр", wantSource: "heuristic_name_only"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := heuristicParse(tc.input, now)
			if got.Name != tc.wantName {
				t.Fatalf("expected name %q, got %q", tc.wantName, got.Name)
			}
			if tc.wantDate != "" && (got.ExpiresOn == nil || got.ExpiresOn.Format("2006-01-02") != tc.wantDate) {
				t.Fatalf("expected expiry %s, got %#v", tc.wantDate, got.ExpiresOn)
			}
			if tc.wantDate == "" && got.ExpiresOn != nil {
				t.Fatalf("expected no expiry, got %#v", got.ExpiresOn)
			}
			if got.Source != tc.wantSource {
				t.Fatalf("expected source %s, got %s", tc.wantSource, got.Source)
			}
		})
	}
}

func TestExtractNaturalDateFromText(t *testing.T) {
	now := time.Date(2026, time.April, 20, 10, 0, 0, 0, time.UTC)
	name, phrase, resolved, ok := extractNaturalDateFromText("зефир завтра", now)
	if !ok {
		t.Fatalf("expected extraction")
	}
	if name != "зефир" {
		t.Fatalf("expected name зефир, got %q", name)
	}
	if phrase != "завтра" {
		t.Fatalf("expected phrase завтра, got %q", phrase)
	}
	if resolved.Value == nil || resolved.Value.Format("2006-01-02") != "2026-04-21" {
		t.Fatalf("expected 2026-04-21, got %#v", resolved.Value)
	}
}

func TestExtractNaturalDateOnlyFromText(t *testing.T) {
	now := time.Date(2026, time.April, 20, 10, 0, 0, 0, time.UTC)
	phrase, resolved, ok := extractNaturalDateOnlyFromText("слушай надо до пятницы", now)
	if !ok {
		t.Fatalf("expected noisy date-only extraction")
	}
	if phrase != "до пятницы" {
		t.Fatalf("expected phrase %q, got %q", "до пятницы", phrase)
	}
	if resolved.Value == nil || resolved.Value.Format("2006-01-02") != "2026-04-24" {
		t.Fatalf("expected 2026-04-24, got %#v", resolved.Value)
	}
}

func TestShouldTryTextModelSkipsPlainProductName(t *testing.T) {
	if !looksLikePlainProductName("молоко") {
		t.Fatalf("expected plain product name to be detected")
	}
	if shouldTryTextModel("молоко", parsedDraft{Name: "молоко", Confidence: "low", Source: "heuristic_name_only"}) {
		t.Fatalf("expected plain product name to skip text model")
	}
}

func TestShouldTryTextModelKeepsModelFallbackForUnresolvedMixedText(t *testing.T) {
	if !shouldTryTextModel("очень странная фраза про молоко когда-нибудь", parsedDraft{Name: "очень странная фраза про молоко когда-нибудь", Confidence: "low", Source: "heuristic_name_only"}) {
		t.Fatalf("expected unresolved mixed text to keep model fallback")
	}
}

func TestNormalizeDraftNameStripsActionPrefix(t *testing.T) {
	if got := normalizeDraftName("нужно добавить молоко"); got != "молоко" {
		t.Fatalf("normalizeDraftName() = %q, want молоко", got)
	}
	if got := normalizeDraftName("добавить йогурт питьевой"); got != "йогурт питьевой" {
		t.Fatalf("normalizeDraftName() = %q, want йогурт питьевой", got)
	}
	if got := normalizeDraftName("колбасу черкашина из ленты"); got != "колбаса черкашина" {
		t.Fatalf("normalizeDraftName() = %q, want колбаса черкашина", got)
	}
	if got := normalizeDraftName("свежих бананов и"); got != "бананы" {
		t.Fatalf("normalizeDraftName() = %q, want бананы", got)
	}
	if got := normalizeDraftName("чистая линия ванильное мороженое с стаканчик триста грамм"); got != "мороженое чистая линия ванильное" {
		t.Fatalf("normalizeDraftName() = %q, want мороженое чистая линия ванильное", got)
	}
	if got := normalizeDraftName("сосиски родионов ские"); got != "сосиски родионовские" {
		t.Fatalf("normalizeDraftName() = %q, want сосиски родионовские", got)
	}
	if got := normalizeDraftName("зеленый рассыпной чай соус"); got != "зеленый рассыпной чай" {
		t.Fatalf("normalizeDraftName() = %q, want зеленый рассыпной чай", got)
	}
	if got := normalizeDraftName("привет можешь мне заказать ложкарев полкилограмма пельмени"); got != "пельмени ложкарев" {
		t.Fatalf("normalizeDraftName() = %q, want пельмени ложкарев", got)
	}
	if got := normalizeDraftName("липтон зеленый чай закажи пожалуйста"); got != "липтон зеленый чай" {
		t.Fatalf("normalizeDraftName() = %q, want липтон зеленый чай", got)
	}
	if got := normalizeDraftName("так мне нужно записать молоко"); got != "молоко" {
		t.Fatalf("normalizeDraftName() = %q, want молоко", got)
	}
	if got := normalizeDraftName("такс молоко"); got != "молоко" {
		t.Fatalf("normalizeDraftName() = %q, want молоко", got)
	}
	if got := normalizeDraftName("у меня тут бананы"); got != "бананы" {
		t.Fatalf("normalizeDraftName() = %q, want бананы", got)
	}
	if got := normalizeDraftName("меня хлеб срок годности"); got != "хлеб" {
		t.Fatalf("normalizeDraftName() = %q, want хлеб", got)
	}
}

func TestParseFastDraftVoiceRegressionCases(t *testing.T) {
	now := time.Date(2026, time.April, 20, 10, 0, 0, 0, time.UTC)

	draft, err := parseFastDraft("добрый вечер мне нужны пельмени лукович килограмм со вкусом сыра вот привезите пожалуйста через полчаса", now)
	if err != nil {
		t.Fatalf("parseFastDraft() unexpected error = %v", err)
	}
	if draft.Name != "пельмени лукович" || draft.ExpiresOn != nil {
		t.Fatalf("draft = %#v, want name-only пельмени лукович", draft)
	}

	if _, err := parseFastDraft("так мне нужны огурцы помидоры болгарский красный перец десяток яиц и молоко обезжиренное литр", now); err == nil {
		t.Fatal("expected multi-item input to be rejected")
	}

	voiceDate, err := parseFastDraft(normalizeVoiceTranscript("фарш до второго число"), now)
	if err != nil {
		t.Fatalf("parseFastDraft() on live date grammar error = %v", err)
	}
	if voiceDate.Name != "фарш" || voiceDate.ExpiresOn == nil || voiceDate.ExpiresOn.Format("2006-01-02") != "2026-05-02" {
		t.Fatalf("voiceDate = %#v, want фарш + 2026-05-02", voiceDate)
	}

	liveNoise, err := parseFastDraft(normalizeVoiceTranscript("ты экс бананы до завтра"), now)
	if err != nil {
		t.Fatalf("parseFastDraft() on live asr noise error = %v", err)
	}
	if liveNoise.Name != "бананы" || liveNoise.ExpiresOn == nil || liveNoise.ExpiresOn.Format("2006-01-02") != "2026-04-21" {
		t.Fatalf("liveNoise = %#v, want бананы + 2026-04-21", liveNoise)
	}
}

func TestValidateResolvedDeadlinePhraseRejectsFalseDateWords(t *testing.T) {
	now := time.Date(2026, time.April, 20, 10, 0, 0, 0, time.UTC)
	for _, input := range []string{"завтрак", "десяток"} {
		t.Run(input, func(t *testing.T) {
			phrase, resolved := validateResolvedDeadlinePhrase(input, domain.ResolveRelativeDate(input, now), now)
			if phrase != "" || resolved.Value != nil {
				t.Fatalf("validateResolvedDeadlinePhrase(%q) = (%q, %#v), want empty invalid phrase", input, phrase, resolved.Value)
			}
		})
	}
}

func TestRunVoskUsesConfiguredModelPathAndGrammar(t *testing.T) {
	dir := t.TempDir()
	argsPath := filepath.Join(dir, "args.txt")
	sourcePath := filepath.Join(dir, "main.go")
	binaryPath := filepath.Join(dir, "vosk-mock")
	source := `package main

import (
	"os"
	"strings"
)

func main() {
	_ = os.WriteFile(os.Getenv("SHELFY_HELPER_ARGS_PATH"), []byte(strings.Join(os.Args[1:], "|")), 0o644)
	_, _ = os.Stdout.WriteString("сметана завтра\n")
}
`
	if err := os.WriteFile(sourcePath, []byte(source), 0o644); err != nil {
		t.Fatalf("write helper source: %v", err)
	}
	build := exec.Command("go", "build", "-o", binaryPath, sourcePath)
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build helper: %v: %s", err, string(output))
	}

	service := &Service{
		logger:          slog.New(slog.NewTextHandler(io.Discard, nil)),
		voskCommand:     binaryPath,
		voskModelPath:   "/models/vosk-model-small-ru-0.22",
		voskGrammarPath: "/app/assets/asr/vosk-grammar.ru.json",
	}
	wavPath := filepath.Join(dir, "input.wav")
	if err := os.WriteFile(wavPath, []byte("wav"), 0o644); err != nil {
		t.Fatalf("write wav: %v", err)
	}

	t.Setenv("SHELFY_HELPER_ARGS_PATH", argsPath)

	text, err := service.runVosk(context.Background(), wavPath)
	if err != nil {
		t.Fatalf("runVosk error = %v", err)
	}
	if text != "сметана завтра" {
		t.Fatalf("expected transcript, got %q", text)
	}

	argsRaw, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatalf("read args: %v", err)
	}
	args := string(argsRaw)
	if !strings.Contains(args, "/models/vosk-model-small-ru-0.22") {
		t.Fatalf("expected model path in args, got %q", args)
	}
	if !strings.Contains(args, wavPath) {
		t.Fatalf("expected wav path in args, got %q", args)
	}
	if !strings.Contains(args, "/app/assets/asr/vosk-grammar.ru.json") {
		t.Fatalf("expected grammar path in args, got %q", args)
	}
}

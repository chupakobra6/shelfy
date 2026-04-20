package ingest

import (
	"testing"
	"time"
)

func TestCleanOCRTextDropsTesseractDiagnostics(t *testing.T) {
	input := `
Error opening data file /usr/share/tesseract-ocr/5/tessdata/rus.traineddata
Please make sure the TESSDATA_PREFIX environment variable is set
Молоко ультрапастеризованное
годен до 21.04.2026
Estimating resolution as 344
`
	got := cleanOCRText(input)
	want := "Молоко ультрапастеризованное\nгоден до 21.04.2026"
	if got != want {
		t.Fatalf("unexpected cleaned OCR text:\nwant: %q\ngot:  %q", want, got)
	}
}

func TestShouldUseOCRHeuristicFallbackRejectsDiagnosticText(t *testing.T) {
	current := parsedDraft{}
	if shouldUseOCRHeuristicFallback("Error opening data file /usr/share/tesseract-ocr/5/tessdata/rus.traineddata", current) {
		t.Fatalf("expected diagnostic OCR text to be rejected")
	}
}

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
		{name: "short weekday marker", input: "молоко до пт", wantName: "молоко", wantDate: "2026-04-24", wantSource: "heuristic_marker"},
		{name: "tomorrow suffix", input: "зефир завтра", wantName: "зефир", wantDate: "2026-04-21", wantSource: "heuristic_suffix"},
		{name: "named month suffix", input: "молоко 1 мая", wantName: "молоко", wantDate: "2026-05-01", wantSource: "heuristic_suffix"},
		{name: "relative days suffix", input: "йогурт через 3 дня", wantName: "йогурт", wantDate: "2026-04-23", wantSource: "heuristic_suffix"},
		{name: "weekday typo marker", input: "ряженка до пятницаы", wantName: "ряженка", wantDate: "2026-04-24", wantSource: "heuristic_marker"},
		{name: "weekday typo suffix", input: "кефир субота", wantName: "кефир", wantDate: "2026-04-25", wantSource: "heuristic_suffix"},
		{name: "natural phrase via when", input: "сыр до следующей пятницы", wantName: "сыр", wantDate: "2026-04-24", wantSource: "heuristic_marker"},
		{name: "dative weekday", input: "творог к субботе", wantName: "творог", wantDate: "2026-04-25", wantSource: "heuristic_suffix"},
		{name: "numeric day suffix", input: "сметана 26", wantName: "сметана", wantDate: "2026-04-26", wantSource: "heuristic_suffix"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := heuristicParse(tc.input, now)
			if got.Name != tc.wantName {
				t.Fatalf("expected name %q, got %q", tc.wantName, got.Name)
			}
			if got.ExpiresOn == nil || got.ExpiresOn.Format("2006-01-02") != tc.wantDate {
				t.Fatalf("expected expiry %s, got %#v", tc.wantDate, got.ExpiresOn)
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

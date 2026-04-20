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

func TestHeuristicParseRussianTextStillWorks(t *testing.T) {
	now := time.Date(2026, time.April, 20, 10, 0, 0, 0, time.UTC)
	got := heuristicParse("молоко до пятницы", now)
	if got.Name != "молоко" {
		t.Fatalf("expected name молоко, got %q", got.Name)
	}
	if got.ExpiresOn == nil || got.ExpiresOn.Format("2006-01-02") != "2026-04-24" {
		t.Fatalf("expected expiry 2026-04-24, got %#v", got.ExpiresOn)
	}
}

func TestHeuristicParseShortRussianWeekday(t *testing.T) {
	now := time.Date(2026, time.April, 20, 10, 0, 0, 0, time.UTC)
	got := heuristicParse("молоко до пт", now)
	if got.Name != "молоко" {
		t.Fatalf("expected name молоко, got %q", got.Name)
	}
	if got.ExpiresOn == nil || got.ExpiresOn.Format("2006-01-02") != "2026-04-24" {
		t.Fatalf("expected expiry 2026-04-24, got %#v", got.ExpiresOn)
	}
}

func TestHeuristicParseTomorrowSuffix(t *testing.T) {
	now := time.Date(2026, time.April, 20, 10, 0, 0, 0, time.UTC)
	got := heuristicParse("зефир завтра", now)
	if got.Name != "зефир" {
		t.Fatalf("expected name зефир, got %q", got.Name)
	}
	if got.ExpiresOn == nil || got.ExpiresOn.Format("2006-01-02") != "2026-04-21" {
		t.Fatalf("expected expiry 2026-04-21, got %#v", got.ExpiresOn)
	}
}

func TestHeuristicParseTrailingDatePhrase(t *testing.T) {
	now := time.Date(2026, time.April, 20, 10, 0, 0, 0, time.UTC)
	got := heuristicParse("молоко 1 мая", now)
	if got.Name != "молоко" {
		t.Fatalf("expected name молоко, got %q", got.Name)
	}
	if got.ExpiresOn == nil || got.ExpiresOn.Format("2006-01-02") != "2026-05-01" {
		t.Fatalf("expected expiry 2026-05-01, got %#v", got.ExpiresOn)
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

package domain

import (
	"testing"
	"time"
)

func TestResolveRelativeDateCommonRussianCases(t *testing.T) {
	now := time.Date(2026, time.April, 20, 10, 0, 0, 0, time.UTC)
	cases := []struct {
		name       string
		input      string
		want       string
		confidence string
	}{
		{name: "today", input: "сегодня", want: "2026-04-20", confidence: "high"},
		{name: "tomorrow", input: "завтра", want: "2026-04-21", confidence: "high"},
		{name: "day after tomorrow", input: "послезавтра", want: "2026-04-22", confidence: "high"},
		{name: "weekday with prefix", input: "до пятницы", want: "2026-04-24", confidence: "medium"},
		{name: "short weekday", input: "до пт", want: "2026-04-24", confidence: "medium"},
		{name: "dative weekday", input: "к субботе", want: "2026-04-25", confidence: "medium"},
		{name: "weekday with в", input: "в субботу", want: "2026-04-25", confidence: "medium"},
		{name: "short saturday", input: "сб", want: "2026-04-25", confidence: "medium"},
		{name: "named saturday", input: "суббота", want: "2026-04-25", confidence: "medium"},
		{name: "named month", input: "1 мая", want: "2026-05-01", confidence: "medium"},
		{name: "spoken ordinal day", input: "двадцать шестого", want: "2026-04-26", confidence: "medium"},
		{name: "spoken ordinal day month words", input: "двадцать шестого апреля", want: "2026-04-26", confidence: "medium"},
		{name: "spoken ordinal day month numeric words", input: "десятого ноль третьего", want: "2027-03-10", confidence: "medium"},
		{name: "day only", input: "26", want: "2026-04-26", confidence: "medium"},
		{name: "relative days", input: "через 3 дня", want: "2026-04-23", confidence: "high"},
		{name: "relative week", input: "через неделю", want: "2026-04-27", confidence: "high"},
		{name: "relative weeks", input: "через 2 недели", want: "2026-05-04", confidence: "high"},
		{name: "relative month", input: "через месяц", want: "2026-05-20", confidence: "high"},
		{name: "relative months", input: "через 2 месяца", want: "2026-06-20", confidence: "high"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resolved := ResolveRelativeDate(tc.input, now)
			if resolved.Value == nil {
				t.Fatalf("expected resolved date for %q", tc.input)
			}
			if got := resolved.Value.Format("2006-01-02"); got != tc.want {
				t.Fatalf("expected %s, got %s", tc.want, got)
			}
			if resolved.Confidence != tc.confidence {
				t.Fatalf("expected confidence %s, got %s", tc.confidence, resolved.Confidence)
			}
		})
	}
}

func TestResolveRelativeDateFuturePolicy(t *testing.T) {
	now := time.Date(2026, time.April, 20, 10, 0, 0, 0, time.UTC)
	resolved := ResolveRelativeDate("14.04", now)
	if resolved.Value == nil {
		t.Fatalf("expected resolved date")
	}
	if got := resolved.Value.Format("2006-01-02"); got != "2027-04-14" {
		t.Fatalf("expected 2027-04-14, got %s", got)
	}
}

func TestResolveRelativeDateStrictStampFormats(t *testing.T) {
	now := time.Date(2026, time.April, 20, 10, 0, 0, 0, time.UTC)
	cases := map[string]string{
		"2021.08.03":           "2021-08-03",
		"2021-08-03":           "2021-08-03",
		"2021/08/03":           "2021-08-03",
		"2021 08 03":           "2021-08-03",
		"21-08-03":             "2021-08-03",
		"03.08.21":             "2021-08-03",
		"03 08 2021":           "2021-08-03",
		"03/AUG/2021":          "2021-08-03",
		"Aug 03 2021":          "2021-08-03",
		"best before 11/20/21": "2021-11-20",
		"exp 10 06 21":         "2021-06-10",
		"used by jun 28 2021":  "2021-06-28",
		"21.03.16 a":           "2016-03-21",
	}

	for input, want := range cases {
		t.Run(input, func(t *testing.T) {
			resolved := ResolveRelativeDate(input, now)
			if resolved.Value == nil {
				t.Fatalf("expected resolved date for %q", input)
			}
			if got := resolved.Value.Format("2006-01-02"); got != want {
				t.Fatalf("expected %s, got %s", want, got)
			}
			if !resolved.Absolute {
				t.Fatalf("expected %q to be marked as absolute", input)
			}
		})
	}
}

func TestResolveRelativeDateTypoTolerance(t *testing.T) {
	now := time.Date(2026, time.April, 20, 10, 0, 0, 0, time.UTC)
	cases := map[string]string{
		"до пятницаы":  "2026-04-24",
		"субота":       "2026-04-25",
		"до субота":    "2026-04-25",
		"позавтра":     "2026-04-22",
		"через 3 днеи": "2026-04-23",
	}
	for input, want := range cases {
		t.Run(input, func(t *testing.T) {
			resolved := ResolveRelativeDate(input, now)
			if resolved.Value == nil {
				t.Fatalf("expected resolved date for %q", input)
			}
			if got := resolved.Value.Format("2006-01-02"); got != want {
				t.Fatalf("expected %s, got %s", want, got)
			}
		})
	}
}

func TestResolveRelativeDateUnknownCases(t *testing.T) {
	now := time.Date(2026, time.April, 20, 10, 0, 0, 0, time.UTC)
	cases := []struct {
		input      string
		confidence string
	}{
		{input: "", confidence: "missing"},
		{input: "молоко", confidence: "unknown"},
		{input: "сыр", confidence: "unknown"},
		{input: "32.13", confidence: "unknown"},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			resolved := ResolveRelativeDate(tc.input, now)
			if resolved.Value != nil {
				t.Fatalf("expected nil for %q, got %s", tc.input, resolved.Value.Format(time.RFC3339))
			}
			if resolved.Confidence != tc.confidence {
				t.Fatalf("expected confidence %s, got %s", tc.confidence, resolved.Confidence)
			}
		})
	}
}

func TestExtractDateFromTextRussianPhrases(t *testing.T) {
	now := time.Date(2026, time.April, 20, 10, 0, 0, 0, time.UTC)
	cases := []struct {
		input      string
		wantPhrase string
		wantDate   string
	}{
		{input: "молоко до пятницы", wantPhrase: "до пятницы", wantDate: "2026-04-24"},
		{input: "молоко до двадцать шестого", wantPhrase: "до двадцать шестого", wantDate: "2026-04-26"},
		{input: "молоко до десятого ноль третьего", wantPhrase: "до десятого ноль третьего", wantDate: "2027-03-10"},
		{input: "молоко до десятого ноль третьего двадцать шестого", wantPhrase: "до десятого ноль третьего", wantDate: "2027-03-10"},
		{input: "молоко до двадцать шестого апреля", wantPhrase: "до двадцать шестого апреля", wantDate: "2026-04-26"},
		{input: "молоко до двадцать шестого апреля двадцать девятого апреля", wantPhrase: "до двадцать шестого апреля", wantDate: "2026-04-26"},
		{input: "молоко до пт", wantPhrase: "до пт", wantDate: "2026-04-24"},
		{input: "зефир завтра", wantPhrase: "завтра", wantDate: "2026-04-21"},
		{input: "сыр до следующей пятницы", wantPhrase: "до следующей пятницы", wantDate: "2026-04-24"},
		{input: "йогурт к субботе", wantPhrase: "к субботе", wantDate: "2026-04-25"},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			extracted, ok := ExtractDateFromText(tc.input, now)
			if !ok || extracted.Value == nil {
				t.Fatalf("expected extracted date for %q", tc.input)
			}
			if extracted.Phrase != tc.wantPhrase {
				t.Fatalf("expected phrase %q, got %q", tc.wantPhrase, extracted.Phrase)
			}
			if got := extracted.Value.Format("2006-01-02"); got != tc.wantDate {
				t.Fatalf("expected date %s, got %s", tc.wantDate, got)
			}
		})
	}
}

func TestNormalizeDateInput(t *testing.T) {
	cases := map[string]string{
		"  До   ПЯТНИЦАЫ!!! ": "до пятницы",
		"субота":              "суббота",
		"в СУББОТУ":           "в субботу",
		"через 3 днеи":        "через 3 дней",
	}
	for input, want := range cases {
		t.Run(input, func(t *testing.T) {
			if got := normalizeDateInput(input); got != want {
				t.Fatalf("expected %q, got %q", want, got)
			}
		})
	}
}

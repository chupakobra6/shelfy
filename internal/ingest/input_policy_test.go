package ingest

import (
	"testing"
	"time"
)

func TestNormalizeDraftNameStripsFillerPrefixes(t *testing.T) {
	cases := map[string]string{
		"слушай надо молоко":                               "молоко",
		"вообще надо взять хлеб":                           "хлеб",
		"короче бананы":                                    "бананы",
		"пожалуйста добавить сыр":                          "сыр",
		"слушай алиса закажи мне один килограмм помидоров": "помидоры",
	}
	for input, want := range cases {
		t.Run(input, func(t *testing.T) {
			if got := normalizeDraftName(input); got != want {
				t.Fatalf("normalizeDraftName(%q) = %q, want %q", input, got, want)
			}
		})
	}
}

func TestApplyTextIntentGateRejectsNonFoodAndMultiItem(t *testing.T) {
	cases := []struct {
		name  string
		input string
		draft parsedDraft
		want  string
	}{
		{
			name:  "non food reminder",
			input: "напомни про встречу завтра",
			draft: parsedDraft{Name: "напомни", ExpiresOn: datePtr("2026-04-21")},
			want:  "non_food",
		},
		{
			name:  "non product todo",
			input: "список на неделю и убрать кухню",
			draft: parsedDraft{Name: "список на неделю и убрать кухню"},
			want:  "non_food",
		},
		{
			name:  "multi item",
			input: "молоко и кефир",
			draft: parsedDraft{Name: "молоко и кефир"},
			want:  "multi_item",
		},
		{
			name:  "multi item with quantity noise",
			input: "закажи два пакета молока по одному литру один пакет кефира все с доставкой на дом",
			draft: parsedDraft{Name: "молока кефира"},
			want:  "multi_item",
		},
		{
			name:  "unknown single word kept",
			input: "млако",
			draft: parsedDraft{Name: "млако"},
			want:  "",
		},
		{
			name:  "date only partial allowed",
			input: "слушай надо до пятницы",
			draft: parsedDraft{ExpiresOn: datePtr("2026-04-24")},
			want:  "",
		},
		{
			name:  "single branded product with quantity noise kept",
			input: "закажи на дом молоко простоквашино два с половиной литра и жирностью один процент",
			draft: parsedDraft{Name: "молоко простоквашино"},
			want:  "",
		},
		{
			name:  "single product flavor tail kept",
			input: "добрый вечер мне нужны пельмени лукович килограмм со вкусом сыра вот привезите пожалуйста через полчаса",
			draft: parsedDraft{Name: "пельмени лукович"},
			want:  "",
		},
		{
			name:  "pet food replacement request rejected despite food words",
			input: "корм китикет нужен два пакетика пюре",
			draft: parsedDraft{Name: "корм китикет нужен"},
			want:  "non_food",
		},
		{
			name:  "date only multi item is still rejected",
			input: "так мне нужны огурцы помидоры болгарский красный перец десяток яиц и молоко обезжиренное литр",
			draft: parsedDraft{ExpiresOn: datePtr("2027-01-10")},
			want:  "multi_item",
		},
		{
			name:  "ingredient stack dessert request rejected",
			input: "закажи мне мороженое филевское пломбир с вишней и кусочками миндаля и с корицей ведро пятьсот пятьдесят грамм",
			draft: parsedDraft{Name: "мороженое филевское пломбир"},
			want:  "multi_item",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, got := applyTextIntentGate(tc.input, tc.draft)
			if got != tc.want {
				t.Fatalf("applyTextIntentGate(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestNormalizeVoiceTranscriptAndRepairGate(t *testing.T) {
	now := time.Date(2026, time.April, 20, 10, 0, 0, 0, time.UTC)

	if got := normalizeVoiceTranscript("млако до пятницы"); got != "молоко до пятницы" {
		t.Fatalf("normalizeVoiceTranscript corrected = %q, want %q", got, "молоко до пятницы")
	}
	if got := normalizeVoiceTranscript("молоко он то двадцать шестого апреля"); got != "молоко до двадцать шестого апреля" {
		t.Fatalf("normalizeVoiceTranscript repaired filler = %q, want %q", got, "молоко до двадцать шестого апреля")
	}
	if got := normalizeVoiceTranscript("фарш до второго число"); got != "фарш до второго числа" {
		t.Fatalf("normalizeVoiceTranscript repaired spoken date grammar = %q, want %q", got, "фарш до второго числа")
	}
	if got := normalizeVoiceTranscript("ты экс бананы до завтра"); got != "бананы до завтра" {
		t.Fatalf("normalizeVoiceTranscript removed live asr noise = %q, want %q", got, "бананы до завтра")
	}
	if got := normalizeVoiceTranscript("такс молоко до двадцать девятого"); got != "так молоко до двадцать девятого" {
		t.Fatalf("normalizeVoiceTranscript normalized spoken filler = %q, want %q", got, "так молоко до двадцать девятого")
	}
	if got := normalizeVoiceTranscript("у меня тут бананы до послезавтра"); got != "у меня тут бананы до послезавтра" {
		t.Fatalf("normalizeVoiceTranscript should preserve semantic words before name normalization = %q", got)
	}
	if got := normalizeVoiceTranscript("молоко он то двадцать шестого апреля еще молоко до двадцать девятого апреля"); got != "молоко до двадцать шестого апреля еще молоко до двадцать девятого апреля" {
		t.Fatalf("normalizeVoiceTranscript repaired repeated filler = %q, want %q", got, "молоко до двадцать шестого апреля еще молоко до двадцать девятого апреля")
	}
	if shouldRepairVoiceTranscript("молоко до пятницы", now) {
		t.Fatalf("expected clean grocery transcript to skip repair")
	}
	if !shouldRepairVoiceTranscript("milk x z", now) {
		t.Fatalf("expected latin noise transcript to require repair")
	}
	if !shouldRepairVoiceTranscript("напомни встречу", now) {
		t.Fatalf("expected non-food noisy transcript to require repair")
	}
}

func datePtr(raw string) *time.Time {
	value, err := time.Parse("2006-01-02", raw)
	if err != nil {
		panic(err)
	}
	return &value
}

package ingest

import "testing"

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

func TestNormalizeVoiceTranscript(t *testing.T) {
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
}

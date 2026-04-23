package ingest

func NormalizeFreeTextForBenchmark(input string) string {
	return normalizeFreeText(input)
}

func NormalizeVoiceTranscriptForBenchmark(input string) string {
	return normalizeVoiceTranscript(input)
}

func NormalizeDraftNameForBenchmark(input string) string {
	return normalizeDraftName(input)
}

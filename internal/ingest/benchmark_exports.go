package ingest

import "time"

func NormalizeFreeTextForBenchmark(input string) string {
	return normalizeFreeText(input)
}

func NormalizeVoiceTranscriptForBenchmark(input string) string {
	return normalizeVoiceTranscript(input)
}

func NormalizeDraftNameForBenchmark(input string) string {
	return normalizeDraftName(input)
}

func ShouldRepairVoiceTranscriptForBenchmark(input string, now time.Time) bool {
	return shouldRepairVoiceTranscript(input, now)
}

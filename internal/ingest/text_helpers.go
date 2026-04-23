package ingest

import "unicode"

func tokenHasLetters(value string) bool {
	for _, r := range value {
		if unicode.IsLetter(r) {
			return true
		}
	}
	return false
}

func letterRuneCount(value string) int {
	count := 0
	for _, r := range value {
		if unicode.IsLetter(r) {
			count++
		}
	}
	return count
}

func isLatinOnlyName(value string) bool {
	if value == "" || !tokenHasLetters(value) {
		return false
	}
	hasLatin := false
	for _, r := range value {
		switch {
		case !unicode.IsLetter(r):
			continue
		case unicode.In(r, unicode.Latin):
			hasLatin = true
		default:
			return false
		}
	}
	return hasLatin
}

func int64Ptr(value int64) *int64 {
	if value == 0 {
		return nil
	}
	return &value
}

func ptrValue(value *int64) int64 {
	if value == nil {
		return 0
	}
	return *value
}

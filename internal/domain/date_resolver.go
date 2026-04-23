package domain

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/olebedev/when"
	"github.com/olebedev/when/rules/ru"
)

type ResolvedDate struct {
	Value      *time.Time
	Confidence string
	Absolute   bool
}

type ExtractedDate struct {
	Phrase     string
	Value      *time.Time
	Confidence string
	Source     string
	Absolute   bool
}

var weekdayMap = map[string]time.Weekday{
	"monday":       time.Monday,
	"понедельник":  time.Monday,
	"понедельника": time.Monday,
	"понедельнику": time.Monday,
	"пн":           time.Monday,
	"tuesday":      time.Tuesday,
	"вторник":      time.Tuesday,
	"вторника":     time.Tuesday,
	"вторнику":     time.Tuesday,
	"вт":           time.Tuesday,
	"wednesday":    time.Wednesday,
	"среда":        time.Wednesday,
	"среды":        time.Wednesday,
	"среду":        time.Wednesday,
	"среде":        time.Wednesday,
	"ср":           time.Wednesday,
	"thursday":     time.Thursday,
	"четверг":      time.Thursday,
	"четверга":     time.Thursday,
	"четвергу":     time.Thursday,
	"чт":           time.Thursday,
	"friday":       time.Friday,
	"пятница":      time.Friday,
	"пятницы":      time.Friday,
	"пятницу":      time.Friday,
	"пятнице":      time.Friday,
	"пт":           time.Friday,
	"saturday":     time.Saturday,
	"суббота":      time.Saturday,
	"субботы":      time.Saturday,
	"субботу":      time.Saturday,
	"субботе":      time.Saturday,
	"сб":           time.Saturday,
	"sunday":       time.Sunday,
	"воскресенье":  time.Sunday,
	"воскресенья":  time.Sunday,
	"воскресенью":  time.Sunday,
	"вс":           time.Sunday,
}

var monthMap = map[string]time.Month{
	"jan":      time.January,
	"янв":      time.January,
	"январь":   time.January,
	"января":   time.January,
	"feb":      time.February,
	"фев":      time.February,
	"февраль":  time.February,
	"февраля":  time.February,
	"mar":      time.March,
	"мар":      time.March,
	"март":     time.March,
	"марта":    time.March,
	"apr":      time.April,
	"апр":      time.April,
	"апрель":   time.April,
	"апреля":   time.April,
	"may":      time.May,
	"май":      time.May,
	"мая":      time.May,
	"jun":      time.June,
	"июн":      time.June,
	"июнь":     time.June,
	"июня":     time.June,
	"jul":      time.July,
	"июл":      time.July,
	"июль":     time.July,
	"июля":     time.July,
	"aug":      time.August,
	"авг":      time.August,
	"август":   time.August,
	"августа":  time.August,
	"sep":      time.September,
	"сен":      time.September,
	"сент":     time.September,
	"сентябрь": time.September,
	"сентября": time.September,
	"oct":      time.October,
	"окт":      time.October,
	"октябрь":  time.October,
	"октября":  time.October,
	"nov":      time.November,
	"ноя":      time.November,
	"ноябрь":   time.November,
	"ноября":   time.November,
	"dec":      time.December,
	"дек":      time.December,
	"декабрь":  time.December,
	"декабря":  time.December,
}

var spokenOrdinalDayWords = map[string]int{
	"первого":        1,
	"второго":        2,
	"третьего":       3,
	"четвертого":     4,
	"пятого":         5,
	"шестого":        6,
	"седьмого":       7,
	"восьмого":       8,
	"девятого":       9,
	"десятого":       10,
	"одиннадцатого":  11,
	"двенадцатого":   12,
	"тринадцатого":   13,
	"четырнадцатого": 14,
	"пятнадцатого":   15,
	"шестнадцатого":  16,
	"семнадцатого":   17,
	"восемнадцатого": 18,
	"девятнадцатого": 19,
	"двадцатого":     20,
	"тридцатого":     30,
}

var spokenOrdinalUnitWords = map[string]int{
	"первого":    1,
	"второго":    2,
	"третьего":   3,
	"четвертого": 4,
	"пятого":     5,
	"шестого":    6,
	"седьмого":   7,
	"восьмого":   8,
	"девятого":   9,
}

var spokenOrdinalTensWords = map[string]int{
	"двадцать": 20,
	"тридцать": 30,
}

var (
	yearFirstDatePattern   = regexp.MustCompile(`^(\d{4})(?:[./-]|\s+)(\d{1,2})(?:[./-]|\s+)(\d{1,2})$`)
	shortYearDashPattern   = regexp.MustCompile(`^(\d{2})-(\d{1,2})-(\d{1,2})$`)
	usSlashDatePattern     = regexp.MustCompile(`^(\d{1,2})/(\d{1,2})/(\d{2,4})$`)
	absoluteDatePattern    = regexp.MustCompile(`^(\d{1,2})(?:[./-]|\s+)(\d{1,2})(?:(?:[./-]|\s+)(\d{2,4}))?$`)
	namedMonthPattern      = regexp.MustCompile(`^(\d{1,2})(?:\s+|/)([\p{L}]+)(?:(?:\s+|/)(\d{2,4}))?$`)
	monthFirstNamedPattern = regexp.MustCompile(`^([\p{L}]+)\s+(\d{1,2})(?:\s+(\d{2,4}))?$`)
	numericDayPattern      = regexp.MustCompile(`^\d{1,2}$`)
	relativeUnitsPattern   = regexp.MustCompile(`^(?:через\s+)?(\d{1,2})\s+(день|дня|днеи|дней|неделю|недели|недель|месяц|месяца|месяцев)$`)
	singleRelativePattern  = regexp.MustCompile(`^через\s+(неделю|месяц)$`)
	whenOnce               sync.Once
	whenParser             *when.Parser
	whenInitErr            error
)

var dateVocabulary = []string{
	"сегодня", "завтра", "послезавтра", "позавтра",
	"через", "день", "дня", "днеи", "дней", "неделю", "недели", "недель", "месяц", "месяца", "месяцев",
	"до", "к", "на", "в", "во", "by",
	"следующий", "следующую", "следующей", "следующее",
	"ноль", "двадцать", "тридцать",
	"первого", "второго", "третьего", "четвертого", "пятого", "шестого", "седьмого", "восьмого", "девятого", "десятого",
	"одиннадцатого", "двенадцатого", "тринадцатого", "четырнадцатого", "пятнадцатого",
	"шестнадцатого", "семнадцатого", "восемнадцатого", "девятнадцатого", "двадцатого", "тридцатого",
	"понедельник", "понедельника", "понедельнику", "пн",
	"вторник", "вторника", "вторнику", "вт",
	"среда", "среды", "среду", "среде", "ср",
	"четверг", "четверга", "четвергу", "чт",
	"пятница", "пятницы", "пятницу", "пятнице", "пт",
	"суббота", "субботы", "субботу", "субботе", "сб",
	"воскресенье", "воскресенья", "воскресенью", "вс",
	"jan", "feb", "mar", "apr", "may", "jun", "jul", "aug", "sep", "oct", "nov", "dec",
	"янв", "январь", "января", "фев", "февраль", "февраля",
	"мар", "март", "марта", "апр", "апрель", "апреля",
	"май", "мая", "июн", "июнь", "июня",
	"июл", "июль", "июля", "авг", "август", "августа",
	"сен", "сент", "сентябрь", "сентября",
	"окт", "октябрь", "октября", "ноя", "ноябрь", "ноября",
	"дек", "декабрь", "декабря",
}

var dateTokenOverrides = map[string]string{
	"пятницаы":   "пятницы",
	"субота":     "суббота",
	"суботы":     "субботы",
	"суботу":     "субботу",
	"днеи":       "дней",
	"двацадить":  "двадцать",
	"двадцадить": "двадцать",
	"тридцадого": "тридцатого",
	"шистого":    "шестого",
}

var datePhraseLeadingTokens = map[string]struct{}{
	"до":        {},
	"к":         {},
	"на":        {},
	"в":         {},
	"во":        {},
	"by":        {},
	"следующий": {},
	"следующую": {},
	"следующей": {},
	"следующее": {},
}

var strictDateLeadingPhrases = []string{
	"best before",
	"use by",
	"used by",
	"sell by",
	"exp date",
	"exp",
	"expires",
	"expiry",
	"before",
	"годен до",
	"срок годности",
	"срок годен до",
}

func ResolveRelativeDate(raw string, now time.Time) ResolvedDate {
	raw = normalizeDateInput(raw)
	if raw == "" {
		return ResolvedDate{Confidence: "missing"}
	}
	if resolved, handled := resolveStrictDate(raw, now); handled {
		return resolved
	}
	if extracted, ok := extractDateWithWhen(raw, now); ok {
		return ResolvedDate{Value: extracted.Value, Confidence: extracted.Confidence, Absolute: extracted.Absolute}
	}
	return ResolvedDate{Confidence: "unknown"}
}

func ExtractDateFromText(raw string, now time.Time) (ExtractedDate, bool) {
	raw = normalizeDateInput(raw)
	if raw == "" {
		return ExtractedDate{}, false
	}
	if resolved, handled := resolveStrictDate(raw, now); handled && resolved.Value != nil {
		return ExtractedDate{
			Phrase:     raw,
			Value:      resolved.Value,
			Confidence: resolved.Confidence,
			Source:     "strict",
			Absolute:   resolved.Absolute,
		}, true
	}
	if extracted, ok := extractDateWithWhen(raw, now); ok {
		return extracted, true
	}
	return extractStrictDatePhrase(raw, now)
}

func resolveStrictDate(raw string, now time.Time) (ResolvedDate, bool) {
	raw = trimStrictDateDecorators(raw)
	switch raw {
	case "today", "сегодня":
		value := truncateToDate(now)
		return ResolvedDate{Value: &value, Confidence: "high"}, true
	case "tomorrow", "завтра":
		value := truncateToDate(now).AddDate(0, 0, 1)
		return ResolvedDate{Value: &value, Confidence: "high"}, true
	case "послезавтра", "позавтра":
		value := truncateToDate(now).AddDate(0, 0, 2)
		return ResolvedDate{Value: &value, Confidence: "high"}, true
	}
	if matches := yearFirstDatePattern.FindStringSubmatch(raw); len(matches) == 4 {
		month, day := mustInt(matches[2]), mustInt(matches[3])
		return resolveCalendarDate(day, time.Month(month), matches[1], now, true), true
	}
	if matches := shortYearDashPattern.FindStringSubmatch(raw); len(matches) == 4 {
		month, day := mustInt(matches[2]), mustInt(matches[3])
		return resolveCalendarDate(day, time.Month(month), matches[1], now, true), true
	}
	if matches := usSlashDatePattern.FindStringSubmatch(raw); len(matches) == 4 {
		month, day := mustInt(matches[1]), mustInt(matches[2])
		if day > 12 {
			return resolveCalendarDate(day, time.Month(month), matches[3], now, true), true
		}
	}
	if matches := absoluteDatePattern.FindStringSubmatch(raw); len(matches) == 4 {
		day, month := mustInt(matches[1]), mustInt(matches[2])
		return resolveCalendarDate(day, time.Month(month), matches[3], now, true), true
	}
	if matches := namedMonthPattern.FindStringSubmatch(raw); len(matches) == 4 {
		day := mustInt(matches[1])
		monthToken := normalizeDateInput(matches[2])
		if month, ok := resolveMonthToken(monthToken); ok {
			return resolveCalendarDate(day, month, matches[3], now, true), true
		}
		return ResolvedDate{Confidence: "unknown"}, true
	}
	if matches := monthFirstNamedPattern.FindStringSubmatch(raw); len(matches) == 4 {
		day := mustInt(matches[2])
		monthToken := normalizeDateInput(matches[1])
		if month, ok := resolveMonthToken(monthToken); ok {
			return resolveCalendarDate(day, month, matches[3], now, true), true
		}
		return ResolvedDate{Confidence: "unknown"}, true
	}
	if matches := relativeUnitsPattern.FindStringSubmatch(raw); len(matches) == 3 {
		value := addRelativeDuration(truncateToDate(now), mustInt(matches[1]), matches[2])
		if value == nil {
			return ResolvedDate{Confidence: "unknown"}, true
		}
		return ResolvedDate{Value: value, Confidence: "high"}, true
	}
	if matches := singleRelativePattern.FindStringSubmatch(raw); len(matches) == 2 {
		value := addRelativeDuration(truncateToDate(now), 1, matches[1])
		if value == nil {
			return ResolvedDate{Confidence: "unknown"}, true
		}
		return ResolvedDate{Value: value, Confidence: "high"}, true
	}
	if resolved, handled := resolveSpokenOrdinalDate(raw, now); handled {
		return resolved, true
	}
	if numericDayPattern.MatchString(raw) {
		day := mustInt(raw)
		value := nextFutureDayOfMonth(now, day)
		if value != nil {
			return ResolvedDate{Value: value, Confidence: "medium"}, true
		}
		return ResolvedDate{Confidence: "unknown"}, true
	}
	trimmed := trimDatePrefixes(raw)
	if weekday, ok := resolveWeekdayToken(trimmed); ok {
		value := nextWeekday(now, weekday)
		return ResolvedDate{Value: &value, Confidence: "medium"}, true
	}
	return ResolvedDate{}, false
}

func resolveSpokenOrdinalDate(raw string, now time.Time) (ResolvedDate, bool) {
	trimmed := trimDatePrefixes(raw)
	tokens := strings.Fields(trimmed)
	if len(tokens) == 0 {
		return ResolvedDate{}, false
	}
	day, usedDay := parseSpokenOrdinalDay(tokens)
	if usedDay == 0 {
		return ResolvedDate{}, false
	}
	rest := tokens[usedDay:]
	if len(rest) == 0 {
		value := nextFutureDayOfMonth(now, day)
		if value == nil {
			return ResolvedDate{Confidence: "unknown"}, true
		}
		return ResolvedDate{Value: value, Confidence: "medium"}, true
	}
	month, usedMonth := parseSpokenOrdinalMonth(rest)
	if usedMonth == 0 {
		if resolvedMonth, ok := resolveMonthToken(rest[0]); ok {
			month = resolvedMonth
			usedMonth = 1
		}
	}
	if usedMonth == 0 {
		return ResolvedDate{}, false
	}
	rest = rest[usedMonth:]
	if len(rest) > 0 {
		return ResolvedDate{}, false
	}
	return resolveCalendarDate(day, month, "", now, true), true
}

func parseSpokenOrdinalDay(tokens []string) (int, int) {
	if len(tokens) == 0 {
		return 0, 0
	}
	if value, ok := spokenOrdinalDayWords[tokens[0]]; ok {
		return value, 1
	}
	if len(tokens) < 2 {
		return 0, 0
	}
	tens, ok := spokenOrdinalTensWords[tokens[0]]
	if !ok {
		return 0, 0
	}
	unit, ok := spokenOrdinalUnitWords[tokens[1]]
	if !ok {
		return 0, 0
	}
	value := tens + unit
	if value <= 0 || value > 31 {
		return 0, 0
	}
	return value, 2
}

func parseSpokenOrdinalMonth(tokens []string) (time.Month, int) {
	if len(tokens) == 0 {
		return 0, 0
	}
	if tokens[0] == "ноль" && len(tokens) >= 2 {
		if value, ok := spokenOrdinalDayWords[tokens[1]]; ok && value >= 1 && value <= 9 {
			return time.Month(value), 2
		}
	}
	if value, ok := spokenOrdinalDayWords[tokens[0]]; ok && value >= 1 && value <= 12 {
		return time.Month(value), 1
	}
	return 0, 0
}

func extractStrictDatePhrase(raw string, now time.Time) (ExtractedDate, bool) {
	tokens := strings.Fields(raw)
	if len(tokens) == 0 {
		return ExtractedDate{}, false
	}
	for start := 0; start < len(tokens); start++ {
		if !looksLikeDateLeadToken(tokens[start]) {
			continue
		}
		maxEnd := start + 5
		if maxEnd > len(tokens) {
			maxEnd = len(tokens)
		}
		for end := maxEnd; end > start; end-- {
			phrase := strings.Join(tokens[start:end], " ")
			resolved, handled := resolveStrictDate(phrase, now)
			if !handled || resolved.Value == nil {
				continue
			}
			return ExtractedDate{
				Phrase:     phrase,
				Value:      resolved.Value,
				Confidence: resolved.Confidence,
				Source:     "strict_phrase",
				Absolute:   resolved.Absolute,
			}, true
		}
	}
	return ExtractedDate{}, false
}

func looksLikeDateLeadToken(token string) bool {
	switch token {
	case "до", "к", "на", "в", "во", "через":
		return true
	}
	if hasDigits(token) {
		return true
	}
	if _, ok := resolveWeekdayToken(token); ok {
		return true
	}
	if _, ok := resolveMonthToken(token); ok {
		return true
	}
	if _, ok := spokenOrdinalDayWords[token]; ok {
		return true
	}
	if _, ok := spokenOrdinalTensWords[token]; ok {
		return true
	}
	return token == "ноль"
}

func extractDateWithWhen(raw string, now time.Time) (ExtractedDate, bool) {
	parser, err := ruWhenParser()
	if err != nil {
		return ExtractedDate{}, false
	}
	result, err := parser.Parse(raw, now)
	if err != nil || result == nil {
		return ExtractedDate{}, false
	}
	phrase := normalizeDateInput(expandWhenPhrase(raw, result.Text))
	if phrase == "" {
		return ExtractedDate{}, false
	}
	value := truncateToDate(result.Time.In(now.Location()))
	return ExtractedDate{
		Phrase:     phrase,
		Value:      &value,
		Confidence: "medium",
		Source:     "when",
		Absolute:   false,
	}, true
}

func expandWhenPhrase(raw, phrase string) string {
	raw = normalizeDateInput(raw)
	phrase = normalizeDateInput(phrase)
	if raw == "" || phrase == "" {
		return phrase
	}
	index := strings.LastIndex(raw, phrase)
	if index <= 0 {
		return phrase
	}
	leading := strings.Fields(strings.TrimSpace(raw[:index]))
	if len(leading) == 0 {
		return phrase
	}
	start := len(leading)
	for start > 0 {
		if _, ok := datePhraseLeadingTokens[leading[start-1]]; !ok {
			break
		}
		start--
	}
	if start == len(leading) {
		return phrase
	}
	return strings.Join(append(leading[start:], phrase), " ")
}

func ruWhenParser() (*when.Parser, error) {
	whenOnce.Do(func() {
		parser := when.New(nil)
		parser.Add(ru.All...)
		whenParser = parser
	})
	return whenParser, whenInitErr
}

func truncateToDate(now time.Time) time.Time {
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
}

func nextWeekday(now time.Time, target time.Weekday) time.Time {
	base := truncateToDate(now)
	offset := (int(target) - int(base.Weekday()) + 7) % 7
	if offset == 0 {
		offset = 7
	}
	return base.AddDate(0, 0, offset)
}

func normalizeYear(v int) int {
	if v < 100 {
		return 2000 + v
	}
	return v
}

func mustInt(v string) int {
	var result int
	_, _ = fmt.Sscanf(v, "%d", &result)
	return result
}

func normalizeDateInput(raw string) string {
	lower := strings.ToLower(strings.TrimSpace(raw))
	lower = strings.ReplaceAll(lower, "ё", "е")
	lower = strings.NewReplacer(",", " ", ";", " ", "!", " ", "?", " ", "\"", " ", "'", " ", "«", " ", "»", " ", "(", " ", ")", " ", ":", " ").Replace(lower)
	tokens := strings.Fields(lower)
	for i, token := range tokens {
		tokens[i] = normalizeDateToken(token)
	}
	return strings.Join(tokens, " ")
}

func trimStrictDateDecorators(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	for {
		changed := false
		for _, prefix := range strictDateLeadingPhrases {
			if raw == prefix || strings.HasPrefix(raw, prefix+" ") {
				raw = strings.TrimSpace(strings.TrimPrefix(raw, prefix))
				changed = true
				break
			}
		}
		if !changed {
			break
		}
	}
	fields := strings.Fields(raw)
	for len(fields) > 1 {
		last := fields[len(fields)-1]
		if !hasDigits(last) && utf8.RuneCountInString(last) <= 1 {
			fields = fields[:len(fields)-1]
			continue
		}
		break
	}
	return strings.Join(fields, " ")
}

func normalizeDateToken(token string) string {
	if token == "" {
		return token
	}
	if !hasDigits(token) {
		token = strings.Trim(token, ".")
	}
	if token == "" || hasDigits(token) {
		return token
	}
	if override, ok := dateTokenOverrides[token]; ok {
		return override
	}
	for _, known := range dateVocabulary {
		if token == known {
			return known
		}
	}
	bestDistance := correctionDistanceLimit(token) + 1
	bestToken := ""
	for _, known := range dateVocabulary {
		if abs(utf8.RuneCountInString(token)-utf8.RuneCountInString(known)) > 2 {
			continue
		}
		if firstRune(token) != firstRune(known) {
			continue
		}
		distance := levenshteinDistance(token, known)
		if distance < bestDistance {
			bestDistance = distance
			bestToken = known
		}
	}
	if bestToken != "" {
		return bestToken
	}
	return token
}

func correctionDistanceLimit(token string) int {
	switch n := utf8.RuneCountInString(token); {
	case n <= 3:
		return 0
	case n <= 6:
		return 1
	default:
		return 2
	}
}

func firstRune(value string) rune {
	r, _ := utf8.DecodeRuneInString(value)
	return r
}

func hasDigits(value string) bool {
	for _, r := range value {
		if r >= '0' && r <= '9' {
			return true
		}
	}
	return false
}

func trimDatePrefixes(raw string) string {
	for _, prefix := range []string{"до ", "by ", "к ", "на ", "в ", "во ", "до:", "к:", "на:", "в:", "во:"} {
		if strings.HasPrefix(raw, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(raw, prefix))
		}
	}
	return raw
}

func resolveWeekdayToken(raw string) (time.Weekday, bool) {
	if weekday, ok := weekdayMap[raw]; ok {
		return weekday, true
	}
	bestDistance := 3
	var best time.Weekday
	found := false
	for token, weekday := range weekdayMap {
		if utf8.RuneCountInString(token) <= 2 {
			continue
		}
		if firstRune(raw) != firstRune(token) {
			continue
		}
		distance := levenshteinDistance(raw, token)
		if distance < bestDistance {
			bestDistance = distance
			best = weekday
			found = true
		}
	}
	if found {
		return best, true
	}
	return 0, false
}

func resolveMonthToken(raw string) (time.Month, bool) {
	if month, ok := monthMap[raw]; ok {
		return month, true
	}
	bestDistance := 3
	var best time.Month
	found := false
	for token, month := range monthMap {
		if firstRune(raw) != firstRune(token) {
			continue
		}
		distance := levenshteinDistance(raw, token)
		if distance < bestDistance {
			bestDistance = distance
			best = month
			found = true
		}
	}
	if found {
		return best, true
	}
	return 0, false
}

func resolveCalendarDate(day int, month time.Month, yearToken string, now time.Time, absolute bool) ResolvedDate {
	if day <= 0 || day > 31 || month < time.January || month > time.December {
		return ResolvedDate{Confidence: "unknown"}
	}
	year := now.Year()
	if strings.TrimSpace(yearToken) != "" {
		year = normalizeYear(mustInt(yearToken))
	}
	value := time.Date(year, month, day, 0, 0, 0, 0, now.Location())
	if value.Month() != month || value.Day() != day {
		return ResolvedDate{Confidence: "unknown"}
	}
	if strings.TrimSpace(yearToken) == "" && value.Before(truncateToDate(now)) {
		next := time.Date(now.Year()+1, month, day, 0, 0, 0, 0, now.Location())
		if next.Month() == month && next.Day() == day {
			value = next
		}
	}
	confidence := "high"
	if strings.TrimSpace(yearToken) == "" {
		confidence = "medium"
	}
	return ResolvedDate{Value: &value, Confidence: confidence, Absolute: absolute}
}

func addRelativeDuration(base time.Time, amount int, unit string) *time.Time {
	if amount <= 0 {
		return nil
	}
	var value time.Time
	switch unit {
	case "день", "дня", "днеи", "дней":
		value = base.AddDate(0, 0, amount)
	case "неделю", "недели", "недель":
		value = base.AddDate(0, 0, 7*amount)
	case "месяц", "месяца", "месяцев":
		value = base.AddDate(0, amount, 0)
	default:
		return nil
	}
	return &value
}

func nextFutureDayOfMonth(now time.Time, day int) *time.Time {
	if day <= 0 || day > 31 {
		return nil
	}
	base := truncateToDate(now)
	candidate := time.Date(base.Year(), base.Month(), day, 0, 0, 0, 0, base.Location())
	if candidate.Month() != base.Month() || candidate.Day() != day || candidate.Before(base) {
		candidate = time.Date(base.Year(), base.Month()+1, day, 0, 0, 0, 0, base.Location())
	}
	if candidate.Day() != day {
		return nil
	}
	return &candidate
}

func levenshteinDistance(a, b string) int {
	if a == b {
		return 0
	}
	if a == "" {
		return utf8.RuneCountInString(b)
	}
	if b == "" {
		return utf8.RuneCountInString(a)
	}
	left := []rune(a)
	right := []rune(b)
	if abs(len(left)-len(right)) > 2 {
		return 3
	}
	column := make([]int, len(right)+1)
	for i := range column {
		column[i] = i
	}
	for i, ra := range left {
		prevDiagonal := column[0]
		column[0] = i + 1
		for j, rb := range right {
			insertCost := column[j+1] + 1
			deleteCost := column[j] + 1
			replaceCost := prevDiagonal
			if ra != rb {
				replaceCost++
			}
			prevDiagonal = column[j+1]
			column[j+1] = minInt(insertCost, deleteCost, replaceCost)
		}
	}
	return column[len(right)]
}

func minInt(values ...int) int {
	best := values[0]
	for _, value := range values[1:] {
		if value < best {
			best = value
		}
	}
	return best
}

func abs(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

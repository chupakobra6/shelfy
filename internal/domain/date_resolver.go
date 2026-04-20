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
}

type ExtractedDate struct {
	Phrase     string
	Value      *time.Time
	Confidence string
	Source     string
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
	"янв":      time.January,
	"январь":   time.January,
	"января":   time.January,
	"feb":      time.February,
	"фев":      time.February,
	"февраль":  time.February,
	"февраля":  time.February,
	"мар":      time.March,
	"март":     time.March,
	"марта":    time.March,
	"апр":      time.April,
	"апрель":   time.April,
	"апреля":   time.April,
	"май":      time.May,
	"мая":      time.May,
	"июн":      time.June,
	"июнь":     time.June,
	"июня":     time.June,
	"июл":      time.July,
	"июль":     time.July,
	"июля":     time.July,
	"авг":      time.August,
	"август":   time.August,
	"августа":  time.August,
	"сен":      time.September,
	"сент":     time.September,
	"сентябрь": time.September,
	"сентября": time.September,
	"окт":      time.October,
	"октябрь":  time.October,
	"октября":  time.October,
	"ноя":      time.November,
	"ноябрь":   time.November,
	"ноября":   time.November,
	"дек":      time.December,
	"декабрь":  time.December,
	"декабря":  time.December,
}

var (
	absoluteDatePattern   = regexp.MustCompile(`^(\d{1,2})[./-](\d{1,2})(?:[./-](\d{2,4}))?$`)
	namedMonthPattern     = regexp.MustCompile(`^(\d{1,2})\s+([\p{L}]+)(?:\s+(\d{2,4}))?$`)
	numericDayPattern     = regexp.MustCompile(`^\d{1,2}$`)
	relativeUnitsPattern  = regexp.MustCompile(`^(?:через\s+)?(\d{1,2})\s+(день|дня|днеи|дней|неделю|недели|недель|месяц|месяца|месяцев)$`)
	singleRelativePattern = regexp.MustCompile(`^через\s+(неделю|месяц)$`)
	whenOnce              sync.Once
	whenParser            *when.Parser
	whenInitErr           error
)

var dateVocabulary = []string{
	"сегодня", "завтра", "послезавтра", "позавтра",
	"через", "день", "дня", "днеи", "дней", "неделю", "недели", "недель", "месяц", "месяца", "месяцев",
	"до", "к", "на", "в", "во", "by",
	"следующий", "следующую", "следующей", "следующее",
	"понедельник", "понедельника", "понедельнику", "пн",
	"вторник", "вторника", "вторнику", "вт",
	"среда", "среды", "среду", "среде", "ср",
	"четверг", "четверга", "четвергу", "чт",
	"пятница", "пятницы", "пятницу", "пятнице", "пт",
	"суббота", "субботы", "субботу", "субботе", "сб",
	"воскресенье", "воскресенья", "воскресенью", "вс",
	"янв", "январь", "января", "фев", "февраль", "февраля",
	"мар", "март", "марта", "апр", "апрель", "апреля",
	"май", "мая", "июн", "июнь", "июня",
	"июл", "июль", "июля", "авг", "август", "августа",
	"сен", "сент", "сентябрь", "сентября",
	"окт", "октябрь", "октября", "ноя", "ноябрь", "ноября",
	"дек", "декабрь", "декабря",
}

var dateTokenOverrides = map[string]string{
	"пятницаы": "пятницы",
	"субота":   "суббота",
	"суботы":   "субботы",
	"суботу":   "субботу",
	"днеи":     "дней",
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

func ResolveRelativeDate(raw string, now time.Time) ResolvedDate {
	raw = normalizeDateInput(raw)
	if raw == "" {
		return ResolvedDate{Confidence: "missing"}
	}
	if resolved, handled := resolveStrictDate(raw, now); handled {
		return resolved
	}
	if extracted, ok := extractDateWithWhen(raw, now); ok {
		return ResolvedDate{Value: extracted.Value, Confidence: extracted.Confidence}
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
		}, true
	}
	return extractDateWithWhen(raw, now)
}

func resolveStrictDate(raw string, now time.Time) (ResolvedDate, bool) {
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
	if matches := absoluteDatePattern.FindStringSubmatch(raw); len(matches) == 4 {
		day, month := mustInt(matches[1]), mustInt(matches[2])
		return resolveCalendarDate(day, time.Month(month), matches[3], now), true
	}
	if matches := namedMonthPattern.FindStringSubmatch(raw); len(matches) == 4 {
		day := mustInt(matches[1])
		monthToken := normalizeDateInput(matches[2])
		if month, ok := resolveMonthToken(monthToken); ok {
			return resolveCalendarDate(day, month, matches[3], now), true
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

func normalizeDateToken(token string) string {
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

func resolveCalendarDate(day int, month time.Month, yearToken string, now time.Time) ResolvedDate {
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
	return ResolvedDate{Value: &value, Confidence: confidence}
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

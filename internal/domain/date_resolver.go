package domain

import (
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

type ResolvedDate struct {
	Value      *time.Time
	Confidence string
}

var weekdayMap = map[string]time.Weekday{
	"monday":       time.Monday,
	"понедельник":  time.Monday,
	"пн":           time.Monday,
	"tuesday":      time.Tuesday,
	"вторник":      time.Tuesday,
	"вт":           time.Tuesday,
	"wednesday":    time.Wednesday,
	"среда":        time.Wednesday,
	"ср":           time.Wednesday,
	"четверг":      time.Thursday,
	"thursday":     time.Thursday,
	"чт":           time.Thursday,
	"friday":       time.Friday,
	"пятница":      time.Friday,
	"пятницы":      time.Friday,
	"пт":           time.Friday,
	"суббота":      time.Saturday,
	"субботы":      time.Saturday,
	"saturday":     time.Saturday,
	"сб":           time.Saturday,
	"воскресенье":  time.Sunday,
	"воскресенья":  time.Sunday,
	"sunday":       time.Sunday,
	"вс":           time.Sunday,
	"понедельника": time.Monday,
	"вторника":     time.Tuesday,
	"среды":        time.Wednesday,
	"четверга":     time.Thursday,
}

var absoluteDatePattern = regexp.MustCompile(`\b(\d{1,2})[./-](\d{1,2})(?:[./-](\d{2,4}))?\b`)
var namedMonthPattern = regexp.MustCompile(`(\d{1,2})\s+([\p{L}]+)(?:\s+(\d{2,4}))?`)
var numericDayPattern = regexp.MustCompile(`^\d{1,2}$`)

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

func ResolveRelativeDate(raw string, now time.Time) ResolvedDate {
	raw = normalizeDateInput(raw)
	if raw == "" {
		return ResolvedDate{Confidence: "missing"}
	}
	if raw == "today" || raw == "сегодня" {
		value := truncateToDate(now)
		return ResolvedDate{Value: &value, Confidence: "high"}
	}
	if raw == "tomorrow" || raw == "завтра" {
		value := truncateToDate(now).AddDate(0, 0, 1)
		return ResolvedDate{Value: &value, Confidence: "high"}
	}
	if matches := absoluteDatePattern.FindStringSubmatch(raw); len(matches) == 4 {
		day, month := mustInt(matches[1]), mustInt(matches[2])
		return resolveCalendarDate(day, time.Month(month), matches[3], now)
	}
	if matches := namedMonthPattern.FindStringSubmatch(raw); len(matches) == 4 {
		day := mustInt(matches[1])
		monthToken := normalizeDateInput(matches[2])
		if month, ok := resolveMonthToken(monthToken); ok {
			return resolveCalendarDate(day, month, matches[3], now)
		}
	}
	if numericDayPattern.MatchString(raw) {
		day := mustInt(raw)
		value := nextFutureDayOfMonth(now, day)
		if value != nil {
			return ResolvedDate{Value: value, Confidence: "medium"}
		}
	}
	trimmed := trimDatePrefixes(raw)
	if weekday, ok := resolveWeekdayToken(trimmed); ok {
		value := nextWeekday(now, weekday)
		return ResolvedDate{Value: &value, Confidence: "medium"}
	}
	return ResolvedDate{Confidence: "unknown"}
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
	lower = strings.NewReplacer(",", " ", ";", " ", "!", " ", "?", " ", "\"", " ", "'", " ", "«", " ", "»", " ", "(", " ", ")", " ").Replace(lower)
	return strings.Join(strings.Fields(lower), " ")
}

func trimDatePrefixes(raw string) string {
	for _, prefix := range []string{"до ", "by ", "к ", "на ", "до:", "к:"} {
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

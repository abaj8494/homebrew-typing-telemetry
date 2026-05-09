package stats

import "time"

type Summary struct {
	TodayKeystrokes int64
	TodayWords      int64
	WeekKeystrokes  int64
	WeekWords       int64
	AvgPerDay       float64
	PeakHour        int
	PeakHourCount   int64
}

type DayData struct {
	Date       time.Time
	Keystrokes int64
	Words      int64
}

func CalculateWeeklyAverage(days []DayData) float64 {
	if len(days) == 0 {
		return 0
	}
	var total int64
	for _, d := range days {
		total += d.Keystrokes
	}
	return float64(total) / float64(len(days))
}

// CalculateAverageWords returns the mean words per day across all days.
func CalculateAverageWords(days []DayData) float64 {
	if len(days) == 0 {
		return 0
	}
	var total int64
	for _, d := range days {
		total += d.Words
	}
	return float64(total) / float64(len(days))
}

// CalculateAverageWordsActive returns the mean words per *active* day
// (days with at least one keystroke or word). This is a more honest
// measure of typing volume than dividing by every calendar day.
func CalculateAverageWordsActive(days []DayData) float64 {
	var total int64
	var active int
	for _, d := range days {
		if d.Keystrokes > 0 || d.Words > 0 {
			total += d.Words
			active++
		}
	}
	if active == 0 {
		return 0
	}
	return float64(total) / float64(active)
}

// CountActiveDays returns the number of days with at least one keystroke.
func CountActiveDays(days []DayData) int {
	var active int
	for _, d := range days {
		if d.Keystrokes > 0 {
			active++
		}
	}
	return active
}

// FindPeakDay returns the day with the highest keystroke count.
// If multiple days tie, the earliest one wins.
func FindPeakDay(days []DayData) (DayData, bool) {
	if len(days) == 0 {
		return DayData{}, false
	}
	peak := days[0]
	for _, d := range days[1:] {
		if d.Keystrokes > peak.Keystrokes {
			peak = d
		}
	}
	if peak.Keystrokes == 0 {
		return DayData{}, false
	}
	return peak, true
}

// CurrentStreak returns the number of consecutive active days ending on
// the most recent day in the slice. The slice is expected to be ordered
// chronologically (oldest first); if not, the result is undefined.
func CurrentStreak(days []DayData) int {
	streak := 0
	for i := len(days) - 1; i >= 0; i-- {
		if days[i].Keystrokes > 0 {
			streak++
		} else {
			break
		}
	}
	return streak
}

// LongestStreak returns the longest run of consecutive active days in
// the slice. The slice is expected to be ordered chronologically.
func LongestStreak(days []DayData) int {
	longest, current := 0, 0
	for _, d := range days {
		if d.Keystrokes > 0 {
			current++
			if current > longest {
				longest = current
			}
		} else {
			current = 0
		}
	}
	return longest
}

func FindPeakHour(hourlyData []int64) (hour int, count int64) {
	for h, c := range hourlyData {
		if c > count {
			hour = h
			count = c
		}
	}
	return
}

// FormatHour renders a 24h hour as a 12h clock label (e.g. "3 PM").
func FormatHour(hour int) string {
	if hour < 0 || hour > 23 {
		return "—"
	}
	suffix := "AM"
	h := hour
	if h == 0 {
		h = 12
	} else if h == 12 {
		suffix = "PM"
	} else if h > 12 {
		h -= 12
		suffix = "PM"
	}
	return formatInt(int64(h)) + " " + suffix
}

func FormatKeystrokeCount(count int64) string {
	if count >= 1000000 {
		return formatFloat(float64(count)/1000000) + "M"
	}
	if count >= 1000 {
		return formatFloat(float64(count)/1000) + "K"
	}
	return formatInt(count)
}

func formatFloat(f float64) string {
	intPart := int64(f)
	if f == float64(intPart) {
		return formatInt(intPart)
	}
	// Get first decimal digit
	decimalPart := int((f - float64(intPart)) * 10)
	return formatInt(intPart) + "." + string(byte('0'+decimalPart))
}

func formatInt(i int64) string {
	if i == 0 {
		return "0"
	}
	var result []byte
	for i > 0 {
		result = append([]byte{byte('0' + i%10)}, result...)
		i /= 10
	}
	return string(result)
}

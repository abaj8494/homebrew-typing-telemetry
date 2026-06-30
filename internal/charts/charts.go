// Package charts renders the rich typing/activity statistics dashboard
// (Chart.js HTML) shared by the macOS menubar, the Linux tray, and the CLI's
// `typtel v`. It is pure Go and platform-neutral: every storage/stats call it
// makes is available on all platforms. The only platform-specific input is the
// pixels->feet conversion for mouse distance, injected via Options.PixelsToFeet
// (the menubar passes its display-aware version; others get a fixed-DPI
// default). When there is no mouse data (e.g. the Linux tray), the mouse chart
// simply reads zero.
//
// This generator was moved out of cmd/typtel-menubar so all front-ends share
// one implementation; behaviour is identical to the prior menubar charts.
package charts

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/aayushbajaj/typing-telemetry/internal/storage"
	"github.com/aayushbajaj/typing-telemetry/pkg/stats"
)

// Options configures chart generation.
type Options struct {
	// PixelsToFeet converts a raw mouse pixel distance to feet. If nil, a
	// fixed 100-PPI approximation is used.
	PixelsToFeet func(float64) float64
}

// defaultPixelsToFeet approximates feet from pixels at a fixed 100 PPI — the
// same fallback the CLI used before this package existed.
func defaultPixelsToFeet(pixels float64) float64 {
	return (pixels / 100.0) / 12.0
}

func Generate(store *storage.Store, opts Options) (string, error) {
	pf := opts.PixelsToFeet
	if pf == nil {
		pf = defaultPixelsToFeet
	}
	// Check if key types should be shown
	showKeyTypes := store.IsShowKeyTypesEnabled()

	type chartData struct {
		labels          []string
		keystrokeData   []string
		wordData        []string
		mouseDataFeet   []float64
		letterData      []string
		modifierData    []string
		specialData     []string
		totalKeystrokes int64
		totalWords      int64
		totalMouseDist  float64
		totalLetters    int64
		totalModifiers  int64
		totalSpecial    int64
		heatmapHTML     string
		// Derived stats
		avgKeystrokes  float64
		avgWordsActive float64
		peakDayLabel   string
		peakDayValue   int64
		peakWordsLabel string
		peakWordsValue int64
		peakHourLabel  string
		peakHourValue  int64
		currentStreak  int
		longestStreak  int
		activeDays     int
		totalClicks    int64
	}

	prepareChartData := func(days int) (*chartData, error) {
		data := &chartData{}

		histStats, err := store.GetHistoricalStats(days)
		if err != nil {
			return nil, err
		}

		mouseStats, err := store.GetMouseHistoricalStats(days)
		if err != nil {
			return nil, err
		}

		hourlyData, err := store.GetAllHourlyStatsForDays(days)
		if err != nil {
			return nil, err
		}

		dayData := make([]stats.DayData, 0, len(histStats))
		var peakWords stats.DayData
		hourTotals := make([]int64, 24)

		for i, stat := range histStats {
			t, _ := time.Parse("2006-01-02", stat.Date)
			data.labels = append(data.labels, fmt.Sprintf("'%s'", t.Format("Jan 2")))
			data.keystrokeData = append(data.keystrokeData, fmt.Sprintf("%d", stat.Keystrokes))
			data.wordData = append(data.wordData, fmt.Sprintf("%d", stat.Words))
			data.letterData = append(data.letterData, fmt.Sprintf("%d", stat.Letters))
			data.modifierData = append(data.modifierData, fmt.Sprintf("%d", stat.Modifiers))
			data.specialData = append(data.specialData, fmt.Sprintf("%d", stat.Special))
			data.totalKeystrokes += stat.Keystrokes
			data.totalWords += stat.Words
			data.totalLetters += stat.Letters
			data.totalModifiers += stat.Modifiers
			data.totalSpecial += stat.Special

			dd := stats.DayData{Date: t, Keystrokes: stat.Keystrokes, Words: stat.Words}
			dayData = append(dayData, dd)
			if dd.Words > peakWords.Words {
				peakWords = dd
			}

			if hours, ok := hourlyData[stat.Date]; ok {
				for _, h := range hours {
					if h.Hour >= 0 && h.Hour < 24 {
						hourTotals[h.Hour] += h.Keystrokes
					}
				}
			}

			if i < len(mouseStats) {
				feet := pf(mouseStats[i].TotalDistance)
				data.mouseDataFeet = append(data.mouseDataFeet, feet)
				data.totalMouseDist += mouseStats[i].TotalDistance
				data.totalClicks += mouseStats[i].ClickCount
			} else {
				data.mouseDataFeet = append(data.mouseDataFeet, 0)
			}
		}

		// Derived stats
		if days > 0 {
			data.avgKeystrokes = float64(data.totalKeystrokes) / float64(days)
		}
		data.avgWordsActive = stats.CalculateAverageWordsActive(dayData)
		data.activeDays = stats.CountActiveDays(dayData)
		data.currentStreak = stats.CurrentStreak(dayData)
		data.longestStreak = stats.LongestStreak(dayData)

		if peak, ok := stats.FindPeakDay(dayData); ok {
			data.peakDayLabel = peak.Date.Format("Jan 2")
			data.peakDayValue = peak.Keystrokes
		} else {
			data.peakDayLabel = "—"
		}
		if peakWords.Words > 0 {
			data.peakWordsLabel = peakWords.Date.Format("Jan 2")
			data.peakWordsValue = peakWords.Words
		} else {
			data.peakWordsLabel = "—"
		}

		hour, count := stats.FindPeakHour(hourTotals)
		if count > 0 {
			data.peakHourLabel = stats.FormatHour(hour)
			data.peakHourValue = count
		} else {
			data.peakHourLabel = "—"
		}

		data.heatmapHTML = generateHeatmapHTML(hourlyData, days)
		return data, nil
	}

	weeklyData, err := prepareChartData(7)
	if err != nil {
		return "", err
	}

	monthlyData, err := prepareChartData(30)
	if err != nil {
		return "", err
	}

	yearlyData, err := prepareChartData(365)
	if err != nil {
		return "", err
	}

	formatMouseData := func(feetData []float64, divisor float64) string {
		var result []string
		for _, f := range feetData {
			result = append(result, fmt.Sprintf("%.2f", f/divisor))
		}
		return strings.Join(result, ",")
	}

	weeklyMouseFeetStr := formatMouseData(weeklyData.mouseDataFeet, 1.0)
	weeklyMouseCarsStr := formatMouseData(weeklyData.mouseDataFeet, 15.0)
	weeklyMouseFieldsStr := formatMouseData(weeklyData.mouseDataFeet, 330.0)
	monthlyMouseFeetStr := formatMouseData(monthlyData.mouseDataFeet, 1.0)
	monthlyMouseCarsStr := formatMouseData(monthlyData.mouseDataFeet, 15.0)
	monthlyMouseFieldsStr := formatMouseData(monthlyData.mouseDataFeet, 330.0)
	yearlyMouseFeetStr := formatMouseData(yearlyData.mouseDataFeet, 1.0)
	yearlyMouseCarsStr := formatMouseData(yearlyData.mouseDataFeet, 15.0)
	yearlyMouseFieldsStr := formatMouseData(yearlyData.mouseDataFeet, 330.0)

	// Get odometer data
	odometerSession, _ := store.GetOdometerSession()
	odometerIsActive := false
	odometerStartTime := ""
	odometerKeystrokes := int64(0)
	odometerWords := int64(0)
	odometerClicks := int64(0)
	odometerDistanceFeet := float64(0)

	if odometerSession != nil {
		odometerIsActive = odometerSession.IsActive
		if !odometerSession.StartTime.IsZero() {
			odometerStartTime = odometerSession.StartTime.Format("Jan 2, 2006 3:04 PM")
		}
		odometerKeystrokes = odometerSession.CurrentKeystrokes - odometerSession.StartKeystrokes
		odometerWords = odometerSession.CurrentWords - odometerSession.StartWords
		odometerClicks = odometerSession.CurrentClicks - odometerSession.StartClicks
		odometerDistanceFeet = pf(odometerSession.CurrentDistance - odometerSession.StartDistance)
	}

	// Get odometer history
	odometerHistory, _ := store.GetOdometerHistory()
	var historyJSON strings.Builder
	historyJSON.WriteString("[")
	for i, entry := range odometerHistory {
		if i > 0 {
			historyJSON.WriteString(",")
		}
		duration := entry.EndTime.Sub(entry.StartTime)
		historyJSON.WriteString(fmt.Sprintf(`{"id":%d,"startTime":"%s","endTime":"%s","keystrokes":%d,"words":%d,"clicks":%d,"distanceFeet":%.2f,"durationSecs":%d}`,
			entry.ID,
			entry.StartTime.Format("Jan 2, 2006 3:04 PM"),
			entry.EndTime.Format("Jan 2, 2006 3:04 PM"),
			entry.Keystrokes,
			entry.Words,
			entry.Clicks,
			pf(entry.Distance),
			int64(duration.Seconds()),
		))
	}
	historyJSON.WriteString("]")

	// Determine if key types section should be visible
	keyTypesDisplay := "none"
	if showKeyTypes {
		keyTypesDisplay = "flex"
	}

	html := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
    <title>Typtel - Typing Statistics</title>
    <script src="https://cdn.jsdelivr.net/npm/chart.js"></script>
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            background: linear-gradient(135deg, #1a1a2e 0%%, #16213e 100%%);
            color: #eee;
            min-height: 100vh;
            padding: 30px;
        }
        h1 {
            text-align: center;
            margin-bottom: 10px;
            font-size: 2.5em;
            background: linear-gradient(90deg, #00d2ff, #3a7bd5);
            -webkit-background-clip: text;
            -webkit-text-fill-color: transparent;
        }
        .controls {
            display: flex;
            justify-content: center;
            gap: 20px;
            margin-bottom: 30px;
        }
        .control-group {
            display: flex;
            align-items: center;
            gap: 10px;
        }
        .control-group label {
            color: #888;
            font-size: 0.9em;
        }
        select {
            background: rgba(255,255,255,0.1);
            border: 1px solid rgba(255,255,255,0.2);
            border-radius: 8px;
            color: #eee;
            padding: 8px 16px;
            font-size: 0.9em;
            cursor: pointer;
        }
        select:hover {
            background: rgba(255,255,255,0.15);
        }
        .charts-container {
            display: grid;
            grid-template-columns: 1fr 1fr;
            gap: 30px;
            max-width: 1400px;
            margin: 0 auto 40px;
        }
        .chart-box {
            background: rgba(255,255,255,0.05);
            border-radius: 16px;
            padding: 25px;
            backdrop-filter: blur(10px);
            border: 1px solid rgba(255,255,255,0.1);
        }
        .chart-box h2 {
            margin-bottom: 20px;
            font-size: 1.3em;
            color: #aaa;
        }
        .heatmap-container {
            max-width: 1400px;
            margin: 0 auto;
        }
        .heatmap-box {
            background: rgba(255,255,255,0.05);
            border-radius: 16px;
            padding: 25px;
            backdrop-filter: blur(10px);
            border: 1px solid rgba(255,255,255,0.1);
        }
        .heatmap-box h2 {
            margin-bottom: 20px;
            font-size: 1.3em;
            color: #aaa;
        }
        .heatmap {
            display: flex;
            flex-direction: column;
            gap: 3px;
        }
        .heatmap-row {
            display: flex;
            align-items: center;
            gap: 3px;
        }
        .heatmap-label {
            width: 70px;
            font-size: 11px;
            color: #888;
            text-align: right;
            padding-right: 10px;
        }
        .heatmap-cell {
            width: 20px;
            height: 20px;
            border-radius: 3px;
            transition: transform 0.2s;
        }
        .heatmap-cell:hover {
            transform: scale(1.3);
            z-index: 10;
        }
        .hour-labels {
            display: flex;
            gap: 3px;
            margin-left: 80px;
            margin-bottom: 5px;
        }
        .hour-label {
            width: 20px;
            font-size: 10px;
            color: #666;
            text-align: center;
        }
        .legend {
            display: flex;
            align-items: center;
            justify-content: center;
            gap: 8px;
            margin-top: 20px;
        }
        .legend-text { color: #666; font-size: 12px; }
        .legend-box {
            width: 15px;
            height: 15px;
            border-radius: 2px;
        }
        .stats-summary {
            display: flex;
            justify-content: center;
            flex-wrap: wrap;
            gap: 40px;
            margin: 30px 0;
        }
        .stats-summary-secondary {
            margin-top: -10px;
            padding-top: 20px;
            border-top: 1px solid rgba(255,255,255,0.08);
            max-width: 1400px;
            margin-left: auto;
            margin-right: auto;
        }
        .stat-item {
            text-align: center;
            min-width: 110px;
        }
        .stat-value {
            font-size: 2.5em;
            font-weight: bold;
            background: linear-gradient(90deg, #00d2ff, #3a7bd5);
            -webkit-background-clip: text;
            -webkit-text-fill-color: transparent;
        }
        .stat-secondary {
            font-size: 1.6em;
            background: linear-gradient(90deg, #ffd166, #f08a5d);
            -webkit-background-clip: text;
            -webkit-text-fill-color: transparent;
        }
        .stat-label {
            color: #888;
            font-size: 0.9em;
        }
        .tooltip-container {
            display: inline-flex;
            align-items: center;
            gap: 6px;
        }
        .tooltip-help {
            display: inline-flex;
            align-items: center;
            justify-content: center;
            width: 16px;
            height: 16px;
            border-radius: 50%%;
            background: rgba(255,255,255,0.15);
            color: #888;
            font-size: 11px;
            cursor: help;
            position: relative;
        }
        .tooltip-help:hover {
            background: rgba(255,255,255,0.25);
            color: #fff;
        }
        .tooltip-content {
            display: none;
            position: absolute;
            bottom: 130%%;
            left: 50%%;
            transform: translateX(-50%%);
            background: rgba(30,30,50,0.98);
            border: 1px solid rgba(255,255,255,0.2);
            border-radius: 8px;
            padding: 10px 14px;
            min-width: 220px;
            font-size: 12px;
            color: #ddd;
            text-align: left;
            z-index: 100;
            line-height: 1.5;
            box-shadow: 0 4px 12px rgba(0,0,0,0.3);
        }
        .tooltip-content::after {
            content: '';
            position: absolute;
            top: 100%%;
            left: 50%%;
            transform: translateX(-50%%);
            border: 6px solid transparent;
            border-top-color: rgba(30,30,50,0.98);
        }
        .tooltip-help:hover .tooltip-content {
            display: block;
        }
        .tooltip-content strong {
            color: #fff;
        }
        .odometer-display {
            display: none;
            max-width: 800px;
            margin: 30px auto;
        }
        .odometer-box {
            background: rgba(255,255,255,0.05);
            border-radius: 16px;
            padding: 25px;
            backdrop-filter: blur(10px);
            border: 1px solid rgba(255,255,255,0.1);
        }
        .odometer-box h2 {
            margin-bottom: 20px;
            font-size: 1.3em;
            color: #aaa;
        }
        .odometer-status {
            text-align: center;
            padding: 15px;
            margin-bottom: 20px;
            border-radius: 8px;
            font-size: 1.2em;
            font-weight: bold;
        }
        .odometer-status.active {
            background: rgba(122, 201, 111, 0.2);
            color: #7bc96f;
        }
        .odometer-status.inactive {
            background: rgba(255, 107, 107, 0.2);
            color: #ff6b6b;
        }
        .odometer-table {
            width: 100%%;
            border-collapse: collapse;
        }
        .odometer-table th {
            text-align: left;
            padding: 12px;
            border-bottom: 2px solid rgba(255,255,255,0.2);
            color: #888;
            font-size: 0.9em;
        }
        .odometer-table td {
            padding: 12px;
            border-bottom: 1px solid rgba(255,255,255,0.05);
        }
        .odometer-table tr:hover {
            background: rgba(255,255,255,0.05);
        }
        .odometer-value {
            font-weight: bold;
            background: linear-gradient(90deg, #00d2ff, #3a7bd5);
            -webkit-background-clip: text;
            -webkit-text-fill-color: transparent;
        }
    </style>
</head>
<body>
    <h1>Typtel Statistics</h1>

    <div class="controls">
        <div class="control-group">
            <label>Time Period:</label>
            <select id="periodSelect" onchange="updateCharts()">
                <option value="weekly">Weekly (7 days)</option>
                <option value="monthly">Monthly (30 days)</option>
                <option value="yearly">Yearly (365 days)</option>
                <option value="odometer">Odometer</option>
            </select>
        </div>
        <div class="control-group">
            <label>Distance Unit:</label>
            <select id="unitSelect" onchange="updateCharts()">
                <option value="feet">Feet</option>
                <option value="cars">Car Lengths (~15ft)</option>
                <option value="fields">Frisbee Fields (~330ft)</option>
            </select>
        </div>
    </div>

    <div class="stats-summary">
        <div class="stat-item">
            <div class="stat-value" id="totalKeystrokes">-</div>
            <div class="stat-label">Total Keystrokes</div>
        </div>
        <div class="stat-item">
            <div class="stat-value" id="totalWords">-</div>
            <div class="stat-label">Words</div>
        </div>
        <div class="stat-item">
            <div class="stat-value" id="avgKeystrokes">-</div>
            <div class="stat-label">Avg Keystrokes/Day</div>
        </div>
        <div class="stat-item">
            <div class="stat-value" id="avgWords">-</div>
            <div class="stat-label tooltip-container">Avg Words / Active Day <span class="tooltip-help">?<div class="tooltip-content"><strong>Average Words per Active Day</strong><br>Mean words on days with at least one keystroke. Excludes idle days so the figure reflects how much you type when you're actually typing.</div></span></div>
        </div>
        <div class="stat-item">
            <div class="stat-value" id="totalMouse">-</div>
            <div class="stat-label">Mouse Distance</div>
        </div>
        <div class="stat-item">
            <div class="stat-value" id="totalClicks">-</div>
            <div class="stat-label">Mouse Clicks</div>
        </div>
    </div>

    <div class="stats-summary stats-summary-secondary">
        <div class="stat-item">
            <div class="stat-value stat-secondary" id="peakDay">-</div>
            <div class="stat-label" id="peakDayLabel">Peak Day</div>
        </div>
        <div class="stat-item">
            <div class="stat-value stat-secondary" id="peakWords">-</div>
            <div class="stat-label" id="peakWordsLabel">Peak Words Day</div>
        </div>
        <div class="stat-item">
            <div class="stat-value stat-secondary" id="peakHour">-</div>
            <div class="stat-label" id="peakHourLabel">Most Active Hour</div>
        </div>
        <div class="stat-item">
            <div class="stat-value stat-secondary" id="currentStreak">-</div>
            <div class="stat-label tooltip-container">Current Streak <span class="tooltip-help">?<div class="tooltip-content"><strong>Current Streak</strong><br>Number of consecutive days, ending today, with at least one keystroke recorded.</div></span></div>
        </div>
        <div class="stat-item">
            <div class="stat-value stat-secondary" id="longestStreak">-</div>
            <div class="stat-label">Longest Streak</div>
        </div>
        <div class="stat-item">
            <div class="stat-value stat-secondary" id="activeDays">-</div>
            <div class="stat-label" id="activeDaysLabel">Active Days</div>
        </div>
    </div>

    <div class="stats-summary" id="keyTypesStats" style="display: %[1]s;">
        <div class="stat-item">
            <div class="stat-value" id="totalLetters" style="background: linear-gradient(90deg, #7bc96f, #4caf50); -webkit-background-clip: text; -webkit-text-fill-color: transparent;">-</div>
            <div class="stat-label">Letters (A-Z)</div>
        </div>
        <div class="stat-item">
            <div class="stat-value" id="totalModifiers" style="background: linear-gradient(90deg, #ff9800, #f57c00); -webkit-background-clip: text; -webkit-text-fill-color: transparent;">-</div>
            <div class="stat-label tooltip-container">Modifiers <span class="tooltip-help">?<div class="tooltip-content"><strong>Modifier Keys:</strong><br>Shift (Left/Right), Control (Left/Right), Option/Alt (Left/Right), Command (Left/Right), Fn, Caps Lock</div></span></div>
        </div>
        <div class="stat-item">
            <div class="stat-value" id="totalSpecial" style="background: linear-gradient(90deg, #e91e63, #c2185b); -webkit-background-clip: text; -webkit-text-fill-color: transparent;">-</div>
            <div class="stat-label tooltip-container">Special Keys <span class="tooltip-help">?<div class="tooltip-content"><strong>Special Keys:</strong><br>Numbers (0-9), punctuation (!@#$%%), function keys (F1-F12), arrow keys, Tab, Return/Enter, Space, Backspace, Delete, Escape, etc.</div></span></div>
        </div>
    </div>

    <div class="charts-container">
        <div class="chart-box">
            <h2>Keystrokes per Day</h2>
            <canvas id="keystrokesChart"></canvas>
        </div>
        <div class="chart-box">
            <h2>Words per Day</h2>
            <canvas id="wordsChart"></canvas>
        </div>
    </div>

    <div class="charts-container" id="keyTypesCharts" style="display: %[1]s;">
        <div class="chart-box" style="grid-column: span 2;">
            <h2>Key Type Breakdown per Day</h2>
            <canvas id="keyTypesChart"></canvas>
        </div>
    </div>

    <div class="charts-container">
        <div class="chart-box" style="grid-column: span 2;">
            <h2 id="mouseChartTitle">Mouse Distance per Day</h2>
            <canvas id="mouseChart"></canvas>
        </div>
    </div>

    <div class="heatmap-container" id="heatmapSection">
        <div class="heatmap-box">
            <h2>Activity Heatmap (Hourly)</h2>
            <div class="hour-labels">
                %[2]s
            </div>
            <div class="heatmap" id="heatmapContainer">
            </div>
            <div class="legend">
                <span class="legend-text">Less</span>
                <div class="legend-box" style="background: #1a1a2e;"></div>
                <div class="legend-box" style="background: #2d4a3e;"></div>
                <div class="legend-box" style="background: #3d6b4f;"></div>
                <div class="legend-box" style="background: #5a9a6f;"></div>
                <div class="legend-box" style="background: #7bc96f;"></div>
                <span class="legend-text">More</span>
            </div>
        </div>
    </div>

    <div class="odometer-display" id="odometerDisplay">
        <div class="odometer-box">
            <h2>⏱️ Current Session</h2>
            <div class="odometer-status" id="odometerStatusBox">Inactive</div>
            <table class="odometer-table" id="currentSessionTable">
                <thead>
                    <tr>
                        <th>Metric</th>
                        <th>Value</th>
                    </tr>
                </thead>
                <tbody>
                    <tr>
                        <td>Start Time</td>
                        <td class="odometer-value" id="odometerStartTime">-</td>
                    </tr>
                    <tr>
                        <td>Keystrokes</td>
                        <td class="odometer-value" id="odometerKeystrokes">-</td>
                    </tr>
                    <tr>
                        <td>Words</td>
                        <td class="odometer-value" id="odometerWords">-</td>
                    </tr>
                    <tr>
                        <td>Mouse Clicks</td>
                        <td class="odometer-value" id="odometerClicks">-</td>
                    </tr>
                    <tr>
                        <td>Mouse Distance</td>
                        <td class="odometer-value" id="odometerDistance">-</td>
                    </tr>
                    <tr>
                        <td>Duration</td>
                        <td class="odometer-value" id="odometerDuration">-</td>
                    </tr>
                </tbody>
            </table>
        </div>
        <div class="odometer-box" style="margin-top: 20px;">
            <h2>📜 Session History</h2>
            <div id="historyTableContainer">
                <table class="odometer-table" id="historyTable">
                    <thead>
                        <tr>
                            <th>Start</th>
                            <th>End</th>
                            <th>Duration</th>
                            <th>Keystrokes</th>
                            <th>Words</th>
                            <th>Clicks</th>
                            <th>Distance</th>
                        </tr>
                    </thead>
                    <tbody id="historyTableBody">
                    </tbody>
                </table>
                <p id="noHistoryMsg" style="color: #888; text-align: center; padding: 20px;">No history yet. Complete an odometer session to see it here.</p>
            </div>
        </div>
    </div>

    <script>
        const data = {
            weekly: {
                labels: [%s],
                keystrokes: [%s],
                words: [%s],
                mouse: { feet: [%s], cars: [%s], fields: [%s] },
                letters: [%s],
                modifiers: [%s],
                special: [%s],
                totalKeystrokes: %d,
                totalWords: %d,
                totalMouseFeet: %.2f,
                totalClicks: %d,
                totalLetters: %d,
                totalModifiers: %d,
                totalSpecial: %d,
                avgKeystrokes: %.2f,
                avgWordsActive: %.2f,
                peakDayLabel: '%s',
                peakDayValue: %d,
                peakWordsLabel: '%s',
                peakWordsValue: %d,
                peakHourLabel: '%s',
                peakHourValue: %d,
                currentStreak: %d,
                longestStreak: %d,
                activeDays: %d,
                days: 7,
                heatmap: `+"`%s`"+`
            },
            monthly: {
                labels: [%s],
                keystrokes: [%s],
                words: [%s],
                mouse: { feet: [%s], cars: [%s], fields: [%s] },
                letters: [%s],
                modifiers: [%s],
                special: [%s],
                totalKeystrokes: %d,
                totalWords: %d,
                totalMouseFeet: %.2f,
                totalClicks: %d,
                totalLetters: %d,
                totalModifiers: %d,
                totalSpecial: %d,
                avgKeystrokes: %.2f,
                avgWordsActive: %.2f,
                peakDayLabel: '%s',
                peakDayValue: %d,
                peakWordsLabel: '%s',
                peakWordsValue: %d,
                peakHourLabel: '%s',
                peakHourValue: %d,
                currentStreak: %d,
                longestStreak: %d,
                activeDays: %d,
                days: 30,
                heatmap: `+"`%s`"+`
            },
            yearly: {
                labels: [%s],
                keystrokes: [%s],
                words: [%s],
                mouse: { feet: [%s], cars: [%s], fields: [%s] },
                letters: [%s],
                modifiers: [%s],
                special: [%s],
                totalKeystrokes: %d,
                totalWords: %d,
                totalMouseFeet: %.2f,
                totalClicks: %d,
                totalLetters: %d,
                totalModifiers: %d,
                totalSpecial: %d,
                avgKeystrokes: %.2f,
                avgWordsActive: %.2f,
                peakDayLabel: '%s',
                peakDayValue: %d,
                peakWordsLabel: '%s',
                peakWordsValue: %d,
                peakHourLabel: '%s',
                peakHourValue: %d,
                currentStreak: %d,
                longestStreak: %d,
                activeDays: %d,
                days: 365,
                heatmap: `+"`%s`"+`
            },
            odometer: {
                isActive: %t,
                startTime: '%s',
                keystrokes: %d,
                words: %d,
                clicks: %d,
                distanceFeet: %.2f,
                history: %s
            }
        };

        const unitLabels = { feet: 'feet', cars: 'car lengths', fields: 'frisbee fields' };

        let keystrokesChart, wordsChart, mouseChart, keyTypesChart;

        const chartConfig = {
            responsive: true,
            plugins: { legend: { display: false } },
            scales: {
                y: { beginAtZero: true, grid: { color: 'rgba(255,255,255,0.1)' }, ticks: { color: '#888' } },
                x: { grid: { display: false }, ticks: { color: '#888' } }
            }
        };

        const stackedChartConfig = {
            responsive: true,
            plugins: { legend: { display: true, labels: { color: '#888' } } },
            scales: {
                y: { beginAtZero: true, stacked: true, grid: { color: 'rgba(255,255,255,0.1)' }, ticks: { color: '#888' } },
                x: { stacked: true, grid: { display: false }, ticks: { color: '#888' } }
            }
        };

        function formatNumber(n) {
            if (n >= 1000000) return (n/1000000).toFixed(1) + 'M';
            if (n >= 1000) return (n/1000).toFixed(1) + 'K';
            return n.toString();
        }

        function formatDistance(feet, unit) {
            unit = unit || 'feet';
            switch(unit) {
                case 'cars':
                    const cars = feet / 15.0;
                    if (cars >= 1000) return (cars/1000).toFixed(1) + 'k cars';
                    if (cars >= 1) return cars.toFixed(0) + ' cars';
                    return cars.toFixed(1) + ' cars';
                case 'fields':
                    const fields = feet / 330.0;
                    if (fields >= 100) return fields.toFixed(0) + ' fields';
                    if (fields >= 1) return fields.toFixed(1) + ' fields';
                    return fields.toFixed(2) + ' fields';
                default:
                    if (feet >= 5280) return (feet/5280).toFixed(2) + ' mi';
                    if (feet >= 1) return feet.toFixed(0) + ' ft';
                    return (feet * 12).toFixed(0) + ' in';
            }
        }

        function updateCharts() {
            const period = document.getElementById('periodSelect').value;
            const unit = document.getElementById('unitSelect').value;

            // Handle odometer display separately
            if (period === 'odometer') {
                document.querySelectorAll('.stats-summary').forEach(el => el.style.display = 'none');
                document.getElementById('keyTypesStats').style.display = 'none';
                document.querySelectorAll('.charts-container').forEach(el => el.style.display = 'none');
                document.getElementById('heatmapSection').style.display = 'none';
                document.getElementById('odometerDisplay').style.display = 'block';
                updateOdometerDisplay();
                return;
            }

            // Show regular charts, hide odometer
            document.querySelectorAll('.stats-summary').forEach(el => el.style.display = 'flex');
            const keyTypesStats = document.getElementById('keyTypesStats');
            if (keyTypesStats) keyTypesStats.style.display = keyTypesStats.getAttribute('data-visible') === 'true' ? 'flex' : 'none';
            document.querySelectorAll('.charts-container').forEach(el => el.style.display = 'grid');
            document.getElementById('heatmapSection').style.display = 'block';
            document.getElementById('odometerDisplay').style.display = 'none';

            const d = data[period];

            document.getElementById('totalKeystrokes').textContent = formatNumber(d.totalKeystrokes);
            document.getElementById('totalWords').textContent = formatNumber(d.totalWords);
            document.getElementById('avgKeystrokes').textContent = formatNumber(Math.round(d.avgKeystrokes));
            document.getElementById('avgWords').textContent = formatNumber(Math.round(d.avgWordsActive));
            document.getElementById('totalMouse').textContent = formatDistance(d.totalMouseFeet, unit);
            document.getElementById('totalClicks').textContent = formatNumber(d.totalClicks);

            // Secondary stats row
            const peakDayEl = document.getElementById('peakDay');
            const peakDayLabelEl = document.getElementById('peakDayLabel');
            if (d.peakDayValue > 0) {
                peakDayEl.textContent = formatNumber(d.peakDayValue);
                peakDayLabelEl.textContent = 'Peak Day (' + d.peakDayLabel + ')';
            } else {
                peakDayEl.textContent = '—';
                peakDayLabelEl.textContent = 'Peak Day';
            }

            const peakWordsEl = document.getElementById('peakWords');
            const peakWordsLabelEl = document.getElementById('peakWordsLabel');
            if (d.peakWordsValue > 0) {
                peakWordsEl.textContent = formatNumber(d.peakWordsValue);
                peakWordsLabelEl.textContent = 'Peak Words (' + d.peakWordsLabel + ')';
            } else {
                peakWordsEl.textContent = '—';
                peakWordsLabelEl.textContent = 'Peak Words Day';
            }

            const peakHourEl = document.getElementById('peakHour');
            const peakHourLabelEl = document.getElementById('peakHourLabel');
            if (d.peakHourValue > 0) {
                peakHourEl.textContent = d.peakHourLabel;
                peakHourLabelEl.textContent = 'Most Active Hour (' + formatNumber(d.peakHourValue) + ')';
            } else {
                peakHourEl.textContent = '—';
                peakHourLabelEl.textContent = 'Most Active Hour';
            }

            document.getElementById('currentStreak').textContent = d.currentStreak + (d.currentStreak === 1 ? ' day' : ' days');
            document.getElementById('longestStreak').textContent = d.longestStreak + (d.longestStreak === 1 ? ' day' : ' days');
            document.getElementById('activeDays').textContent = d.activeDays;
            document.getElementById('activeDaysLabel').textContent = 'Active Days / ' + d.days;

            // Update key types stats if visible
            if (keyTypesStats && keyTypesStats.style.display !== 'none') {
                document.getElementById('totalLetters').textContent = formatNumber(d.totalLetters);
                document.getElementById('totalModifiers').textContent = formatNumber(d.totalModifiers);
                document.getElementById('totalSpecial').textContent = formatNumber(d.totalSpecial);
            }

            document.getElementById('mouseChartTitle').textContent = 'Mouse Distance per Day (' + unitLabels[unit] + ')';

            if (keystrokesChart) keystrokesChart.destroy();
            if (wordsChart) wordsChart.destroy();
            if (mouseChart) mouseChart.destroy();
            if (keyTypesChart) keyTypesChart.destroy();

            keystrokesChart = new Chart(document.getElementById('keystrokesChart'), {
                type: 'bar',
                data: {
                    labels: d.labels,
                    datasets: [{ data: d.keystrokes, backgroundColor: 'rgba(0, 210, 255, 0.6)', borderColor: 'rgba(0, 210, 255, 1)', borderWidth: 1, borderRadius: 4 }]
                },
                options: chartConfig
            });

            wordsChart = new Chart(document.getElementById('wordsChart'), {
                type: 'line',
                data: {
                    labels: d.labels,
                    datasets: [{ data: d.words, borderColor: 'rgba(122, 201, 111, 1)', backgroundColor: 'rgba(122, 201, 111, 0.2)', fill: true, tension: 0.4, pointRadius: 4, pointBackgroundColor: 'rgba(122, 201, 111, 1)' }]
                },
                options: chartConfig
            });

            // Key types chart (stacked bar) - only create if element exists
            const keyTypesCanvas = document.getElementById('keyTypesChart');
            if (keyTypesCanvas) {
                keyTypesChart = new Chart(keyTypesCanvas, {
                    type: 'bar',
                    data: {
                        labels: d.labels,
                        datasets: [
                            { label: 'Letters', data: d.letters, backgroundColor: 'rgba(122, 201, 111, 0.7)', borderColor: 'rgba(122, 201, 111, 1)', borderWidth: 1 },
                            { label: 'Modifiers', data: d.modifiers, backgroundColor: 'rgba(255, 152, 0, 0.7)', borderColor: 'rgba(255, 152, 0, 1)', borderWidth: 1 },
                            { label: 'Special', data: d.special, backgroundColor: 'rgba(233, 30, 99, 0.7)', borderColor: 'rgba(233, 30, 99, 1)', borderWidth: 1 }
                        ]
                    },
                    options: stackedChartConfig
                });
            }

            mouseChart = new Chart(document.getElementById('mouseChart'), {
                type: 'bar',
                data: {
                    labels: d.labels,
                    datasets: [{ data: d.mouse[unit], backgroundColor: 'rgba(255, 107, 107, 0.6)', borderColor: 'rgba(255, 107, 107, 1)', borderWidth: 1, borderRadius: 4 }]
                },
                options: chartConfig
            });

            document.getElementById('heatmapContainer').innerHTML = d.heatmap;
        }

        function formatDuration(totalSeconds) {
            const hours = Math.floor(totalSeconds / 3600);
            const minutes = Math.floor((totalSeconds %% 3600) / 60);
            const seconds = totalSeconds %% 60;
            let durationStr = '';
            if (hours > 0) durationStr += hours + 'h ';
            if (minutes > 0 || hours > 0) durationStr += minutes + 'm ';
            durationStr += seconds + 's';
            return durationStr || '0s';
        }

        function updateOdometerDisplay() {
            const od = data.odometer;
            const unit = document.getElementById('unitSelect').value;

            const statusBox = document.getElementById('odometerStatusBox');
            if (od.isActive) {
                statusBox.textContent = 'Active';
                statusBox.className = 'odometer-status active';
            } else {
                statusBox.textContent = 'Inactive';
                statusBox.className = 'odometer-status inactive';
            }

            document.getElementById('odometerStartTime').textContent = od.startTime || '-';
            document.getElementById('odometerKeystrokes').textContent = formatNumber(od.keystrokes);
            document.getElementById('odometerWords').textContent = formatNumber(od.words);
            document.getElementById('odometerClicks').textContent = formatNumber(od.clicks);
            document.getElementById('odometerDistance').textContent = formatDistance(od.distanceFeet, unit);

            // Calculate duration if active
            if (od.isActive && od.startTime) {
                const start = new Date(od.startTime);
                const now = new Date();
                const totalSeconds = Math.floor((now - start) / 1000);
                document.getElementById('odometerDuration').textContent = formatDuration(totalSeconds);
            } else {
                document.getElementById('odometerDuration').textContent = '-';
            }

            // Populate history table
            const historyBody = document.getElementById('historyTableBody');
            const noHistoryMsg = document.getElementById('noHistoryMsg');
            const historyTable = document.getElementById('historyTable');

            historyBody.innerHTML = '';
            if (od.history && od.history.length > 0) {
                historyTable.style.display = 'table';
                noHistoryMsg.style.display = 'none';
                od.history.forEach(function(entry) {
                    var row = document.createElement('tr');
                    row.innerHTML = '<td>' + entry.startTime + '</td>' +
                        '<td>' + entry.endTime + '</td>' +
                        '<td class="odometer-value">' + formatDuration(entry.durationSecs) + '</td>' +
                        '<td class="odometer-value">' + formatNumber(entry.keystrokes) + '</td>' +
                        '<td class="odometer-value">' + formatNumber(entry.words) + '</td>' +
                        '<td class="odometer-value">' + formatNumber(entry.clicks) + '</td>' +
                        '<td class="odometer-value">' + formatDistance(entry.distanceFeet, unit) + '</td>';
                    historyBody.appendChild(row);
                });
            } else {
                historyTable.style.display = 'none';
                noHistoryMsg.style.display = 'block';
            }
        }

        // Store initial visibility state for keyTypesStats
        (function() {
            const keyTypesStats = document.getElementById('keyTypesStats');
            if (keyTypesStats) {
                keyTypesStats.setAttribute('data-visible', keyTypesStats.style.display !== 'none');
            }
        })();

        updateCharts();
    </script>
</body>
</html>`,
		keyTypesDisplay,      // %[1]s - key types stats and charts display
		generateHourLabels(), // %[2]s - hour labels
		strings.Join(weeklyData.labels, ","),
		strings.Join(weeklyData.keystrokeData, ","),
		strings.Join(weeklyData.wordData, ","),
		weeklyMouseFeetStr,
		weeklyMouseCarsStr,
		weeklyMouseFieldsStr,
		strings.Join(weeklyData.letterData, ","),
		strings.Join(weeklyData.modifierData, ","),
		strings.Join(weeklyData.specialData, ","),
		weeklyData.totalKeystrokes,
		weeklyData.totalWords,
		pf(weeklyData.totalMouseDist),
		weeklyData.totalClicks,
		weeklyData.totalLetters,
		weeklyData.totalModifiers,
		weeklyData.totalSpecial,
		weeklyData.avgKeystrokes,
		weeklyData.avgWordsActive,
		weeklyData.peakDayLabel,
		weeklyData.peakDayValue,
		weeklyData.peakWordsLabel,
		weeklyData.peakWordsValue,
		weeklyData.peakHourLabel,
		weeklyData.peakHourValue,
		weeklyData.currentStreak,
		weeklyData.longestStreak,
		weeklyData.activeDays,
		weeklyData.heatmapHTML,
		strings.Join(monthlyData.labels, ","),
		strings.Join(monthlyData.keystrokeData, ","),
		strings.Join(monthlyData.wordData, ","),
		monthlyMouseFeetStr,
		monthlyMouseCarsStr,
		monthlyMouseFieldsStr,
		strings.Join(monthlyData.letterData, ","),
		strings.Join(monthlyData.modifierData, ","),
		strings.Join(monthlyData.specialData, ","),
		monthlyData.totalKeystrokes,
		monthlyData.totalWords,
		pf(monthlyData.totalMouseDist),
		monthlyData.totalClicks,
		monthlyData.totalLetters,
		monthlyData.totalModifiers,
		monthlyData.totalSpecial,
		monthlyData.avgKeystrokes,
		monthlyData.avgWordsActive,
		monthlyData.peakDayLabel,
		monthlyData.peakDayValue,
		monthlyData.peakWordsLabel,
		monthlyData.peakWordsValue,
		monthlyData.peakHourLabel,
		monthlyData.peakHourValue,
		monthlyData.currentStreak,
		monthlyData.longestStreak,
		monthlyData.activeDays,
		monthlyData.heatmapHTML,
		strings.Join(yearlyData.labels, ","),
		strings.Join(yearlyData.keystrokeData, ","),
		strings.Join(yearlyData.wordData, ","),
		yearlyMouseFeetStr,
		yearlyMouseCarsStr,
		yearlyMouseFieldsStr,
		strings.Join(yearlyData.letterData, ","),
		strings.Join(yearlyData.modifierData, ","),
		strings.Join(yearlyData.specialData, ","),
		yearlyData.totalKeystrokes,
		yearlyData.totalWords,
		pf(yearlyData.totalMouseDist),
		yearlyData.totalClicks,
		yearlyData.totalLetters,
		yearlyData.totalModifiers,
		yearlyData.totalSpecial,
		yearlyData.avgKeystrokes,
		yearlyData.avgWordsActive,
		yearlyData.peakDayLabel,
		yearlyData.peakDayValue,
		yearlyData.peakWordsLabel,
		yearlyData.peakWordsValue,
		yearlyData.peakHourLabel,
		yearlyData.peakHourValue,
		yearlyData.currentStreak,
		yearlyData.longestStreak,
		yearlyData.activeDays,
		yearlyData.heatmapHTML,
		odometerIsActive,
		odometerStartTime,
		odometerKeystrokes,
		odometerWords,
		odometerClicks,
		odometerDistanceFeet,
		historyJSON.String(),
	)

	dataDir, err := storage.LogDir()
	if err != nil {
		return "", err
	}
	htmlPath := filepath.Join(dataDir, "charts.html")
	if err := os.WriteFile(htmlPath, []byte(html), 0644); err != nil {
		return "", err
	}

	return htmlPath, nil
}

func generateHourLabels() string {
	var labels []string
	for h := 0; h < 24; h++ {
		if h%3 == 0 {
			labels = append(labels, fmt.Sprintf(`<div class="hour-label">%d</div>`, h))
		} else {
			labels = append(labels, `<div class="hour-label"></div>`)
		}
	}
	return strings.Join(labels, "\n                ")
}

func generateHeatmapHTML(hourlyData map[string][]storage.HourlyStats, days int) string {
	var maxVal int64 = 1
	for _, hours := range hourlyData {
		for _, h := range hours {
			if h.Keystrokes > maxVal {
				maxVal = h.Keystrokes
			}
		}
	}

	dates := make([]string, 0, len(hourlyData))
	for date := range hourlyData {
		dates = append(dates, date)
	}
	sort.Strings(dates)

	var rows []string
	for _, date := range dates {
		hours := hourlyData[date]
		t, _ := time.Parse("2006-01-02", date)
		dateLabel := t.Format("Mon Jan 2")

		var cells []string
		for _, h := range hours {
			color := getHeatmapColor(h.Keystrokes, maxVal)
			title := fmt.Sprintf("%s %d:00 - %d keystrokes", dateLabel, h.Hour, h.Keystrokes)
			cells = append(cells, fmt.Sprintf(
				`<div class="heatmap-cell" style="background: %s;" title="%s"></div>`,
				color, title,
			))
		}

		rows = append(rows, fmt.Sprintf(
			`<div class="heatmap-row"><div class="heatmap-label">%s</div>%s</div>`,
			dateLabel,
			strings.Join(cells, ""),
		))
	}

	return strings.Join(rows, "\n                ")
}

func getHeatmapColor(value, max int64) string {
	if value == 0 {
		return "#1a1a2e"
	}
	ratio := float64(value) / float64(max)
	if ratio < 0.25 {
		return "#2d4a3e"
	} else if ratio < 0.5 {
		return "#3d6b4f"
	} else if ratio < 0.75 {
		return "#5a9a6f"
	}
	return "#7bc96f"
}

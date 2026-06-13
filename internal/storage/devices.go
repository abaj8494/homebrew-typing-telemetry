package storage

// Device ingest storage (v1.4142). A deliberately separate source from the
// macOS daily_summary: external devices (e.g. a reMarkable tablet) PUT absolute
// daily aggregates over the Tailscale-bound ingest API, which land here via
// UpsertDeviceDay. The device classifies its own keys (Linux evdev, not macOS
// CGKeyCodes), so letters/modifiers/special/words are opaque counts on this
// side — never re-classified here. Storage is INSERT OR REPLACE so a retried
// PUT over a flaky link can never double-count; the latest PUT wins.

import (
	"database/sql"
	"time"
)

// DeviceDayCounts is the absolute per-day aggregate for a device. All fields
// are running totals for the device-local date, not deltas.
type DeviceDayCounts struct {
	Keystrokes int64 `json:"keystrokes"`
	Letters    int64 `json:"letters"`
	Modifiers  int64 `json:"modifiers"`
	Special    int64 `json:"special"`
	Words      int64 `json:"words"`
	ActiveMs   int64 `json:"active_ms"`
}

// DeviceDay pairs a device-local date with its counts.
type DeviceDay struct {
	Date string `json:"date"`
	DeviceDayCounts
}

// DeviceInfo is a registered device's identity plus when it last reported.
type DeviceInfo struct {
	DeviceID string `json:"device_id"`
	Name     string `json:"name"`
	LastSeen string `json:"last_seen"`
}

// UpsertDevice registers a device if absent and touches its last_seen. A
// non-empty name overwrites the stored name; an empty name leaves it untouched
// so a bare PUT never clears a previously-set friendly name.
func (s *Store) UpsertDevice(deviceID, name string) error {
	now := time.Now().Format(time.RFC3339)
	if _, err := s.db.Exec(
		`INSERT OR IGNORE INTO devices (device_id, name, last_seen) VALUES (?, ?, ?)`,
		deviceID, name, now,
	); err != nil {
		return err
	}
	if name != "" {
		if _, err := s.db.Exec(
			`UPDATE devices SET name = ?, last_seen = ? WHERE device_id = ?`,
			name, now, deviceID,
		); err != nil {
			return err
		}
		return nil
	}
	_, err := s.db.Exec(`UPDATE devices SET last_seen = ? WHERE device_id = ?`, now, deviceID)
	return err
}

// UpsertDeviceDay replaces the entire row for (deviceID, date) with the given
// absolute counts, then auto-registers the device and bumps its last_seen. This
// is the ingest workhorse: first contact from an unknown device self-registers.
func (s *Store) UpsertDeviceDay(deviceID, date string, c DeviceDayCounts) error {
	if _, err := s.db.Exec(`
		INSERT OR REPLACE INTO device_daily_summary
			(device_id, date, keystrokes, letters, modifiers, special, words, active_ms, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, deviceID, date, c.Keystrokes, c.Letters, c.Modifiers, c.Special, c.Words, c.ActiveMs,
		time.Now().Format(time.RFC3339)); err != nil {
		return err
	}
	return s.UpsertDevice(deviceID, "")
}

// GetDeviceDay returns the counts for one device-day, or nil if absent.
func (s *Store) GetDeviceDay(deviceID, date string) (*DeviceDayCounts, error) {
	var c DeviceDayCounts
	err := s.db.QueryRow(`
		SELECT keystrokes, letters, modifiers, special, words, active_ms
		FROM device_daily_summary WHERE device_id = ? AND date = ?
	`, deviceID, date).Scan(&c.Keystrokes, &c.Letters, &c.Modifiers, &c.Special, &c.Words, &c.ActiveMs)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// GetDeviceDays returns a device's days on or after sinceDate (empty = all),
// newest first.
func (s *Store) GetDeviceDays(deviceID, sinceDate string) ([]DeviceDay, error) {
	var rows *sql.Rows
	var err error
	if sinceDate == "" {
		rows, err = s.db.Query(`
			SELECT date, keystrokes, letters, modifiers, special, words, active_ms
			FROM device_daily_summary WHERE device_id = ?
			ORDER BY date DESC
		`, deviceID)
	} else {
		rows, err = s.db.Query(`
			SELECT date, keystrokes, letters, modifiers, special, words, active_ms
			FROM device_daily_summary WHERE device_id = ? AND date >= ?
			ORDER BY date DESC
		`, deviceID, sinceDate)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []DeviceDay
	for rows.Next() {
		var d DeviceDay
		if err := rows.Scan(&d.Date, &d.Keystrokes, &d.Letters, &d.Modifiers,
			&d.Special, &d.Words, &d.ActiveMs); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// ListDevices returns all registered devices, most-recently-seen first.
func (s *Store) ListDevices() ([]DeviceInfo, error) {
	rows, err := s.db.Query(`
		SELECT device_id, COALESCE(name, ''), COALESCE(last_seen, '')
		FROM devices ORDER BY last_seen DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []DeviceInfo
	for rows.Next() {
		var d DeviceInfo
		if err := rows.Scan(&d.DeviceID, &d.Name, &d.LastSeen); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// DeleteDeviceDay erases a single device-day. Absent rows are a no-op.
func (s *Store) DeleteDeviceDay(deviceID, date string) error {
	_, err := s.db.Exec(
		`DELETE FROM device_daily_summary WHERE device_id = ? AND date = ?`,
		deviceID, date,
	)
	return err
}

// DeleteDevice forgets a device: its registration row plus all of its daily
// rows.
func (s *Store) DeleteDevice(deviceID string) error {
	if _, err := s.db.Exec(`DELETE FROM device_daily_summary WHERE device_id = ?`, deviceID); err != nil {
		return err
	}
	_, err := s.db.Exec(`DELETE FROM devices WHERE device_id = ?`, deviceID)
	return err
}

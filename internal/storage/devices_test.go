package storage

import "testing"

func TestUpsertDeviceDayRoundTrip(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	want := DeviceDayCounts{
		Keystrokes: 4210, Letters: 3100, Modifiers: 620,
		Special: 490, Words: 780, ActiveMs: 1380000,
	}
	if err := store.UpsertDeviceDay("ferrari", "2026-06-13", want); err != nil {
		t.Fatalf("UpsertDeviceDay: %v", err)
	}

	got, err := store.GetDeviceDay("ferrari", "2026-06-13")
	if err != nil {
		t.Fatalf("GetDeviceDay: %v", err)
	}
	if got == nil {
		t.Fatal("GetDeviceDay returned nil for a present row")
	}
	if *got != want {
		t.Fatalf("round-trip mismatch: got %+v want %+v", *got, want)
	}

	// First contact self-registers the device.
	devices, err := store.ListDevices()
	if err != nil {
		t.Fatalf("ListDevices: %v", err)
	}
	if len(devices) != 1 || devices[0].DeviceID != "ferrari" {
		t.Fatalf("expected ferrari auto-registered, got %+v", devices)
	}
	if devices[0].LastSeen == "" {
		t.Fatal("expected last_seen to be set on first contact")
	}
}

func TestUpsertDeviceDayIdempotent(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	c := DeviceDayCounts{Keystrokes: 100, Words: 20}
	for i := 0; i < 3; i++ {
		if err := store.UpsertDeviceDay("ferrari", "2026-06-13", c); err != nil {
			t.Fatalf("UpsertDeviceDay #%d: %v", i, err)
		}
	}

	// Absolute semantics: three identical PUTs must leave exactly one row with
	// the same values — never accumulate.
	days, err := store.GetDeviceDays("ferrari", "")
	if err != nil {
		t.Fatalf("GetDeviceDays: %v", err)
	}
	if len(days) != 1 {
		t.Fatalf("expected 1 row, got %d", len(days))
	}
	if days[0].Keystrokes != 100 || days[0].Words != 20 {
		t.Fatalf("idempotency broken: got %+v", days[0])
	}
}

func TestUpsertDeviceDayReplacesNotAdds(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	if err := store.UpsertDeviceDay("ferrari", "2026-06-13", DeviceDayCounts{Keystrokes: 100}); err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	// A later absolute total fully replaces the prior row.
	if err := store.UpsertDeviceDay("ferrari", "2026-06-13", DeviceDayCounts{Keystrokes: 250}); err != nil {
		t.Fatalf("second upsert: %v", err)
	}
	got, err := store.GetDeviceDay("ferrari", "2026-06-13")
	if err != nil {
		t.Fatalf("GetDeviceDay: %v", err)
	}
	if got == nil || got.Keystrokes != 250 {
		t.Fatalf("expected replacement to 250, got %+v", got)
	}
}

func TestGetDeviceDayAbsent(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	got, err := store.GetDeviceDay("ghost", "2026-06-13")
	if err != nil {
		t.Fatalf("GetDeviceDay: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil for absent device-day, got %+v", got)
	}
}

func TestGetDeviceDaysSinceAndOrder(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	for _, d := range []string{"2026-06-11", "2026-06-12", "2026-06-13"} {
		if err := store.UpsertDeviceDay("ferrari", d, DeviceDayCounts{Keystrokes: 1}); err != nil {
			t.Fatalf("upsert %s: %v", d, err)
		}
	}

	all, err := store.GetDeviceDays("ferrari", "")
	if err != nil {
		t.Fatalf("GetDeviceDays all: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("expected 3 days, got %d", len(all))
	}
	// Newest first.
	if all[0].Date != "2026-06-13" || all[2].Date != "2026-06-11" {
		t.Fatalf("expected newest-first order, got %s..%s", all[0].Date, all[2].Date)
	}

	since, err := store.GetDeviceDays("ferrari", "2026-06-12")
	if err != nil {
		t.Fatalf("GetDeviceDays since: %v", err)
	}
	if len(since) != 2 {
		t.Fatalf("expected 2 days since 2026-06-12, got %d", len(since))
	}
}

func TestUpsertDeviceNamePreservation(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	if err := store.UpsertDevice("ferrari", "reMarkable Paper Pro"); err != nil {
		t.Fatalf("UpsertDevice with name: %v", err)
	}
	// A subsequent bare upsert (empty name, as UpsertDeviceDay does) must not
	// clear the friendly name.
	if err := store.UpsertDevice("ferrari", ""); err != nil {
		t.Fatalf("UpsertDevice bare: %v", err)
	}
	devices, err := store.ListDevices()
	if err != nil {
		t.Fatalf("ListDevices: %v", err)
	}
	if len(devices) != 1 || devices[0].Name != "reMarkable Paper Pro" {
		t.Fatalf("expected name preserved, got %+v", devices)
	}
}

func TestDeleteDeviceCascades(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	if err := store.UpsertDeviceDay("ferrari", "2026-06-13", DeviceDayCounts{Keystrokes: 1}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if err := store.UpsertDeviceDay("ferrari", "2026-06-12", DeviceDayCounts{Keystrokes: 1}); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	if err := store.DeleteDevice("ferrari"); err != nil {
		t.Fatalf("DeleteDevice: %v", err)
	}

	devices, err := store.ListDevices()
	if err != nil {
		t.Fatalf("ListDevices: %v", err)
	}
	if len(devices) != 0 {
		t.Fatalf("expected device removed, got %+v", devices)
	}
	days, err := store.GetDeviceDays("ferrari", "")
	if err != nil {
		t.Fatalf("GetDeviceDays: %v", err)
	}
	if len(days) != 0 {
		t.Fatalf("expected daily rows cascaded, got %d", len(days))
	}
}

func TestDeleteDeviceDay(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	if err := store.UpsertDeviceDay("ferrari", "2026-06-13", DeviceDayCounts{Keystrokes: 1}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if err := store.DeleteDeviceDay("ferrari", "2026-06-13"); err != nil {
		t.Fatalf("DeleteDeviceDay: %v", err)
	}
	got, err := store.GetDeviceDay("ferrari", "2026-06-13")
	if err != nil {
		t.Fatalf("GetDeviceDay: %v", err)
	}
	if got != nil {
		t.Fatalf("expected day deleted, got %+v", got)
	}
	// Deleting a day leaves the device registered.
	devices, err := store.ListDevices()
	if err != nil {
		t.Fatalf("ListDevices: %v", err)
	}
	if len(devices) != 1 {
		t.Fatalf("expected device still registered, got %+v", devices)
	}
}

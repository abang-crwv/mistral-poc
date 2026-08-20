package store

import (
	"context"
	"testing"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestMigrate_EvidenceHasDeviceslotColumn(t *testing.T) {
	s := openTestStore(t)
	row := s.db.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM pragma_table_info('evidence') WHERE name = 'deviceslot'`)
	var n int
	if err := row.Scan(&n); err != nil {
		t.Fatalf("introspect: %v", err)
	}
	if n != 1 {
		t.Errorf("evidence.deviceslot column missing")
	}
}

func TestMigrate_RackDeviceslotsTableExists(t *testing.T) {
	s := openTestStore(t)
	row := s.db.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='rack_deviceslots'`)
	var n int
	if err := row.Scan(&n); err != nil {
		t.Fatalf("introspect: %v", err)
	}
	if n != 1 {
		t.Errorf("rack_deviceslots table missing")
	}
}

func TestMigrate_BMNIgnoresTableExists(t *testing.T) {
	s := openTestStore(t)
	row := s.db.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='bmn_ignores'`)
	var n int
	if err := row.Scan(&n); err != nil {
		t.Fatalf("introspect: %v", err)
	}
	if n != 1 {
		t.Errorf("bmn_ignores table missing")
	}
}

func TestMigrate_Idempotent(t *testing.T) {
	s := openTestStore(t)
	// Run migrate twice — second call must not error.
	if err := s.migrate(context.Background()); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
}

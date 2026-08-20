package store

import (
	"context"
	"path/filepath"
	"testing"
)

func TestUpsertTemplate_InsertThenUpdate(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "fwqual.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	if err := s.UpsertTemplate(ctx, "demo", 1, "schema: qac.template/v1\nid: demo", `{"id":"demo","version":1}`); err != nil {
		t.Fatalf("Upsert v1: %v", err)
	}

	got, ok, err := s.GetTemplate(ctx, "demo")
	if err != nil {
		t.Fatalf("Get after insert: %v", err)
	}
	if !ok {
		t.Fatal("Get returned ok=false after insert")
	}
	if got.Version != 1 {
		t.Fatalf("Version = %d, want 1", got.Version)
	}

	if err := s.UpsertTemplate(ctx, "demo", 2, "v2-body", `{"id":"demo","version":2}`); err != nil {
		t.Fatalf("Upsert v2: %v", err)
	}
	got, _, err = s.GetTemplate(ctx, "demo")
	if err != nil {
		t.Fatalf("Get after update: %v", err)
	}
	if got.Version != 2 {
		t.Fatalf("Version after update = %d, want 2", got.Version)
	}
	if got.BodyYAML != "v2-body" {
		t.Fatalf("BodyYAML = %q, want v2-body", got.BodyYAML)
	}
}

func TestListTemplates_ReturnsSummaries(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "fwqual.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	mk := func(id string, v int) {
		body := `id: ` + id
		parsed := `{"id":"` + id + `","version":` + itoa(v) + `,"title":"T"}`
		if err := s.UpsertTemplate(ctx, id, v, body, parsed); err != nil {
			t.Fatalf("Upsert %s: %v", id, err)
		}
	}
	mk("alpha", 1)
	mk("beta", 3)

	got, err := s.ListTemplates(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	// Result should be ordered by id ascending for stable output.
	if got[0].ID != "alpha" || got[1].ID != "beta" {
		t.Fatalf("ids = [%q, %q], want [alpha, beta]", got[0].ID, got[1].ID)
	}
	if got[0].Title != "T" {
		t.Fatalf("Title = %q, want T", got[0].Title)
	}
}

func TestGetTemplate_NotFound(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "fwqual.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	_, ok, err := s.GetTemplate(ctx, "ghost")
	if err != nil {
		t.Fatalf("Get on missing id returned err: %v", err)
	}
	if ok {
		t.Fatal("ok = true on missing id; want false")
	}
}

// itoa is a tiny inline helper — strconv.Itoa would also work; this keeps
// the test self-contained without an extra import.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

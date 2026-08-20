package lokiclient

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestResolveLokiUID(t *testing.T) {
	cases := map[string]string{
		"us-central-03a": "af20t1e3rwcu8c",
		"US-WEST-04":     "af20t1e3rwcu8c",
		"rno2a":          "af20t1e3rwcu8c",
		"us-east-01a":    "bf9ywk3urkyrkf",
		"ca-east-01a":    "bf9ywk3urkyrkf",
		"eu-south-01a":   "ef9dupn3z3s3kd",
		"mars-01":        "",
	}
	for region, want := range cases {
		if got := ResolveLokiUID(region); got != want {
			t.Errorf("ResolveLokiUID(%q) = %q, want %q", region, got, want)
		}
	}
}

func TestParseL11Launch(t *testing.T) {
	line := `2026-07-03T05:28:48.109201301Z stdout F {"time":"2026-07-03T05:28:48.109163101Z","level":"INFO","msg":"AWX job launched, see details for jobID[810195]: https://awx.us-central-03a.int.coreweave.com/#/jobs/playbook/810195/output","reconciler_type":"action","reconciler_name":"action-l11-controller","event_type":"Normal","reason":"RLCCAWXJobStarted","name":"dhb-r055-us-central-03a"}`
	ref, ok := parseL11Launch(line)
	if !ok {
		t.Fatal("parseL11Launch returned ok=false for a valid launch line")
	}
	if ref.Rack != "dhb-r055-us-central-03a" || ref.JobID != 810195 || ref.Region != "us-central-03a" {
		t.Errorf("ref = %+v, want dhb-r055.../810195/us-central-03a", ref)
	}
	if ref.LaunchedAt.IsZero() {
		t.Errorf("LaunchedAt not parsed")
	}
	// A non-launch line (e.g. a reconcile diff) is rejected.
	if _, ok := parseL11Launch(`2026-07-03T05:00:00Z stdout F {"msg":"reconcile","reason":"","name":"x"}`); ok {
		t.Errorf("parseL11Launch accepted a non-launch line")
	}
	// A line with no JSON is rejected.
	if _, ok := parseL11Launch("garbage without brace"); ok {
		t.Errorf("parseL11Launch accepted a line with no JSON")
	}
}

func TestMapClient_L11FieldDiagJobs(t *testing.T) {
	c := NewMapClient(SeedDemoL11Logs())
	refs, err := L11FieldDiagJobs(context.Background(), c, "us-east-01a", "dh3-r012-us-east-01a", 48*time.Hour)
	if err != nil {
		t.Fatalf("L11FieldDiagJobs: %v", err)
	}
	if len(refs) != 2 {
		t.Fatalf("refs = %d, want 2 (two seeded attempts)", len(refs))
	}
	if refs[0].JobID == 0 || refs[0].Rack != "dh3-r012-us-east-01a" || refs[0].Region != "us-east-01a" {
		t.Errorf("ref[0] = %+v", refs[0])
	}
	// A rack with no l11 launches -> empty, no error.
	none, err := L11FieldDiagJobs(context.Background(), c, "us-east-01a", "dh9-r999-us-east-01a", 48*time.Hour)
	if err != nil || len(none) != 0 {
		t.Errorf("unknown rack: refs=%d err=%v, want 0/nil", len(none), err)
	}
}

func TestMapClient_FailingRegion(t *testing.T) {
	c := NewMapClient(SeedDemoL11Logs())
	_, err := L11FieldDiagJobs(context.Background(), c, FailingSourceRegion, "r", time.Hour)
	if !errors.Is(err, ErrSourceUnavailable) {
		t.Fatalf("err = %v, want ErrSourceUnavailable", err)
	}
}

func TestHTTPClient_QueryRangeParsesStreams(t *testing.T) {
	var gotAuth, gotQuery, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotQuery = r.URL.Query().Get("query")
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "success",
			"data": map[string]any{
				"resultType": "streams",
				"result": []map[string]any{{
					"stream": map[string]string{"app": "rack-lifecycle-controller"},
					"values": [][2]string{
						{strconv.FormatInt(time.Now().UnixNano(), 10), "line-newest"},
					},
				}},
			},
		})
	}))
	defer srv.Close()

	c := NewHTTPClient(Config{BaseURL: srv.URL, Token: "tok", HTTPClient: srv.Client()})
	entries, err := c.QueryRange(context.Background(), "us-central-03a", `{app="x"}`, time.Now().Add(-time.Hour), time.Now(), 10)
	if err != nil {
		t.Fatalf("QueryRange: %v", err)
	}
	if len(entries) != 1 || entries[0].Line != "line-newest" {
		t.Fatalf("entries = %+v, want one line-newest", entries)
	}
	if gotAuth != "Bearer tok" {
		t.Errorf("auth header = %q, want Bearer tok", gotAuth)
	}
	if gotQuery != `{app="x"}` {
		t.Errorf("query = %q", gotQuery)
	}
	// Uses the datasource-proxy path with the resolved us-central Loki UID.
	if !strings.Contains(gotPath, "/api/datasources/proxy/uid/af20t1e3rwcu8c/loki/api/v1/query_range") {
		t.Errorf("path = %q, want the af20t1e3rwcu8c datasource-proxy path", gotPath)
	}
}

func TestHTTPClient_UnknownRegion(t *testing.T) {
	c := NewHTTPClient(Config{Token: "t"})
	_, err := c.QueryRange(context.Background(), "mars-01", `{app="x"}`, time.Now().Add(-time.Hour), time.Now(), 10)
	if !errors.Is(err, ErrUnknownRegion) {
		t.Fatalf("err = %v, want ErrUnknownRegion", err)
	}
}

func TestHTTPClient_Non200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	c := NewHTTPClient(Config{BaseURL: srv.URL, Token: "", HTTPClient: srv.Client()})
	_, err := c.QueryRange(context.Background(), "us-east-01a", `{app="x"}`, time.Now().Add(-time.Hour), time.Now(), 10)
	if !errors.Is(err, ErrSourceUnavailable) {
		t.Fatalf("err = %v, want ErrSourceUnavailable", err)
	}
}

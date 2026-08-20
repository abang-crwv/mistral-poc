package lifecycleclient

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"qac/internal/vm"
)

// newTestPromClient builds a PromClient whose single "test" super-region
// points at srv, with creds present so the authed path is exercised.
func newTestPromClient(srv *httptest.Server) *PromClient {
	return NewPromClient(vm.New(vm.Config{
		AuthedURLs: map[string]string{"test": srv.URL},
		Username:   "u",
		Password:   "p",
		HTTPClient: srv.Client(),
	}))
}

func TestPromClient_QueryRLCCIgnored_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got == "" {
			t.Errorf("Authorization header missing")
		}
		if r.URL.Path != "/api/v1/query" {
			t.Errorf("path = %q, want /api/v1/query", r.URL.Path)
		}
		q := r.URL.Query().Get("query")
		if !strings.Contains(q, `rack="dh3-r012-us-east-01a"`) {
			t.Errorf("query missing rack label: %q", q)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "success",
			"data": map[string]any{
				"resultType": "vector",
				"result": []map[string]any{
					{
						"metric": map[string]string{
							"deviceslot": "dh3-r012-node-07-us-east-01a",
							"bmn":        "s90txs57",
						},
						"value": []any{1709337600.0, "1"},
					},
				},
			},
		})
	}))
	defer srv.Close()
	c := newTestPromClient(srv)
	out, err := c.QueryRLCCIgnored(context.Background(),
		RackKey{Rack: "dh3-r012-us-east-01a", Zone: "us-east-01a"})
	if err != nil {
		t.Fatalf("QueryRLCCIgnored: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("len = %d, want 1", len(out))
	}
	if out[0].Deviceslot != "dh3-r012-node-07-us-east-01a" {
		t.Errorf("Deviceslot = %q", out[0].Deviceslot)
	}
	if out[0].BMNName != "s90txs57" {
		t.Errorf("BMNName = %q", out[0].BMNName)
	}
}

func TestPromClient_QueryActionState_BuildsQueryAndAuth(t *testing.T) {
	var gotQueries []string
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQueries = append(gotQueries, r.URL.Query().Get("query"))
		gotAuth = r.Header.Get("Authorization")
		// First deviceslot present (=1), second absent.
		if strings.Contains(r.URL.Query().Get("query"), "node-01") {
			_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[{"metric":{"node":"node-01"}}]}}`))
			return
		}
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[]}}`))
	}))
	defer srv.Close()

	c := newTestPromClient(srv)
	got, err := c.QueryActionState(context.Background(),
		[]string{"dh3-r012-node-01-us-east-01a", "dh3-r012-node-02-us-east-01a"}, "l11-fielddiag")
	if err != nil {
		t.Fatalf("QueryActionState: %v", err)
	}
	if !got["dh3-r012-node-01-us-east-01a"] {
		t.Error("node-01 should be complete (metric present)")
	}
	if got["dh3-r012-node-02-us-east-01a"] {
		t.Error("node-02 should be incomplete (no metric)")
	}
	if len(gotQueries) != 2 {
		t.Fatalf("expected 2 queries, got %d", len(gotQueries))
	}
	// r.URL.Query().Get decodes percent-encoding, so the captured query
	// carries plain double quotes (state="l11-fielddiag"), not the escaped
	// (\") or percent (%22) forms. Accept any of the three so the assertion
	// holds whether the server reads decoded values or raw query bytes.
	if !strings.Contains(gotQueries[0], `state="l11-fielddiag"`) &&
		!strings.Contains(gotQueries[0], `state=\"l11-fielddiag\"`) &&
		!strings.Contains(gotQueries[0], "state=%22l11-fielddiag%22") {
		t.Errorf("query missing state label: %s", gotQueries[0])
	}
	if gotAuth == "" {
		t.Error("missing basic auth header")
	}
}

func TestPromClient_QueryFLCCState(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// One series for the node-07 deviceslot (state=fail), none for node-08.
		if strings.Contains(r.URL.RawQuery, "node-07") {
			_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[
				{"metric":{"device_slot":"dh3-r012-node-07-us-east-01a","state":"fail","step":"fielddiag","prev_state":"fielddiag","prev_step":"fielddiag","workflow":"provision"}}
			]}}`))
			return
		}
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[]}}`))
	}))
	defer srv.Close()

	c := newTestPromClient(srv)
	got, err := c.QueryFLCCState(context.Background(), []string{"dh3-r012-node-07-us-east-01a", "dh3-r012-node-08-us-east-01a"})
	if err != nil {
		t.Fatalf("QueryFLCCState: %v", err)
	}
	if got["dh3-r012-node-07-us-east-01a"].State != "fail" {
		t.Errorf("node-07 state = %q, want fail", got["dh3-r012-node-07-us-east-01a"].State)
	}
	if got["dh3-r012-node-07-us-east-01a"].Step != "fielddiag" {
		t.Errorf("node-07 step = %q, want fielddiag", got["dh3-r012-node-07-us-east-01a"].Step)
	}
	if got["dh3-r012-node-08-us-east-01a"].State != "" {
		t.Errorf("node-08 = %+v, want zero (no series)", got["dh3-r012-node-08-us-east-01a"])
	}
}

func TestPromClient_ServerError_ReturnsErr(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	c := newTestPromClient(srv)
	_, err := c.QueryRLCCIgnored(context.Background(),
		RackKey{Rack: "r", Zone: "z"})
	// The single "test" region 500s with no vmui fallback, so every
	// region failed → vm.QueryVector wraps ErrUpstream.
	if !errors.Is(err, vm.ErrUpstream) {
		t.Errorf("err = %v, want ErrUpstream", err)
	}
}

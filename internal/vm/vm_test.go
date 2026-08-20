package vm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestDefaults_FourSuperRegionsBoth(t *testing.T) {
	authed, unauthed := Defaults()
	want := []string{"eu-south", "us-east", "us-lab", "us-west"}

	var gotA []string
	for k := range authed {
		gotA = append(gotA, k)
	}
	sort.Strings(gotA)
	if strings.Join(gotA, ",") != strings.Join(want, ",") {
		t.Errorf("authed keys = %v, want %v", gotA, want)
	}
	if authed["us-lab"] != "http://vmauth.us-lab.int.coreweave.com:8427/prometheus" {
		t.Errorf("authed us-lab = %q", authed["us-lab"])
	}

	var gotU []string
	for k := range unauthed {
		gotU = append(gotU, k)
	}
	sort.Strings(gotU)
	if strings.Join(gotU, ",") != strings.Join(want, ",") {
		t.Errorf("unauthed keys = %v, want %v", gotU, want)
	}
	if unauthed["eu-south"] != "http://vmui.eu-south.int.coreweave.com/select/0/prometheus" {
		t.Errorf("unauthed eu-south = %q", unauthed["eu-south"])
	}
}

// vmServer returns an httptest server that replies to /api/v1/query with
// the given series label maps. wantAuth toggles a basic-auth assertion.
func vmServer(t *testing.T, wantAuth bool, series ...map[string]string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/query" {
			t.Errorf("path = %q, want /api/v1/query", r.URL.Path)
		}
		if _, _, ok := r.BasicAuth(); ok != wantAuth {
			t.Errorf("basic auth present = %v, want %v", ok, wantAuth)
		}
		rows := make([]map[string]any, 0, len(series))
		for _, m := range series {
			rows = append(rows, map[string]any{"metric": m, "value": []any{0, "1"}})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "success",
			"data":   map[string]any{"resultType": "vector", "result": rows},
		})
	}))
}

func TestQueryVector_UnionAcrossSuperRegions(t *testing.T) {
	east := vmServer(t, true, map[string]string{"deviceslot": "e-1"})
	defer east.Close()
	west := vmServer(t, true, map[string]string{"deviceslot": "w-1"})
	defer west.Close()

	c := New(Config{
		AuthedURLs: map[string]string{"us-east": east.URL, "us-west": west.URL},
		Username:   "u", Password: "p",
		HTTPClient: east.Client(),
	})
	got, err := c.QueryVector(context.Background(), "up")
	if err != nil {
		t.Fatalf("QueryVector: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2 (union of both regions)", len(got))
	}
	slots := map[string]bool{got[0]["deviceslot"]: true, got[1]["deviceslot"]: true}
	if !slots["e-1"] || !slots["w-1"] {
		t.Errorf("union = %v, want e-1 and w-1", got)
	}
}

func TestQueryVector_AuthedFailsFallsBackToVMUI(t *testing.T) {
	authed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer authed.Close()
	vmui := vmServer(t, false, map[string]string{"deviceslot": "via-vmui"})
	defer vmui.Close()

	c := New(Config{
		AuthedURLs:   map[string]string{"us-west": authed.URL},
		UnauthedURLs: map[string]string{"us-west": vmui.URL},
		Username:     "u", Password: "p",
		HTTPClient: vmui.Client(),
	})
	got, err := c.QueryVector(context.Background(), "up")
	if err != nil {
		t.Fatalf("QueryVector: %v", err)
	}
	if len(got) != 1 || got[0]["deviceslot"] != "via-vmui" {
		t.Fatalf("got %v, want one series via-vmui", got)
	}
}

func TestQueryVector_AllErroredReturnsErrUpstream(t *testing.T) {
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer bad.Close()

	c := New(Config{
		AuthedURLs: map[string]string{"us-west": bad.URL},
		Username:   "u", Password: "p",
		HTTPClient: bad.Client(),
	})
	_, err := c.QueryVector(context.Background(), "up")
	if !errors.Is(err, ErrUpstream) {
		t.Fatalf("err = %v, want ErrUpstream", err)
	}
}

func TestQueryVector_MissDistinctFromOutage(t *testing.T) {
	empty := vmServer(t, true) // 200 success, zero series
	defer empty.Close()

	c := New(Config{
		AuthedURLs: map[string]string{"us-west": empty.URL},
		Username:   "u", Password: "p",
		HTTPClient: empty.Client(),
	})
	got, err := c.QueryVector(context.Background(), "up")
	if err != nil {
		t.Fatalf("clean miss should be nil error, got %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %v, want empty", got)
	}
}

func TestNew_NoCredsUsesUnauthedAndWarnsOnce(t *testing.T) {
	var buf bytes.Buffer
	prev := SetWarnWriter(&buf)
	defer SetWarnWriter(prev)
	ResetWarnOnce()
	t.Cleanup(ResetWarnOnce)

	vmui := vmServer(t, false, map[string]string{"deviceslot": "noauth"})
	defer vmui.Close()

	c := New(Config{
		UnauthedURLs: map[string]string{"us-west": vmui.URL},
		// no Username/Password → unauthed mode
		HTTPClient: vmui.Client(),
	})
	got, err := c.QueryVector(context.Background(), "up")
	if err != nil {
		t.Fatalf("QueryVector: %v", err)
	}
	if len(got) != 1 || got[0]["deviceslot"] != "noauth" {
		t.Fatalf("got %v, want one series noauth (no basic auth sent)", got)
	}
	if !strings.Contains(buf.String(), "falling back to unauthed vmui") {
		t.Errorf("warning not emitted: %q", buf.String())
	}
	// Building a second unauthed client must NOT warn again.
	buf.Reset()
	_ = New(Config{UnauthedURLs: map[string]string{"us-west": vmui.URL}, HTTPClient: vmui.Client()})
	if buf.Len() != 0 {
		t.Errorf("second warning emitted: %q", buf.String())
	}
}

func TestNew_TrimsTrailingSlash(t *testing.T) {
	srv := vmServer(t, true, map[string]string{"deviceslot": "x"})
	defer srv.Close()
	// A trailing slash on the base URL must not double the / before
	// /api/v1/query — the request still reaches the handler.
	c := New(Config{
		AuthedURLs: map[string]string{"us-west": srv.URL + "/"},
		Username:   "u", Password: "p",
		HTTPClient: srv.Client(),
	})
	got, err := c.QueryVector(context.Background(), "up")
	if err != nil || len(got) != 1 {
		t.Fatalf("got (%v, %v), want one series and nil err", got, err)
	}
}

func TestQueryVector_NoEndpointsReturnsErrUpstream(t *testing.T) {
	c := New(Config{
		AuthedURLs: map[string]string{}, UnauthedURLs: map[string]string{},
		Username: "u", Password: "p",
	})
	_, err := c.QueryVector(context.Background(), "up")
	if !errors.Is(err, ErrUpstream) {
		t.Fatalf("err = %v, want ErrUpstream", err)
	}
}

// TestQueryVector_RequestTimeoutHonored asserts a region that hangs past the
// configured per-request timeout fails fast at the timeout rather than
// blocking for the old hardcoded 10s. This is the lever that bounds the
// "Resolving trays… for 35s" blast radius when an authed endpoint is slow.
func TestQueryVector_RequestTimeoutHonored(t *testing.T) {
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(3 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer slow.Close()

	c := New(Config{
		AuthedURLs: map[string]string{"us-west": slow.URL},
		Username:   "u", Password: "p",
		QueryTimeout: 200 * time.Millisecond,
		HTTPClient:   slow.Client(),
	})
	start := time.Now()
	_, err := c.QueryVector(context.Background(), "up")
	elapsed := time.Since(start)
	if !errors.Is(err, ErrUpstream) {
		t.Fatalf("err = %v, want ErrUpstream", err)
	}
	if elapsed > 2*time.Second {
		t.Fatalf("QueryVector took %v, want < 2s (per-request timeout not honored)", elapsed)
	}
}

// TestQueryVector_RacesAuthedAndUnauthedConcurrently asserts the authed and
// unauthed attempts for one region run concurrently, not serially. authed is
// slow-but-succeeds; unauthed answers instantly. Serial fallback would block
// on the slow authed and never reach unauthed; racing returns the fast
// unauthed result well before authed finishes.
func TestQueryVector_RacesAuthedAndUnauthedConcurrently(t *testing.T) {
	authedSlow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "success",
			"data": map[string]any{"resultType": "vector", "result": []map[string]any{
				{"metric": map[string]string{"deviceslot": "from-authed"}, "value": []any{0, "1"}},
			}},
		})
	}))
	defer authedSlow.Close()
	unauthedFast := vmServer(t, false, map[string]string{"deviceslot": "from-unauthed"})
	defer unauthedFast.Close()

	c := New(Config{
		AuthedURLs:   map[string]string{"us-west": authedSlow.URL},
		UnauthedURLs: map[string]string{"us-west": unauthedFast.URL},
		Username:     "u", Password: "p",
		QueryTimeout: 5 * time.Second, // long enough that authed would NOT time out
		HTTPClient:   unauthedFast.Client(),
	})
	start := time.Now()
	got, err := c.QueryVector(context.Background(), "up")
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("QueryVector: %v", err)
	}
	if elapsed > 1*time.Second {
		t.Fatalf("QueryVector took %v, want < 1s (authed and unauthed not raced)", elapsed)
	}
	if len(got) != 1 || got[0]["deviceslot"] != "from-unauthed" {
		t.Fatalf("got %v, want the fast unauthed series", got)
	}
}

func TestQueryVector_PartialOutageReturnsSurvivors(t *testing.T) {
	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer dead.Close()
	alive := vmServer(t, true, map[string]string{"deviceslot": "survivor"})
	defer alive.Close()

	// "dead" has no vmui fallback configured, so it errors entirely; "alive"
	// succeeds. The union must contain only the survivor, with a nil error.
	c := New(Config{
		AuthedURLs: map[string]string{"us-east": dead.URL, "us-west": alive.URL},
		Username:   "u", Password: "p",
		HTTPClient: alive.Client(),
	})
	got, err := c.QueryVector(context.Background(), "up")
	if err != nil {
		t.Fatalf("partial outage should not error, got %v", err)
	}
	if len(got) != 1 || got[0]["deviceslot"] != "survivor" {
		t.Fatalf("got %v, want only the survivor series", got)
	}
}

// matrixSeries is one range-query result series for the test server helper.
type matrixSeries struct {
	metric map[string]string
	values [][2]any // each: [unixSeconds, "value"]
}

// vmRangeServer returns an httptest server that replies to /api/v1/query_range
// with the given matrix series. It also asserts the start/end/step params are
// present so a malformed range request fails loudly.
func vmRangeServer(t *testing.T, wantAuth bool, series ...matrixSeries) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/query_range" {
			t.Errorf("path = %q, want /api/v1/query_range", r.URL.Path)
		}
		if _, _, ok := r.BasicAuth(); ok != wantAuth {
			t.Errorf("basic auth present = %v, want %v", ok, wantAuth)
		}
		q := r.URL.Query()
		for _, p := range []string{"query", "start", "end", "step"} {
			if q.Get(p) == "" {
				t.Errorf("missing query param %q", p)
			}
		}
		rows := make([]map[string]any, 0, len(series))
		for _, s := range series {
			rows = append(rows, map[string]any{"metric": s.metric, "values": s.values})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "success",
			"data":   map[string]any{"resultType": "matrix", "result": rows},
		})
	}))
}

func TestQueryRange_ParsesMatrixAndUnions(t *testing.T) {
	east := vmRangeServer(t, true, matrixSeries{
		metric: map[string]string{"alertname": "A", "node": "n-east"},
		values: [][2]any{{1700000000, "1"}, {1700000060, "1"}},
	})
	defer east.Close()
	west := vmRangeServer(t, true, matrixSeries{
		metric: map[string]string{"alertname": "B", "node": "n-west"},
		values: [][2]any{{1700000000.5, "1"}},
	})
	defer west.Close()

	c := New(Config{
		AuthedURLs: map[string]string{"us-east": east.URL, "us-west": west.URL},
		Username:   "u", Password: "p",
		HTTPClient: east.Client(),
	})
	got, err := c.QueryRange(context.Background(), "ALERTS", RangeParams{
		Start: time.Unix(1700000000, 0),
		End:   time.Unix(1700003600, 0),
		Step:  60 * time.Second,
	})
	if err != nil {
		t.Fatalf("QueryRange: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2 (union of both regions)", len(got))
	}
	byNode := map[string]RangeSeries{}
	for _, s := range got {
		byNode[s.Metric["node"]] = s
	}
	east1 := byNode["n-east"]
	if len(east1.Values) != 2 {
		t.Errorf("n-east samples = %d, want 2", len(east1.Values))
	}
	if east1.Values[0].TS != 1700000000 || east1.Values[1].TS != 1700000060 {
		t.Errorf("n-east timestamps = %v, want [1700000000 1700000060]", east1.Values)
	}
	if east1.Values[0].Value != "1" {
		t.Errorf("n-east value = %q, want \"1\"", east1.Values[0].Value)
	}
	// Float-seconds timestamp truncates to int seconds.
	west1 := byNode["n-west"]
	if len(west1.Values) != 1 || west1.Values[0].TS != 1700000000 {
		t.Errorf("n-west values = %v, want one sample at TS 1700000000", west1.Values)
	}
}

func TestQueryRange_AllErroredReturnsErrUpstream(t *testing.T) {
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer bad.Close()

	c := New(Config{
		AuthedURLs: map[string]string{"us-west": bad.URL},
		Username:   "u", Password: "p",
		HTTPClient: bad.Client(),
	})
	_, err := c.QueryRange(context.Background(), "ALERTS", RangeParams{
		Start: time.Unix(1700000000, 0), End: time.Unix(1700003600, 0),
	})
	if !errors.Is(err, ErrUpstream) {
		t.Fatalf("err = %v, want ErrUpstream", err)
	}
}

// vmSamplesServer replies to /api/v1/query with the given series, each carrying
// the provided numeric value (as the VM value tuple [ts, "<value>"]).
func vmSamplesServer(t *testing.T, series ...Sample) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/query" {
			t.Errorf("path = %q, want /api/v1/query", r.URL.Path)
		}
		rows := make([]map[string]any, 0, len(series))
		for _, s := range series {
			rows = append(rows, map[string]any{
				"metric": s.Metric,
				"value":  []any{0, strconv.FormatFloat(s.Value, 'f', -1, 64)},
			})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "success",
			"data":   map[string]any{"resultType": "vector", "result": rows},
		})
	}))
}

func TestQueryVectorSamples_ParsesValuesAndUnions(t *testing.T) {
	east := vmSamplesServer(t, Sample{Metric: map[string]string{"node": "e1", "gpu": "0"}, Value: 1365144.125})
	defer east.Close()
	west := vmSamplesServer(t, Sample{Metric: map[string]string{"node": "w1", "gpu": "3"}, Value: 683489.5})
	defer west.Close()

	c := New(Config{
		AuthedURLs: map[string]string{"us-east": east.URL, "us-west": west.URL},
		Username:   "u", Password: "p",
		HTTPClient: east.Client(),
	})
	got, err := c.QueryVectorSamples(context.Background(), "gpu:hpc_verification_gpu_blaze_gflops:avg5m")
	if err != nil {
		t.Fatalf("QueryVectorSamples: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2 (union of both regions)", len(got))
	}
	byNode := map[string]float64{}
	for _, s := range got {
		byNode[s.Metric["node"]] = s.Value
	}
	if byNode["e1"] != 1365144.125 || byNode["w1"] != 683489.5 {
		t.Errorf("values = %v, want e1=1365144.125 w1=683489.5", byNode)
	}
}

func TestQueryVectorSamples_AllErroredReturnsErrUpstream(t *testing.T) {
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer bad.Close()
	c := New(Config{
		AuthedURLs: map[string]string{"us-west": bad.URL},
		Username:   "u", Password: "p",
		HTTPClient: bad.Client(),
	})
	_, err := c.QueryVectorSamples(context.Background(), "up")
	if !errors.Is(err, ErrUpstream) {
		t.Fatalf("err = %v, want ErrUpstream", err)
	}
}

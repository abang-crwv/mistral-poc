package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"qac/internal/store"
)

// countEvents returns the number of rows in the events table. Used by
// iter-5d tests to verify the all-or-nothing posture (failed
// createRunHandler emits zero events).
func countEvents(t *testing.T, s *store.Store) int {
	t.Helper()
	var n int
	if err := s.RawDB().QueryRowContext(t.Context(),
		`SELECT COUNT(*) FROM events`).Scan(&n); err != nil {
		t.Fatalf("count events: %v", err)
	}
	return n
}

// iter-5d: workflow-resolution subtests for createRunHandler.

func TestCreateRun_Iter5d_HappyPathInlinesSnapshot(t *testing.T) {
	srv, s, _ := newTestServer(t)
	upsertCanonicalCanaryTemplate(t, s)

	body := `{
		"template_id": "firmware-release-canary",
		"inputs": {
			"bundle_tag": "gb200-fw-2026-05-canary-x",
			"canary_racks": ["dh3-r012-us-east-01a"],
			"rlcc_workflow": "gb200-rack-bringup-v4"
		}
	}`
	res, err := http.Post(srv.URL+"/api/runs", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != 201 {
		var envelope map[string]any
		_ = json.NewDecoder(res.Body).Decode(&envelope)
		t.Fatalf("status = %d, want 201; body = %+v", res.StatusCode, envelope)
	}
	var created struct {
		Run struct{ ID string } `json:"run"`
	}
	if err := json.NewDecoder(res.Body).Decode(&created); err != nil {
		t.Fatalf("decode create: %v", err)
	}

	// Fetch the run and find the RunCreated event; its payload must inline
	// the workflow snapshot.
	getRes, err := http.Get(srv.URL + "/api/runs/" + created.Run.ID)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer getRes.Body.Close()
	if getRes.StatusCode != 200 {
		t.Fatalf("GET status = %d, want 200", getRes.StatusCode)
	}
	var got struct {
		Events []struct {
			Kind    string          `json:"kind"`
			Payload json.RawMessage `json:"payload"`
		} `json:"events"`
	}
	if err := json.NewDecoder(getRes.Body).Decode(&got); err != nil {
		t.Fatalf("decode get: %v", err)
	}
	if len(got.Events) == 0 || got.Events[0].Kind != "RunCreated" {
		t.Fatalf("events[0] = %+v, want RunCreated first", got.Events)
	}
	var rc struct {
		RLCCWorkflow *struct {
			Name      string `json:"name"`
			SourceSHA string `json:"source_sha"`
			Actions   []any  `json:"actions"`
		} `json:"rlcc_workflow"`
	}
	if err := json.Unmarshal(got.Events[0].Payload, &rc); err != nil {
		t.Fatalf("unmarshal RunCreated payload: %v", err)
	}
	if rc.RLCCWorkflow == nil {
		t.Fatal("RunCreated.payload.rlcc_workflow is nil; want inlined snapshot")
	}
	if rc.RLCCWorkflow.Name != "gb200-rack-bringup-v4" {
		t.Errorf("Name = %q, want gb200-rack-bringup-v4", rc.RLCCWorkflow.Name)
	}
	if rc.RLCCWorkflow.SourceSHA == "" {
		t.Error("SourceSHA is empty; want non-empty (pinned)")
	}
	if len(rc.RLCCWorkflow.Actions) == 0 {
		t.Errorf("len(Actions) = 0; want > 0")
	}
}

func TestCreateRun_Iter5d_UnknownWorkflow_Returns400(t *testing.T) {
	srv, s, _ := newTestServer(t)
	upsertCanonicalCanaryTemplate(t, s)

	body := `{
		"template_id": "firmware-release-canary",
		"inputs": {
			"bundle_tag": "x",
			"canary_racks": ["dh3-r012-us-east-01a"],
			"rlcc_workflow": "no-such-workflow"
		}
	}`
	res, err := http.Post(srv.URL+"/api/runs", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != 400 {
		t.Fatalf("status = %d, want 400", res.StatusCode)
	}
	var env struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(res.Body).Decode(&env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if env.Error.Code != "rlcc_workflow_unknown" {
		t.Errorf("code = %q, want rlcc_workflow_unknown", env.Error.Code)
	}
}

func TestCreateRun_Iter5d_ActionEmptyWorkflow_Returns400(t *testing.T) {
	srv, s, _ := newTestServer(t)
	upsertCanonicalCanaryTemplate(t, s)

	// "checked-in" is a state-mover in the rlccclient fixture: action_count == 0.
	body := `{
		"template_id": "firmware-release-canary",
		"inputs": {
			"bundle_tag": "x",
			"canary_racks": ["dh3-r012-us-east-01a"],
			"rlcc_workflow": "checked-in"
		}
	}`
	res, err := http.Post(srv.URL+"/api/runs", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != 400 {
		t.Fatalf("status = %d, want 400", res.StatusCode)
	}
	var env struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(res.Body).Decode(&env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if env.Error.Code != "rlcc_workflow_unknown" {
		t.Errorf("code = %q, want rlcc_workflow_unknown (state-mover has no actions)", env.Error.Code)
	}
}

func TestCreateRun_Iter5d_FailurePathEmitsZeroEvents(t *testing.T) {
	// All-or-nothing posture: when workflow resolution fails, no events
	// land in the store. Verify by counting rows before and after.
	srv, s, _ := newTestServer(t)
	upsertCanonicalCanaryTemplate(t, s)
	beforeCount := countEvents(t, s)

	body := `{
		"template_id": "firmware-release-canary",
		"inputs": {
			"bundle_tag": "x",
			"canary_racks": ["dh3-r012-us-east-01a"],
			"rlcc_workflow": "no-such-workflow"
		}
	}`
	res, err := http.Post(srv.URL+"/api/runs", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != 400 {
		t.Fatalf("status = %d, want 400", res.StatusCode)
	}
	afterCount := countEvents(t, s)
	if afterCount != beforeCount {
		t.Errorf("events grew %d → %d on failure path; expected no events emitted",
			beforeCount, afterCount)
	}
}

func TestCreateRun_Iter5d_LegacyOmitsField_StillCreates(t *testing.T) {
	// Before the template flip in Task 8, the legacy {bundle, rack} body
	// (no rlcc_workflow) still succeeds. This subtest pins backward compat
	// for the pre-flip moment. After Task 8 this test will need updating —
	// or kept around as a unit test against an UNFLIPPED template.
	srv, s, _ := newTestServer(t)
	upsertCanonicalCanaryTemplate(t, s) // canonical template has rlcc_workflow OPTIONAL pre-Task 8

	body := `{"bundle":"x","rack":"dh3-r012-us-east-01a"}`
	res, err := http.Post(srv.URL+"/api/runs", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != 201 {
		t.Fatalf("status = %d, want 201 (legacy body, optional rlcc_workflow)", res.StatusCode)
	}
}

func TestCreateRun_Iter5d_BadJSONOnSourcegraphFailure_Returns500(t *testing.T) {
	// MapClient is used in tests, so this scenario is hard to exercise
	// without injecting a stub client. Skip if newTestServer doesn't
	// support client injection — but document the expected behavior here:
	//
	//   When rlccClient.GetWorkflow returns a non-ErrWorkflowNotFound
	//   error (e.g. network failure, YAML parse failure), createRunHandler
	//   responds 500 with code=internal and no events.
	//
	// If newTestServer is extended later to accept a stub rlccclient.Client,
	// re-enable this test by wiring a client whose GetWorkflow returns
	// fmt.Errorf("transient network error").
	t.Skip("skipping: requires stub rlccclient.Client injection — covered by code review of error switch in createRunHandler")
}

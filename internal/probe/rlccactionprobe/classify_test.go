package rlccactionprobe

import (
	"testing"

	"qac/internal/lifecycleclient"
)

func TestClassifyFLCC(t *testing.T) {
	const diag = "l11-fielddiag"
	cases := []struct {
		name string
		obs  lifecycleclient.FLCCObservation
		want ctOutcome
	}{
		{"fail", lifecycleclient.FLCCObservation{State: "fail", PrevStep: "gb200-l11-fielddiag"}, outcomeFailed},
		{"fail-beats-ignore-workflow", lifecycleclient.FLCCObservation{State: "fail", Workflow: "broken-collect"}, outcomeFailed},
		{"ignore-rma", lifecycleclient.FLCCObservation{State: "rma"}, outcomeIgnorable},
		{"ignore-broken-collect-wf", lifecycleclient.FLCCObservation{State: "hold", Workflow: "broken-collect"}, outcomeIgnorable},
		{"in-progress-at-diag", lifecycleclient.FLCCObservation{State: diag}, outcomeInProgress},
		{"passed-moved-past-diag", lifecycleclient.FLCCObservation{State: "ready", PrevState: diag}, outcomeSuccess},
		{"pre-diag-still-inprogress", lifecycleclient.FLCCObservation{State: "l10-test", PrevState: "l10-fielddiag"}, outcomeInProgress},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := classifyFLCC(tc.obs, diag); got != tc.want {
				t.Errorf("classifyFLCC(%+v, %q) = %q, want %q", tc.obs, diag, got, tc.want)
			}
		})
	}
}

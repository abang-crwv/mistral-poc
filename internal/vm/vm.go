// Package vm fans out PromQL instant queries across CoreWeave's
// VictoriaMetrics super-regions, with per-super-region unauthenticated
// (vmui) fallback. It is the single home for VM endpoints, basic-auth,
// and the /api/v1/query HTTP plumbing shared by the inventory resolver
// (internal/inventoryclient) and the lifecycle client
// (internal/lifecycleclient). Ported from tiphys/inventory — qac owns
// this copy outright (bare `qac` module: port, don't import).
package vm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ErrUpstream is wrapped by QueryVector when EVERY configured
// super-region failed (transport error, non-200, or non-"success"
// body). A clean miss — zero series with at least one super-region
// answering — returns (nil, nil) instead, so callers can distinguish
// "rack not found" from "VM unreachable".
var ErrUpstream = errors.New("vm: all super-regions failed")

// Defaults returns qac's authoritative super-region endpoint maps:
// authed VMauth (port 8427, HTTP basic auth) and unauthed vmui
// (multitenant /select/0 path, no auth). Both cover all four
// super-regions. VMauth is plain HTTP — an https:// scheme yields a
// "server gave HTTP response to HTTPS client" TLS error.
func Defaults() (authed, unauthed map[string]string) {
	authed = map[string]string{
		"us-east":  "http://vmauth.us-east.int.coreweave.com:8427/prometheus",
		"us-west":  "http://vmauth.us-west.int.coreweave.com:8427/prometheus",
		"eu-south": "http://vmauth.eu-south.int.coreweave.com:8427/prometheus",
		"us-lab":   "http://vmauth.us-lab.int.coreweave.com:8427/prometheus",
	}
	unauthed = map[string]string{
		"us-east":  "http://vmui.us-east.int.coreweave.com/select/0/prometheus",
		"us-west":  "http://vmui.us-west.int.coreweave.com/select/0/prometheus",
		"eu-south": "http://vmui.eu-south.int.coreweave.com/select/0/prometheus",
		"us-lab":   "http://vmui.us-lab.int.coreweave.com/select/0/prometheus",
	}
	return authed, unauthed
}

// defaultQueryTimeout caps a single super-region attempt (one authed or one
// unauthed HTTP query). Healthy VM queries finish in well under a second; the
// old 10s ceiling let a single slow/hanging endpoint balloon an interactive
// preview to tens of seconds (3 serial fan-outs × a region waiting out its
// timeout). 4s is generous for a real query yet bounds the blast radius.
const defaultQueryTimeout = 4 * time.Second

// Config configures a Client. Nil AuthedURLs AND nil UnauthedURLs fall
// back to Defaults(). An empty Username or Password puts the Client in
// unauthed mode (vmui only) and emits a one-time warning.
type Config struct {
	AuthedURLs   map[string]string
	UnauthedURLs map[string]string
	Username     string
	Password     string
	HTTPClient   *http.Client
	// QueryTimeout caps each per-super-region HTTP attempt. Zero uses
	// defaultQueryTimeout.
	QueryTimeout time.Duration
}

// endpoint is one super-region's URL pair (either side may be empty).
type endpoint struct {
	name        string
	authedURL   string
	unauthedURL string
}

// Client fans out instant queries across super-regions. Goroutine-safe.
type Client struct {
	endpoints []endpoint
	user      string
	pass      string
	unauthed  bool          // creds absent: skip authed attempts entirely
	timeout   time.Duration // per-attempt HTTP timeout
	httpDo    func(*http.Request) (*http.Response, error)
}

// New builds a Client. It never errors: an empty endpoint set simply
// makes QueryVector return ErrUpstream. When creds are absent it logs a
// one-time unauthed-fallback warning.
func New(cfg Config) *Client {
	authed, unauthed := cfg.AuthedURLs, cfg.UnauthedURLs
	if authed == nil && unauthed == nil {
		authed, unauthed = Defaults()
	}
	// Union of super-region keys, sorted for deterministic iteration.
	keySet := map[string]struct{}{}
	for k := range authed {
		keySet[k] = struct{}{}
	}
	for k := range unauthed {
		keySet[k] = struct{}{}
	}
	keys := make([]string, 0, len(keySet))
	for k := range keySet {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	eps := make([]endpoint, 0, len(keys))
	for _, k := range keys {
		eps = append(eps, endpoint{
			name:        k,
			authedURL:   strings.TrimRight(authed[k], "/"),
			unauthedURL: strings.TrimRight(unauthed[k], "/"),
		})
	}
	timeout := cfg.QueryTimeout
	if timeout <= 0 {
		timeout = defaultQueryTimeout
	}
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		// No client-level timeout: doQuery layers a per-attempt context
		// deadline (c.timeout) onto every request, which is the single
		// source of truth for how long one attempt may run.
		httpClient = &http.Client{}
	}
	noCreds := cfg.Username == "" || cfg.Password == ""
	if noCreds {
		maybeWarnUnauthed("AWXCTL_VMAUTH_USERNAME / _PASSWORD unset")
	}
	return &Client{
		endpoints: eps,
		user:      cfg.Username,
		pass:      cfg.Password,
		unauthed:  noCreds,
		timeout:   timeout,
		httpDo:    httpClient.Do,
	}
}

// QueryVector runs q as an instant query against every super-region in
// parallel and returns the UNION of result-series label maps. Within each
// super-region the authed URL (when creds present) and the unauthed vmui
// URL race concurrently and the first success wins; a super-region errors
// only when both attempts fail. Returns (nil, ErrUpstream) only when every
// super-region errored.
func (c *Client) QueryVector(ctx context.Context, q string) ([]map[string]string, error) {
	if len(c.endpoints) == 0 {
		return nil, fmt.Errorf("%w: no super-regions configured", ErrUpstream)
	}
	type result struct {
		series []map[string]string
		err    error
	}
	results := make([]result, len(c.endpoints))
	var wg sync.WaitGroup
	for i, ep := range c.endpoints {
		i, ep := i, ep
		wg.Add(1)
		go func() {
			defer wg.Done()
			series, err := c.queryEndpoint(ctx, ep, q)
			results[i] = result{series: series, err: err}
		}()
	}
	wg.Wait()

	var union []map[string]string
	allErrored := true
	for _, r := range results {
		if r.err == nil {
			allErrored = false
			union = append(union, r.series...)
		}
	}
	if allErrored {
		errs := make([]string, len(results))
		for i, r := range results {
			errs[i] = fmt.Sprintf("%s: %v", c.endpoints[i].name, r.err)
		}
		return nil, fmt.Errorf("%w: %s", ErrUpstream, strings.Join(errs, "; "))
	}
	return union, nil
}

// queryEndpoint resolves one super-region. It runs the authed VMauth and
// unauthed vmui attempts CONCURRENTLY and returns the first success — so a
// slow or hanging authed endpoint no longer serializes a second timeout
// before the vmui fallback answers. The two paths read the same bare-metal
// inventory (vmui is the trusted equivalent fallback the rest of this
// package already relies on), so first-success-wins does not change which
// data a caller sees, only how fast it arrives. When creds are absent
// (c.unauthed) only vmui is queried; when a region configures just one URL,
// only that one runs. Returns the last error when every attempt fails.
func (c *Client) queryEndpoint(ctx context.Context, ep endpoint, q string) ([]map[string]string, error) {
	type attempt struct {
		series []map[string]string
		err    error
	}
	results := make(chan attempt, 2)
	n := 0
	if !c.unauthed && ep.authedURL != "" {
		n++
		go func() {
			s, err := c.doQuery(ctx, ep.authedURL, q, true)
			results <- attempt{s, err}
		}()
	}
	if ep.unauthedURL != "" {
		n++
		go func() {
			s, err := c.doQuery(ctx, ep.unauthedURL, q, false)
			results <- attempt{s, err}
		}()
	}
	if n == 0 {
		return nil, fmt.Errorf("vm %s: no usable endpoint", ep.name)
	}
	var lastErr error
	for i := 0; i < n; i++ {
		r := <-results
		if r.err == nil {
			return r.series, nil
		}
		lastErr = r.err
	}
	return nil, lastErr
}

// vmResponse is the standard Prometheus instant-query response. Only the
// per-series label map is read; the sample value is ignored (both
// callers ignore it).
type vmResponse struct {
	Status string `json:"status"`
	Data   struct {
		Result []struct {
			Metric map[string]string `json:"metric"`
		} `json:"result"`
	} `json:"data"`
}

// doQuery issues one GET <base>/api/v1/query?query=<q>. useAuth toggles
// the basic-auth header (true for VMauth, false for vmui). A 10s
// per-request timeout is layered onto the caller's context.
func (c *Client) doQuery(ctx context.Context, base, q string, useAuth bool) ([]map[string]string, error) {
	u := fmt.Sprintf("%s/api/v1/query?query=%s", base, url.QueryEscape(q))
	reqCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	if useAuth {
		req.SetBasicAuth(c.user, c.pass)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.httpDo(req)
	if err != nil {
		return nil, fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("vm %s: status %d: %s", base, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var vr vmResponse
	if err := json.NewDecoder(resp.Body).Decode(&vr); err != nil {
		return nil, fmt.Errorf("decode vm response: %w", err)
	}
	if vr.Status != "success" {
		return nil, fmt.Errorf("vm status: %s", vr.Status)
	}
	out := make([]map[string]string, 0, len(vr.Data.Result))
	for _, s := range vr.Data.Result {
		out = append(out, s.Metric)
	}
	return out, nil
}

// Sample is one instant-vector result: its label map plus the parsed numeric
// value. QueryVector drops the value; QueryVectorSamples keeps it for callers
// (e.g. gpuperfclient) whose payload IS the value.
type Sample struct {
	Metric map[string]string
	Value  float64
}

// vmSamplesResponse is the instant-query response WITH the sample value. The
// value field is [<ts float>, "<value string>"]; we read index 1.
type vmSamplesResponse struct {
	Status string `json:"status"`
	Data   struct {
		Result []struct {
			Metric map[string]string `json:"metric"`
			Value  []json.RawMessage `json:"value"`
		} `json:"result"`
	} `json:"data"`
}

// QueryVectorSamples runs q as an instant query and returns the UNION of
// result samples (labels + numeric value) across every super-region. Fan-out,
// authed/vmui race, and all-fail→ErrUpstream semantics mirror QueryVector
// exactly — it differs only in keeping the sample value.
func (c *Client) QueryVectorSamples(ctx context.Context, q string) ([]Sample, error) {
	if len(c.endpoints) == 0 {
		return nil, fmt.Errorf("%w: no super-regions configured", ErrUpstream)
	}
	type result struct {
		samples []Sample
		err     error
	}
	results := make([]result, len(c.endpoints))
	var wg sync.WaitGroup
	for i, ep := range c.endpoints {
		i, ep := i, ep
		wg.Add(1)
		go func() {
			defer wg.Done()
			samples, err := c.queryEndpointSamples(ctx, ep, q)
			results[i] = result{samples: samples, err: err}
		}()
	}
	wg.Wait()

	var union []Sample
	allErrored := true
	for _, r := range results {
		if r.err == nil {
			allErrored = false
			union = append(union, r.samples...)
		}
	}
	if allErrored {
		errs := make([]string, len(results))
		for i, r := range results {
			errs[i] = fmt.Sprintf("%s: %v", c.endpoints[i].name, r.err)
		}
		return nil, fmt.Errorf("%w: %s", ErrUpstream, strings.Join(errs, "; "))
	}
	return union, nil
}

// queryEndpointSamples is the sample-keeping twin of queryEndpoint: it races
// the authed VMauth and unauthed vmui attempts and returns the first success.
func (c *Client) queryEndpointSamples(ctx context.Context, ep endpoint, q string) ([]Sample, error) {
	type attempt struct {
		samples []Sample
		err     error
	}
	results := make(chan attempt, 2)
	n := 0
	if !c.unauthed && ep.authedURL != "" {
		n++
		go func() {
			s, err := c.doQuerySamples(ctx, ep.authedURL, q, true)
			results <- attempt{s, err}
		}()
	}
	if ep.unauthedURL != "" {
		n++
		go func() {
			s, err := c.doQuerySamples(ctx, ep.unauthedURL, q, false)
			results <- attempt{s, err}
		}()
	}
	if n == 0 {
		return nil, fmt.Errorf("vm %s: no usable endpoint", ep.name)
	}
	var lastErr error
	for i := 0; i < n; i++ {
		r := <-results
		if r.err == nil {
			return r.samples, nil
		}
		lastErr = r.err
	}
	return nil, lastErr
}

// doQuerySamples is the sample-keeping twin of doQuery. It parses each result's
// value tuple [ts, "val"] and converts the value string to float64; a series
// whose value can't be parsed is skipped (rather than failing the whole query).
func (c *Client) doQuerySamples(ctx context.Context, base, q string, useAuth bool) ([]Sample, error) {
	u := fmt.Sprintf("%s/api/v1/query?query=%s", base, url.QueryEscape(q))
	reqCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	if useAuth {
		req.SetBasicAuth(c.user, c.pass)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.httpDo(req)
	if err != nil {
		return nil, fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("vm %s: status %d: %s", base, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var vr vmSamplesResponse
	if err := json.NewDecoder(resp.Body).Decode(&vr); err != nil {
		return nil, fmt.Errorf("decode vm response: %w", err)
	}
	if vr.Status != "success" {
		return nil, fmt.Errorf("vm status: %s", vr.Status)
	}
	out := make([]Sample, 0, len(vr.Data.Result))
	for _, s := range vr.Data.Result {
		if len(s.Value) != 2 {
			continue
		}
		var raw string
		if err := json.Unmarshal(s.Value[1], &raw); err != nil {
			continue
		}
		v, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			continue
		}
		out = append(out, Sample{Metric: s.Metric, Value: v})
	}
	return out, nil
}

// RangeSample is one (timestamp, raw value) point from a matrix series.
// Value is the raw string VictoriaMetrics returns ("1", "0", "1.5"); callers
// that only care whether the series was present at a step ignore it.
type RangeSample struct {
	TS    int64  // unix seconds (truncated from VM's float-seconds timestamp)
	Value string // raw sample value as returned
}

// RangeSeries is one matrix series: its label map plus the time-ordered
// samples that fell in [Start,End] at the query step.
type RangeSeries struct {
	Metric map[string]string
	Values []RangeSample
}

// RangeParams bounds a single QueryRange call. Step zero defaults to
// defaultRangeStep.
type RangeParams struct {
	Start time.Time
	End   time.Time
	Step  time.Duration
}

// defaultRangeStep is the resolution used when RangeParams.Step is zero.
// A 24h window at 60s is 1440 points — comfortably under VM's
// maxPointsPerTimeseries cap (default 30000).
const defaultRangeStep = 60 * time.Second

// QueryRange runs q as a range query against /api/v1/query_range over
// [p.Start,p.End] at p.Step, fanning out across every super-region in
// parallel and returning the UNION of matrix series. Fan-out, authed/vmui
// race, and all-fail→ErrUpstream semantics mirror QueryVector exactly.
func (c *Client) QueryRange(ctx context.Context, q string, p RangeParams) ([]RangeSeries, error) {
	if len(c.endpoints) == 0 {
		return nil, fmt.Errorf("%w: no super-regions configured", ErrUpstream)
	}
	if p.Step <= 0 {
		p.Step = defaultRangeStep
	}
	type result struct {
		series []RangeSeries
		err    error
	}
	results := make([]result, len(c.endpoints))
	var wg sync.WaitGroup
	for i, ep := range c.endpoints {
		i, ep := i, ep
		wg.Add(1)
		go func() {
			defer wg.Done()
			series, err := c.queryRangeEndpoint(ctx, ep, q, p)
			results[i] = result{series: series, err: err}
		}()
	}
	wg.Wait()

	var union []RangeSeries
	allErrored := true
	for _, r := range results {
		if r.err == nil {
			allErrored = false
			union = append(union, r.series...)
		}
	}
	if allErrored {
		errs := make([]string, len(results))
		for i, r := range results {
			errs[i] = fmt.Sprintf("%s: %v", c.endpoints[i].name, r.err)
		}
		return nil, fmt.Errorf("%w: %s", ErrUpstream, strings.Join(errs, "; "))
	}
	return union, nil
}

// queryRangeEndpoint resolves one super-region for a range query, racing the
// authed and unauthed attempts and returning the first success — the same
// first-success-wins shape as queryEndpoint.
func (c *Client) queryRangeEndpoint(ctx context.Context, ep endpoint, q string, p RangeParams) ([]RangeSeries, error) {
	type attempt struct {
		series []RangeSeries
		err    error
	}
	results := make(chan attempt, 2)
	n := 0
	if !c.unauthed && ep.authedURL != "" {
		n++
		go func() {
			s, err := c.doQueryRange(ctx, ep.authedURL, q, p, true)
			results <- attempt{s, err}
		}()
	}
	if ep.unauthedURL != "" {
		n++
		go func() {
			s, err := c.doQueryRange(ctx, ep.unauthedURL, q, p, false)
			results <- attempt{s, err}
		}()
	}
	if n == 0 {
		return nil, fmt.Errorf("vm %s: no usable endpoint", ep.name)
	}
	var lastErr error
	for i := 0; i < n; i++ {
		r := <-results
		if r.err == nil {
			return r.series, nil
		}
		lastErr = r.err
	}
	return nil, lastErr
}

// vmRangeResponse is the standard Prometheus range-query (matrix) response.
// Each value entry is a [<float seconds>, "<string value>"] pair, decoded
// here as a two-element raw array so the timestamp and value can be parsed
// independently.
type vmRangeResponse struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string `json:"resultType"`
		Result     []struct {
			Metric map[string]string    `json:"metric"`
			Values [][2]json.RawMessage `json:"values"`
		} `json:"result"`
	} `json:"data"`
}

// doQueryRange issues one GET <base>/api/v1/query_range?query=&start=&end=&step=.
// useAuth toggles basic auth (true for VMauth, false for vmui). A per-attempt
// timeout is layered onto the caller's context, matching doQuery.
func (c *Client) doQueryRange(ctx context.Context, base, q string, p RangeParams, useAuth bool) ([]RangeSeries, error) {
	v := url.Values{}
	v.Set("query", q)
	v.Set("start", strconv.FormatInt(p.Start.Unix(), 10))
	v.Set("end", strconv.FormatInt(p.End.Unix(), 10))
	v.Set("step", strconv.FormatInt(int64(p.Step.Seconds()), 10)+"s")
	u := base + "/api/v1/query_range?" + v.Encode()

	reqCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	if useAuth {
		req.SetBasicAuth(c.user, c.pass)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.httpDo(req)
	if err != nil {
		return nil, fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("vm %s: status %d: %s", base, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var vr vmRangeResponse
	if err := json.NewDecoder(resp.Body).Decode(&vr); err != nil {
		return nil, fmt.Errorf("decode vm range response: %w", err)
	}
	if vr.Status != "success" {
		return nil, fmt.Errorf("vm status: %s", vr.Status)
	}
	out := make([]RangeSeries, 0, len(vr.Data.Result))
	for _, s := range vr.Data.Result {
		rs := RangeSeries{Metric: s.Metric, Values: make([]RangeSample, 0, len(s.Values))}
		for _, pair := range s.Values {
			var tsFloat float64
			if err := json.Unmarshal(pair[0], &tsFloat); err != nil {
				return nil, fmt.Errorf("decode sample timestamp: %w", err)
			}
			var val string
			if err := json.Unmarshal(pair[1], &val); err != nil {
				return nil, fmt.Errorf("decode sample value: %w", err)
			}
			rs.Values = append(rs.Values, RangeSample{TS: int64(tsFloat), Value: val})
		}
		out = append(out, rs)
	}
	return out, nil
}

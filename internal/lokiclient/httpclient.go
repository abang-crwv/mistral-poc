package lokiclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// DefaultBaseURL is the CoreWeave Grafana base URL that proxies all regional
// Loki datasources.
const DefaultBaseURL = "https://grafana.int.coreweave.com"

// Config configures an HTTPClient. Token is the Grafana service-account token
// (from GRAFANA_SERVICE_ACCOUNT); an empty BaseURL falls back to
// DefaultBaseURL; a nil HTTPClient falls back to a 30s-timeout client.
type Config struct {
	BaseURL    string
	Token      string
	HTTPClient *http.Client
}

// HTTPClient queries Loki through the Grafana datasource proxy.
type HTTPClient struct {
	baseURL string
	token   string
	hc      *http.Client
}

// NewHTTPClient builds an HTTPClient from cfg.
func NewHTTPClient(cfg Config) *HTTPClient {
	base := cfg.BaseURL
	if base == "" {
		base = DefaultBaseURL
	}
	hc := cfg.HTTPClient
	if hc == nil {
		hc = &http.Client{Timeout: 30 * time.Second}
	}
	return &HTTPClient{baseURL: base, token: cfg.Token, hc: hc}
}

// Compile-time satisfaction check.
var _ Client = (*HTTPClient)(nil)

// lokiRangeResponse is the streams-shaped body of /loki/api/v1/query_range.
// values is a list of [unix_ns_string, line] pairs.
type lokiRangeResponse struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string `json:"resultType"`
		Result     []struct {
			Stream map[string]string `json:"stream"`
			Values [][2]string       `json:"values"`
		} `json:"result"`
	} `json:"data"`
}

// QueryRange runs logql against the region's Loki datasource over [start, end].
func (c *HTTPClient) QueryRange(ctx context.Context, region, logql string, start, end time.Time, limit int) ([]LogEntry, error) {
	uid := ResolveLokiUID(region)
	if uid == "" {
		return nil, fmt.Errorf("%w: %q", ErrUnknownRegion, region)
	}
	if limit <= 0 {
		limit = 100
	}

	q := url.Values{}
	q.Set("query", logql)
	q.Set("start", strconv.FormatInt(start.UnixNano(), 10))
	q.Set("end", strconv.FormatInt(end.UnixNano(), 10))
	q.Set("limit", strconv.Itoa(limit))
	q.Set("direction", "backward") // newest-first
	endpoint := fmt.Sprintf("%s/api/datasources/proxy/uid/%s/loki/api/v1/query_range?%s", c.baseURL, uid, q.Encode())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: build request: %v", ErrSourceUnavailable, err)
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.hc.Do(req)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		return nil, fmt.Errorf("%w: %s Loki query: %v", ErrSourceUnavailable, region, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: %s Loki query: HTTP %d", ErrSourceUnavailable, region, resp.StatusCode)
	}

	var body lokiRangeResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("%w: decode %s Loki response: %v", ErrSourceUnavailable, region, err)
	}

	var out []LogEntry
	for _, stream := range body.Data.Result {
		for _, v := range stream.Values {
			ts := parseUnixNano(v[0])
			out = append(out, LogEntry{Timestamp: ts, Line: v[1], Labels: stream.Stream})
		}
	}
	return out, nil
}

// parseUnixNano converts a Loki nanosecond-timestamp string to a time.Time;
// an unparseable value yields the zero time.
func parseUnixNano(s string) time.Time {
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return time.Time{}
	}
	return time.Unix(0, n).UTC()
}

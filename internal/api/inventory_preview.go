package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"qac/internal/inventoryclient"
	"qac/internal/lifecycleclient"
)

type previewBMN struct {
	Deviceslot string `json:"deviceslot"`
	BMNName    string `json:"bmn_name"`
	CTPosition int    `json:"ct_position,omitempty"`
}

type previewIgnored struct {
	Deviceslot string `json:"deviceslot"`
	BMNName    string `json:"bmn_name,omitempty"`
}

type previewRack struct {
	Rack         string           `json:"rack"`
	Zone         string           `json:"zone"`
	InstanceType string           `json:"instance_type,omitempty"`
	SKU          string           `json:"sku,omitempty"`
	BMNs         []previewBMN     `json:"bmns"`
	RLCCIgnored  []previewIgnored `json:"rlcc_ignored,omitempty"`
}

type previewError struct {
	Rack    string `json:"rack"`
	Message string `json:"message"`
}

type previewResponse struct {
	Racks  []previewRack  `json:"racks"`
	Errors []previewError `json:"errors,omitempty"`
}

// previewCache is a tiny TTL cache shared by all preview-handler invocations
// from the same Router. 60s is the recommended interval (operator iterates
// the wizard within this window; chart updates land slower).
type previewCache struct {
	mu      sync.Mutex
	entries map[string]previewCacheEntry
}

type previewCacheEntry struct {
	body      previewResponse
	expiresAt time.Time
}

const previewTTL = 60 * time.Second

func (c *previewCache) get(key string, now time.Time) (previewResponse, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[key]
	if !ok || e.expiresAt.Before(now) {
		return previewResponse{}, false
	}
	return e.body, true
}

func (c *previewCache) put(key string, body previewResponse) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries == nil {
		c.entries = map[string]previewCacheEntry{}
	}
	c.entries[key] = previewCacheEntry{body: body, expiresAt: time.Now().Add(previewTTL)}
}

// inventoryPreviewHandler returns a handler for GET /api/inventory/preview.
// The cache is per-handler-instance (one closure-captured cache per Router
// build), so test isolation is automatic.
func inventoryPreviewHandler(resolver inventoryclient.Resolver, lifeC lifecycleclient.Client) http.HandlerFunc {
	cache := &previewCache{}
	return func(w http.ResponseWriter, r *http.Request) {
		raw := r.URL.Query().Get("racks")
		if strings.TrimSpace(raw) == "" {
			writeError(w, http.StatusBadRequest, codeInvalidRequest, "racks query parameter required")
			return
		}
		racks := strings.Split(raw, ",")
		// Normalize cache key: joined after trimming whitespace.
		key := strings.ToLower(strings.Join(racks, ","))
		if cached, ok := cache.get(key, time.Now()); ok {
			writeJSON(w, http.StatusOK, cached)
			return
		}
		// Initialize as empty slices, not nil. Go marshals nil slices as
		// `null`, which forces every JSON consumer to defensively coalesce.
		// Emitting `[]` keeps the contract stable: callers always get an
		// array, even when nothing resolved.
		body := previewResponse{
			Racks:  []previewRack{},
			Errors: []previewError{},
		}
		for _, rack := range racks {
			rack = strings.TrimSpace(rack)
			if rack == "" {
				continue
			}
			pr, err := buildPreviewRack(r.Context(), resolver, lifeC, rack)
			if err != nil {
				body.Errors = append(body.Errors, previewError{Rack: rack, Message: err.Error()})
				continue
			}
			body.Racks = append(body.Racks, pr)
		}
		cache.put(key, body)
		writeJSON(w, http.StatusOK, body)
	}
}

// buildPreviewRack resolves a single rack's facts + BMN list + RLCC ignores.
// Returns ErrNotFound-derived error when the rack doesn't resolve so the
// caller can put it in errors[] rather than failing the whole response.
func buildPreviewRack(ctx context.Context, resolver inventoryclient.Resolver, lifeC lifecycleclient.Client, rack string) (previewRack, error) {
	rf, err := inventoryclient.ResolveRack(ctx, resolver, rack)
	if err != nil {
		if errors.Is(err, inventoryclient.ErrNotFound) {
			return previewRack{}, errors.New("rack not found")
		}
		return previewRack{}, err
	}
	bmns, err := resolver.ResolveBMNs(ctx, rack)
	if err != nil {
		return previewRack{}, err
	}
	zone := ""
	if len(bmns) > 0 {
		zone = bmns[0].Zone
	}
	out := previewRack{
		Rack:         rack,
		Zone:         zone,
		InstanceType: rf.InstanceType,
		SKU:          rf.SKU,
	}
	out.BMNs = make([]previewBMN, 0, len(bmns))
	for _, b := range bmns {
		out.BMNs = append(out.BMNs, previewBMN{
			Deviceslot: b.Deviceslot,
			BMNName:    b.BMNName,
			CTPosition: b.CTPosition,
		})
	}
	// Best-effort RLCC ignore detect. A query failure leaves RLCCIgnored
	// unset — the operator can manually mark CTs in the wizard. Don't fail
	// the whole rack on a VM hiccup.
	ignored, qerr := lifeC.QueryRLCCIgnored(ctx, lifecycleclient.RackKey{Rack: rack, Zone: zone})
	if qerr == nil {
		out.RLCCIgnored = make([]previewIgnored, 0, len(ignored))
		for _, ib := range ignored {
			out.RLCCIgnored = append(out.RLCCIgnored, previewIgnored{
				Deviceslot: ib.Deviceslot,
				BMNName:    ib.BMNName,
			})
		}
	}
	return out, nil
}

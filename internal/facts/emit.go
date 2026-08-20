package facts

import (
	"context"
	"encoding/json"
	"fmt"

	"qac/internal/store"
)

// Emit appends a FactsDiscovered event for the given (scope, source)
// batch and lets the store's transactional projection UPSERT each
// fact row. The whole batch is atomic — if any key is unregistered,
// no event is appended and no facts land.
//
// An empty kvs is a no-op (returns nil without appending an event).
// Producers can call Emit unconditionally and skip the "is the batch
// empty" check in their own code.
func Emit(ctx context.Context, s *store.Store, runID, scope, source string, kvs map[Key]any) error {
	if len(kvs) == 0 {
		return nil
	}

	// Validate every key up front so we don't half-emit.
	plain := make(map[string]any, len(kvs))
	for k, v := range kvs {
		if !IsRegistered(k) {
			return fmt.Errorf("%w: %s", ErrUnknownFactKey, k)
		}
		plain[string(k)] = v
	}

	payload, err := json.Marshal(map[string]any{
		"scope":  scope,
		"source": source,
		"facts":  plain,
	})
	if err != nil {
		return fmt.Errorf("marshal FactsDiscovered payload: %w", err)
	}

	if err := s.AppendEvent(ctx, runID, "FactsDiscovered", payload); err != nil {
		return fmt.Errorf("append FactsDiscovered: %w", err)
	}
	return nil
}

package client

import (
	"context"
	"fmt"
	"log/slog"
)

// ExistsByKey reports whether a secret with the given key exists in the
// organization. It backs the read-only /exists/{key} endpoint used by
// external-secrets' PushSecret verify flow (updatePolicy: IfNotExists),
// which sends the store-wide POST method and therefore must never reach
// the create-or-update /key endpoint.
func (b *Bitwarden) ExistsByKey(ctx context.Context, key, orgID, clientToken string) (bool, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := b.connect(clientToken); err != nil {
		return false, fmt.Errorf("connect: %w", err)
	}
	defer b.close()
	return b.doExists(ctx, b.Client, key, orgID)
}

// doExists holds the existence logic against an already-connected SDK
// client. It always resolves via a list scan, never the key->ID cache:
// a cached mapping can be stale after an out-of-band delete, and a false
// "exists" would make IfNotExists skip pushes forever. The scan seeds the
// whole key->ID mapping as a side effect, same as the other read paths.
func (b *Bitwarden) doExists(ctx context.Context, cl sdkClient, key, orgID string) (bool, error) {
	list, err := cl.Secrets().List(orgID)
	if err != nil {
		return false, fmt.Errorf("list secrets: %w", err)
	}
	found := false
	for _, pair := range list.Data {
		b.Cache.SetID(pair.Key, pair.ID)
		if pair.Key == key {
			found = true
		}
	}
	if !found {
		// Drop a possibly stale key->ID mapping so later flows
		// re-resolve instead of trusting a cache entry for a secret
		// that no longer exists.
		b.Cache.DeleteID(key)
		slog.DebugContext(ctx, fmt.Sprintf("key %s not present in organization", key))
	}
	return found, nil
}

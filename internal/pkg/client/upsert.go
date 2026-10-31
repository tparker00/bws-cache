package client

import (
	"context"
	"fmt"
	"log/slog"
)

// UpsertByKey stores value under the named secret in the organization:
// update-in-place when a secret with that key exists, create otherwise.
// The key->ID mapping is resolved through the cache first (populated by
// reads on cache miss) and only falls back to a full list scan when cold.
// Written values are invalidated rather than cached: the read path caches
// the full JSON SecretResponse shape, which only the API response has.
func (b *Bitwarden) UpsertByKey(ctx context.Context, key, value, note, orgID, clientToken string) (string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := b.connect(clientToken); err != nil {
		return "", fmt.Errorf("connect: %w", err)
	}
	defer b.close()

	id := b.Cache.GetID(key)
	if id == "" {
		slog.DebugContext(ctx, fmt.Sprintf("key %s not in cache, resolving via list", key))
		list, err := b.Client.Secrets().List(orgID)
		if err != nil {
			return "", fmt.Errorf("list secrets: %w", err)
		}
		for _, pair := range list.Data {
			// Populate the whole mapping like GetByKey does: one list
			// serves every key in the organization.
			b.Cache.SetID(pair.Key, pair.ID)
			if pair.Key == key {
				id = pair.ID
			}
		}
	}

	if id != "" {
		slog.DebugContext(ctx, fmt.Sprintf("Updating existing secret: %s", key))
		// The list response omits ProjectIDs; fetch the full secret so the
		// update preserves project membership.
		existing, err := b.Client.Secrets().Get(id)
		if err != nil {
			return "", fmt.Errorf("get secret for update: %w", err)
		}
		var projectIDs []string
		if existing.ProjectID != nil && *existing.ProjectID != "" {
			projectIDs = []string{*existing.ProjectID}
		}
		res, err := b.Client.Secrets().Update(id, key, value, note, orgID, projectIDs)
		if err != nil {
			// The cached key->ID mapping may be stale (secret deleted
			// out-of-band); drop it so the next attempt re-resolves.
			b.Cache.DeleteID(key)
			return "", fmt.Errorf("update secret: %w", err)
		}
		b.Cache.DeleteSecret(res.ID)
		b.Cache.SetID(key, res.ID)
		return res.ID, nil
	}

	slog.DebugContext(ctx, fmt.Sprintf("Creating new secret: %s", key))
	res, err := b.Client.Secrets().Create(key, value, note, orgID, nil)
	if err != nil {
		return "", fmt.Errorf("create secret: %w", err)
	}
	b.Cache.SetID(key, res.ID)
	return res.ID, nil
}

// ResetCache drops all cached secret values; readers repopulate from the API.
func (b *Bitwarden) ResetCache() {
	b.Cache.Reset()
}

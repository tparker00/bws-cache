package client

import (
	"context"
	"fmt"
	"log/slog"
)

// UpsertByKey stores value under the named secret in the organization:
// update-in-place when a secret with that key exists, create otherwise.
// The cache entry for the key is refreshed so readers see the write.
func (b *Bitwarden) UpsertByKey(ctx context.Context, key, value, note, orgID, clientToken string) (string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := b.connect(clientToken); err != nil {
		return "", fmt.Errorf("connect: %w", err)
	}
	defer b.close()

	list, err := b.Client.Secrets().List(orgID)
	if err != nil {
		return "", fmt.Errorf("list secrets: %w", err)
	}
	for _, id := range list.Data {
		if id.Key != key {
			continue
		}
		slog.DebugContext(ctx, fmt.Sprintf("Updating existing secret: %s", key))
		// The list response omits ProjectIDs; fetch the full secret so the
		// update preserves project membership.
		existing, err := b.Client.Secrets().Get(id.ID)
		if err != nil {
			return "", fmt.Errorf("get secret for update: %w", err)
		}
		var projectIDs []string
		if existing.ProjectID != nil && *existing.ProjectID != "" {
			projectIDs = []string{*existing.ProjectID}
		}
		res, err := b.Client.Secrets().Update(id.ID, key, value, note, orgID, projectIDs)
		if err != nil {
			return "", fmt.Errorf("update secret: %w", err)
		}
		b.Cache.SetSecret(res.ID, value)
		return res.ID, nil
	}
	slog.DebugContext(ctx, fmt.Sprintf("Creating new secret: %s", key))
	res, err := b.Client.Secrets().Create(key, value, note, orgID, nil)
	if err != nil {
		return "", fmt.Errorf("create secret: %w", err)
	}
	b.Cache.SetSecret(res.ID, value)
	return res.ID, nil
}

// ResetCache drops all cached secret values; readers repopulate from the API.
func (b *Bitwarden) ResetCache() {
	b.Cache.Reset()
}

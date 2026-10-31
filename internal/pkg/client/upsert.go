package client

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	sdk "github.com/bitwarden/sdk-go"
)

// sdkClient is the SDK surface doUpsert needs; *sdk.BitwardenClient and
// test fakes both satisfy it.
type sdkClient interface {
	Secrets() sdk.SecretsInterface
	Projects() sdk.ProjectsInterface
}

// UpsertByKey stores value under the named secret in the organization:
// update-in-place when a secret with that key exists, create otherwise.
// The key->ID mapping is resolved through the cache first (populated by
// reads on cache miss) and only falls back to a full list scan when cold.
// Returns the full JSON SecretResponse, matching the read path's shape.
// Written values are invalidated rather than cached: the read path caches
// the full JSON SecretResponse shape, which only the API response has.
func (b *Bitwarden) UpsertByKey(ctx context.Context, key, value, note, orgID, clientToken string) (string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := b.connect(clientToken); err != nil {
		return "", fmt.Errorf("connect: %w", err)
	}
	defer b.close()
	return b.doUpsert(ctx, b.Client, key, value, note, orgID)
}

// doUpsert holds the upsert logic against an already-connected SDK client.
// When a project name is configured (BWS_CACHE_PROJECT_NAME) it resolves
// name->ID once per process and targets that project for both create and
// update; when unset, updates preserve existing membership via a full Get.
func (b *Bitwarden) doUpsert(ctx context.Context, cl sdkClient, key, value, note, orgID string) (string, error) {
	projectIDs, err := b.resolveProject(ctx, cl, orgID)
	if err != nil {
		return "", err
	}

	id := b.Cache.GetID(key)
	if id == "" {
		slog.DebugContext(ctx, fmt.Sprintf("key %s not in cache, resolving via list", key))
		list, err := cl.Secrets().List(orgID)
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
		if len(projectIDs) == 0 {
			// No project configured: preserve existing membership, which the
			// list response omits, so fetch the full secret first.
			existing, err := cl.Secrets().Get(id)
			if err != nil {
				return "", fmt.Errorf("get secret for update: %w", err)
			}
			if existing.ProjectID != nil && *existing.ProjectID != "" {
				projectIDs = []string{*existing.ProjectID}
			}
		}
		res, err := cl.Secrets().Update(id, key, value, note, orgID, projectIDs)
		if err != nil {
			// The cached key->ID mapping may be stale (secret deleted
			// out-of-band); drop it so the next attempt re-resolves.
			b.Cache.DeleteID(key)
			return "", fmt.Errorf("update secret: %w", err)
		}
		b.Cache.DeleteSecret(res.ID)
		b.Cache.SetID(key, res.ID)
		return marshalSecret(res), nil
	}

	slog.DebugContext(ctx, fmt.Sprintf("Creating new secret: %s", key))
	res, err := cl.Secrets().Create(key, value, note, orgID, firstOrNil(projectIDs))
	if err != nil {
		return "", fmt.Errorf("create secret: %w", err)
	}
	b.Cache.SetID(key, res.ID)
	return marshalSecret(res), nil
}

// marshalSecret renders the SDK response the same way the read path does,
// so a secret resource has one JSON shape across every verb.
func marshalSecret(res *sdk.SecretResponse) string {
	stored, _ := json.Marshal(res)
	return string(stored)
}

// ResetCache drops all cached secret values; readers repopulate from the API.
func (b *Bitwarden) ResetCache() {
	b.Cache.Reset()
}

// resolveProject returns the project IDs to attach writes to: the
// configured project (resolved by name once per process) or nil when no
// project is configured. An unknown configured name fails loudly, naming
// the project, so a PushSecret retry surfaces the misconfiguration.
func (b *Bitwarden) resolveProject(ctx context.Context, cl sdkClient, orgID string) ([]string, error) {
	if b.projectName == "" {
		return nil, nil
	}
	if b.projectID != "" {
		return []string{b.projectID}, nil
	}
	list, err := cl.Projects().List(orgID)
	if err != nil {
		return nil, fmt.Errorf("list projects to resolve %q: %w", b.projectName, err)
	}
	for _, p := range list.Data {
		if p.Name == b.projectName {
			b.projectID = p.ID
			slog.InfoContext(ctx, fmt.Sprintf("Resolved project %q to %s", b.projectName, p.ID))
			return []string{p.ID}, nil
		}
	}
	return nil, fmt.Errorf("configured project %q not found in organization %s", b.projectName, orgID)
}

func firstOrNil(ids []string) []string {
	if len(ids) == 0 {
		return nil
	}
	return ids
}

package api

import "context"

// SecretClient is the seam between the HTTP handlers and the Bitwarden
// client wrapper. *client.Bitwarden satisfies it; tests supply fakes.
type SecretClient interface {
	ResetCache()
	GetByID(ctx context.Context, id, clientToken string) (string, error)
	GetByKey(ctx context.Context, key, orgID, clientToken string) (string, error)
	UpsertByKey(ctx context.Context, key, value, note, orgID, clientToken string) (string, error)
}

package client

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"bws-cache/internal/pkg/cache"

	sdk "github.com/bitwarden/sdk-go"
	"github.com/google/uuid"
)

type Bitwarden struct {
	Client    sdk.BitwardenClientInterface
	Cache     *cache.Cache
	tokenPath string
	mu        sync.Mutex
}

func New(ttl time.Duration) *Bitwarden {
	bw := Bitwarden{}
	slog.Debug("Setting up cache")
	bw.Cache = cache.New(ttl)
	bw.tokenPath = fmt.Sprintf("/tmp/%s", uuid.New())
	return &bw
}

func (b *Bitwarden) connect(token string) error {
	slog.Debug("Creating new bitwarden client connection")
	return b.newClient(token)
}

func (b *Bitwarden) newClient(token string) error {
	b.Client, _ = sdk.NewBitwardenClient(nil, nil)
	return b.Client.AccessTokenLogin(token, &b.tokenPath)
}

func (b *Bitwarden) close() {
	slog.Debug("Closing bitwarden client connection")
	b.Client.Close()
}

func (b *Bitwarden) GetByID(ctx context.Context, id string, clientToken string) (string, error) {
	slog.DebugContext(ctx, fmt.Sprintf("Getting secret by ID: %s", id))
	value := b.Cache.GetSecret(id)
	if value != "" {
		slog.Debug(fmt.Sprintf("%s ID found in cache", id))
		return value, nil
	}

	slog.Debug(fmt.Sprintf("%s not found in cache, populating", id))

	secret, err := b.getSecretByIDs(ctx, id, clientToken)
	if secret == nil {
		return "", fmt.Errorf("unable to find secret: %s", id)
	}
	if err != nil {
		return "", err
	}

	secretJson, _ := json.Marshal(secret)
	b.Cache.SetSecret(id, string(secretJson))
	return string(secretJson), nil
}

func (b *Bitwarden) GetByKey(ctx context.Context, key string, orgID string, clientToken string) (string, error) {
	secret := ""
	id := b.Cache.GetID(key)
	if id == "" {
		slog.DebugContext(ctx, fmt.Sprintf("%s not found in cache, populating", key))

		keyList, err := b.getSecretList(ctx, orgID, clientToken)
		if err != nil {
			return "", err
		}
		found := false
		for _, keyPair := range keyList.Data {
			b.Cache.SetID(keyPair.Key, keyPair.ID)
			// To avoid running into throttling from Bitwarden only
			// cache the secret value for what was asked for rather
			// than caching every secret returned. The key/id mapping
			// will still expire at the same time necessating another
			// query, but it returns all of them with a single query anyway
			if keyPair.Key == key {
				found = true
			}
		}
		if !found {
			return "", fmt.Errorf("unable to find secret: %s", key)
		}
		// Now that the cache is populated we can get the ID and look it up
		id = b.Cache.GetID(key)
	}
	secret = b.Cache.GetSecret(id)
	if secret == "" {
		slog.DebugContext(ctx, fmt.Sprintf("%s not found in cache, populating", key))
		bwsSecret, err := b.getSecret(ctx, id, clientToken)
		if err != nil {
			return "", err
		}
		storedSecret, _ := json.Marshal(bwsSecret)
		b.Cache.SetSecret(id, string(storedSecret))
		secret = string(storedSecret)
	}
	return secret, nil
}

func (b *Bitwarden) getSecretList(ctx context.Context, orgID string, clientToken string) (*sdk.SecretIdentifiersResponse, error) {
	slog.DebugContext(ctx, "getSecretList: Locking client")
	b.mu.Lock()

	slog.DebugContext(ctx, "getSecretList: Opening client")
	b.connect(clientToken)

	res, err := b.Client.Secrets().List(orgID)
	slog.DebugContext(ctx, "getSecretList: Closing client")
	b.close()

	slog.DebugContext(ctx, "getSecretList: Unlocking client")
	b.mu.Unlock()

	return res, err
}

func (b *Bitwarden) getSecret(ctx context.Context, id string, clientToken string) (*sdk.SecretResponse, error) {
	slog.DebugContext(ctx, "getSecret: Locking client")
	b.mu.Lock()

	slog.DebugContext(ctx, "getSecret: Opening client")
	b.connect(clientToken)

	res, err := b.Client.Secrets().Get(id)
	slog.DebugContext(ctx, "getSecret: Closing Client")
	b.close()

	slog.DebugContext(ctx, "getSecret: Unlocking client")
	b.mu.Unlock()

	return res, err
}

func (b *Bitwarden) getSecretByIDs(ctx context.Context, id string, clientToken string) (*sdk.SecretsResponse, error) {
	slog.DebugContext(ctx, "getSecretByIDs: Locking client")
	b.mu.Lock()

	slog.DebugContext(ctx, "getSecretByIDs: Opening client")
	b.connect(clientToken)

	secretIDs := make([]string, 1)
	secretIDs[0] = id
	res, err := b.Client.Secrets().GetByIDS(secretIDs)

	slog.DebugContext(ctx, "getSecretByIDs: Closing client")
	b.close()

	slog.DebugContext(ctx, "getSecretByIDs: Unlocking client")
	b.mu.Unlock()

	return res, err
}

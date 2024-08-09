package client

import (
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
	return &bw
}

func (b *Bitwarden) connect(token string) error {
	var err error
	slog.Debug("Creating new bitwarden client connection")
	b.Client, err = b.newClient(token)
	if err != nil {
		return err
	}
	return nil
}

func (b *Bitwarden) newClient(token string) (sdk.BitwardenClientInterface, error) {
	bitwardenClient, _ := sdk.NewBitwardenClient(nil, nil)
	if b.tokenPath == "" {
		b.tokenPath = fmt.Sprintf("/tmp/%s", uuid.New())
	}
	err := bitwardenClient.AccessTokenLogin(token, &b.tokenPath)
	if err != nil {
		return nil, err
	}
	return bitwardenClient, nil
}

func (b *Bitwarden) close() {
	slog.Debug("Closing bitwarden client connection")
	b.Client.Close()
}

func (b *Bitwarden) GetByID(id string, clientToken string) (string, error) {
	slog.Debug(fmt.Sprintf("Getting secret by ID: %s", id))
	value := b.Cache.GetSecret(id)
	if value != "" {
		slog.Debug(fmt.Sprintf("%s ID found in cache", id))
		return value, nil
	}

	slog.Debug(fmt.Sprintf("%s not found in cache, populating", id))

	secret, err := b.getSecretByIDs(id, clientToken)
	if secret == nil {
		return "", fmt.Errorf("unable to find secret: %s", id)
	}
	secretJson, _ := json.Marshal(secret)
	b.Cache.SetSecret(id, string(secretJson))
	return string(secretJson), err
}

func (b *Bitwarden) GetByKey(key string, orgID string, clientToken string) (string, error) {
	secret := ""
	id := b.Cache.GetID(key)
	if id == "" {
		slog.Debug(fmt.Sprintf("%s not found in cache, populating", key))

		keyList, err := b.getSecretList(orgID, clientToken)
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

				bwsSecret, err := b.getSecret(keyPair.ID, clientToken)
				if err != nil {
					return "", err
				}
				storedSecret, _ := json.Marshal(bwsSecret)
				b.Cache.SetSecret(keyPair.ID, string(storedSecret))
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
		slog.Debug(fmt.Sprintf("%s not found in cache, populating", key))
		bwsSecret, err := b.getSecret(id, clientToken)
		if err != nil {
			return "", err
		}
		storedSecret, _ := json.Marshal(bwsSecret)
		b.Cache.SetSecret(id, string(storedSecret))
		secret = string(storedSecret)
	}
	return secret, nil
}

func (b *Bitwarden) getSecretList(orgID string, clientToken string) (*sdk.SecretIdentifiersResponse, error) {
	slog.Debug("Locking client")
	b.mu.Lock()
	defer b.mu.Unlock()
	slog.Debug("Client locked")
	b.connect(clientToken)
	defer b.close()

	return b.Client.Secrets().List(orgID)
}

func (b *Bitwarden) getSecret(id string, clientToken string) (*sdk.SecretResponse, error) {
	slog.Debug("Locking client")
	b.mu.Lock()
	defer b.mu.Unlock()
	slog.Debug("Client locked")
	b.connect(clientToken)
	defer b.close()
	return b.Client.Secrets().Get(id)
}

func (b *Bitwarden) getSecretByIDs(id string, clientToken string) (*sdk.SecretsResponse, error) {
	slog.Debug("Locking client")
	b.mu.Lock()
	defer b.mu.Unlock()
	slog.Debug("Client locked")
	b.connect(clientToken)
	defer b.close()
	secretIDs := make([]string, 1)
	secretIDs[0] = id
	return b.Client.Secrets().GetByIDS(secretIDs)
}

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
	Client       sdk.BitwardenClientInterface
	Cache        *cache.Cache
	clientsInUse int
	tokenPath    string
	mu           sync.Mutex
}

func New(ttl time.Duration) *Bitwarden {
	bw := Bitwarden{}
	slog.Debug("Setting up cache")
	bw.Cache = cache.New(ttl)
	return &bw
}

func (b *Bitwarden) Connect(token string) error {
	slog.Debug("Getting connection lock")
	b.mu.Lock()
	slog.Debug("Connection lock acquired")
	defer b.mu.Unlock()
	var err error
	if b.clientsInUse == 0 {
		slog.Debug("Creating new bitwarden client connection")
		b.Client, err = b.newClient(token)
		if err != nil {
			return err
		}
	} else {
		slog.Debug("Client already open/created")
	}
	b.clientsInUse++
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

func (b *Bitwarden) Close() {
	slog.Debug("Getting lock to close connection")
	b.mu.Lock()
	slog.Debug("Connection lock acquired")
	defer b.mu.Unlock()
	b.clientsInUse--
	if b.clientsInUse == 0 {
		slog.Debug("Closing bitwarden client connection")
		b.Client.Close()
		return
	}
	slog.Debug("Client still in use not closing")
}

func (b *Bitwarden) GetByID(id string) (string, error) {
	slog.Debug(fmt.Sprintf("Getting secret by ID: %s", id))
	value := b.Cache.GetSecret(id)
	if value != "" {
		slog.Debug(fmt.Sprintf("%s ID found in cache", id))
		return value, nil
	}
	secretIDs := make([]string, 1)
	secretIDs[0] = id
	slog.Debug(fmt.Sprintf("%s not found in cache, populating", id))
	secret, err := b.Client.Secrets().GetByIDS(secretIDs)
	if secret == nil {
		return "", fmt.Errorf("unable to find secret: %s", id)
	}
	secretJson, _ := json.Marshal(secret)
	b.Cache.SetSecret(id, string(secretJson))
	return string(secretJson), err
}

func (b *Bitwarden) GetByKey(key string, orgID string) (string, error) {
	secret := ""
	id := b.Cache.GetID(key)
	if id == "" {
		slog.Debug(fmt.Sprintf("%s not found in cache, populating", key))
		keyList, err := b.Client.Secrets().List(orgID)
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
				BwsSecret, err := b.Client.Secrets().Get(keyPair.ID)
				if err != nil {
					return "", err
				}
				storedSecret, _ := json.Marshal(BwsSecret)
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
		BwsSecret, err := b.Client.Secrets().Get(id)
		if err != nil {
			return "", err
		}
		storedSecret, _ := json.Marshal(BwsSecret)
		b.Cache.SetSecret(id, string(storedSecret))
		secret = string(storedSecret)
	}
	return secret, nil
}

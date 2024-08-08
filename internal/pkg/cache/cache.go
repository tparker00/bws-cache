package cache

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/jellydator/ttlcache/v3"
)

type Cache struct {
	KeyToID    *ttlcache.Cache[string, string]
	IDtoSecret *ttlcache.Cache[string, string]
}

func New(ttl time.Duration) *Cache {
	slog.Debug(fmt.Sprintf("Setting default ttl for cache to: %s", ttl))
	cache := Cache{}
	cache.KeyToID = ttlcache.New[string, string](ttlcache.WithTTL[string, string](ttl))
	cache.IDtoSecret = ttlcache.New[string, string](ttlcache.WithTTL[string, string](ttl))
	go cache.KeyToID.Start()
	go cache.IDtoSecret.Start()
	return &cache
}

func (cache *Cache) GetID(key string) string {
	if cache.KeyToID.Has(key) {
		slog.Debug(fmt.Sprintf("Found ID for %s", key))
		return cache.KeyToID.Get(key, ttlcache.WithDisableTouchOnHit[string, string]()).Value()
	}
	slog.Debug(fmt.Sprintf("Cache miss for %s", key))
	return ""
}

func (cache *Cache) GetSecret(id string) string {
	if cache.IDtoSecret.Has(id) {
		slog.Debug(fmt.Sprintf("Found secret for %s", id))
		return cache.IDtoSecret.Get(id, ttlcache.WithDisableTouchOnHit[string, string]()).Value()
	}
	slog.Debug(fmt.Sprintf("Cache miss for %s", id))
	return ""
}

func (cache *Cache) SetID(key string, value string) {
	slog.Debug(fmt.Sprintf("Setting ID for key: %s", key))
	cache.KeyToID.Set(key, value, 0)
}

func (cache *Cache) SetSecret(key string, value string) {
	slog.Debug(fmt.Sprintf("Setting secret for id: %s", key))
	cache.IDtoSecret.Set(key, value, 0)
}

func (cache *Cache) Reset() {
	slog.Debug("Resetting cache")
	cache.KeyToID.DeleteAll()
	cache.IDtoSecret.DeleteAll()
}

# Bitwarden Secrets Manager Cache

Golang app implementing a read-through cache for Bitwarden Secrets Manager (BWS) secrets. Inspired by https://github.com/rippleFCL/bws-cache

# Usage

When a secret is queried, not only is the secret cached in memory, but a mapping between ID and key is also cached.

This allows lookup by either ID or key, as shown below.

## Endpoints

* `/id/<string:secret_id>`
* `/key/<string:secret_key>`
* `/reset`

## Authentication

bws-cache delegates authentication to the BWS client library, rather than requiring a defined token for client authentication.

A valid BWS access token should be passed as a bearer token in the `Authorization` header, as shown in the examples below.

## Examples

Query secret by ID: `curl -H "Authorization: Bearer <BWS token>" http://localhost:8080/id/<secret_id>`

Query secret by key: `curl -H "Authorization: Bearer <BWS token>" http://localhost:8080/key/<my_secret>`

Invalidate the secret cache: `curl -H "Authorization: Bearer <BWS token>" http://localhost:8080/reset`

# Run

You can get your BWS organisation ID two ways:
* From BWS CLI:
  * `bws project list` / `bws project get <project_id>` - Your organisation ID is shown in the `organizationId` value of each project returned.
  * `bws secret list` / `bws secret get <secret_id>` - Your organisation ID is shown in the `organizationId` value of each secret returned.
* From browser:
  1. Go to https://vault.bitwarden.com
  2. Open Secrets Manager from the apps list in the top right
  3. Your organisation ID is in the URL like this: `https://vault.bitwarden.com/#/sm/<BWS org ID>`

Docker Run:

```
docker run \
  -p 8080:8080 \
  -e BWS_CACHE_ORG_ID=<org ID> \
  ghcr.io/tparker00/bws-cache:latest
```

Docker Compose:

```yml
services:
  bwscache:
    image: ghcr.io/ripplefcl/bws-cache:latest
    environment:
      BWS_CACHE_ORG_ID: <org ID>
    ports:
      - '8080:8080'
```

## Environment Variables

| Name                     | Info                                                  | Default |
|--------------------------|-------------------------------------------------------|---------|
| `BWS_CACHE_ORG_ID`       | Your BWS organisation ID.                             |         |
| `BWS_CACHE_SECRET_TTL`   | TTL of cached secrets and secret ID-to-key mappings.  | `15m`   |
| `BWS_CACHE_LOG_LEVEL`    | Enable debug logging.                                 | `INFO` |

# How It Works

When a secret is cached, it is cached in memory. Therefore, if the container is restarted, the cache is emptied. 

You can use the `/reset` endpoint if you wish to manually empty the cache.

Since bws-cache allows for secret lookups by key (as opposed to ID), a feature that is not yet natively available in first-party BWS clients, it also caches a map of secret ID/key pairs. We'll call this the keymap cache. The keymap cache expires just as the secret cache does, respecting `SECRET_TTL`.

Upon lookup of a secret ID that **does not** exist in cache, bws-cache will query the BWS API for the secret, store it in the cache, and return the secret object to the client.

Upon lookup of a secret ID that **does** exist in cache, bws-cache will check the timestamp of the secret's cache entry to ensure it has not expired according to `SECRET_TTL` and return the secret object to the client.
If the secret in cache has expired, bws-cache will query the BWS API for the secret, re-cache it, and return the secret object to the client.

Upon lookup of a secret key that **does** exist in cache, bws-cache will check the timestamp of the keymap cache to ensure it has not expired according to `SECRET_TTL` and return the secret object to the client.
If the keymap cache has expired, it will first be refresh as described above, after which the secret object will be returned to the client.

```mermaid
---
title: bws-cache request flow
---
flowchart TD
    Client(Client) --- BwsCache(bws-cache)
    BwsCache -->|ID lookup| IsSecretCached{Secret cached?}
    IsSecretCached -->|Yes| IsSecretExpired{Cached secret older than TTL?}
    IsSecretExpired -->|No| ReturnSecret[Return secret to client]
    IsSecretCached -->|No| QuerySecret[Request secret from BWS API]
    IsSecretExpired -->|Yes| QuerySecret
    QuerySecret --> CacheSecret[Cache secret]
    CacheSecret --> ReturnSecret
    ReturnSecret --> Client
    BwsCache -->|key lookup| IsKeyCacheExist{Keymap cache exists?}
    IsKeyCacheExist -->|Yes| IsKeyCacheExpired{Keymap cache older than TTL?}
    IsKeyCacheExpired -->|No| IsSecretCached
    IsKeyCacheExist -->|No| QuerySecretList[Request list of all secrets from BWS API]
    QuerySecretList -->GenKeyCache[Generate keymap cache]
    GenKeyCache --> IsSecretCached
    IsKeyCacheExpired -->|Yes| QuerySecretList
```
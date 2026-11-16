package client

import (
	"context"
	"testing"

	sdk "github.com/bitwarden/sdk-go"
)

func secretResp(id, key, value string) *sdk.SecretResponse {
	return &sdk.SecretResponse{ID: id, Key: key, Value: value}
}

func TestExistsByKeyFound(t *testing.T) {
	b, f := newTestBitwarden("lucidmate")
	f.secrets["s-1"] = secretResp("s-1", "db-password", "hunter2")

	found, err := b.doExists(context.Background(), f, "db-password", "org")
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("expected found=true for existing key")
	}
	if got := b.Cache.GetID("db-password"); got != "s-1" {
		t.Fatalf("list scan must seed the key->ID mapping, got %q", got)
	}
}

func TestExistsByKeyMissingIsFalseWithoutError(t *testing.T) {
	b, f := newTestBitwarden("lucidmate")
	f.secrets["s-1"] = secretResp("s-1", "db-password", "hunter2")

	found, err := b.doExists(context.Background(), f, "other-key", "org")
	if err != nil {
		t.Fatalf("missing key must not error: %v", err)
	}
	if found {
		t.Fatal("expected found=false for missing key")
	}
}

func TestExistsByKeyIgnoresStaleCachedMapping(t *testing.T) {
	// A cached mapping can go stale after an out-of-band delete; a false
	// "exists" would make ESO's IfNotExists skip pushes forever, so the
	// probe must resolve from the list, not the cache.
	b, f := newTestBitwarden("lucidmate")
	b.Cache.SetID("deleted-key", "s-gone")

	found, err := b.doExists(context.Background(), f, "deleted-key", "org")
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Fatal("stale mapping must not report exists")
	}
	if got := b.Cache.GetID("deleted-key"); got != "" {
		t.Fatalf("stale mapping must be dropped, got %q", got)
	}
}

func TestExistsByKeyListErrorPropagates(t *testing.T) {
	b, f := newTestBitwarden("lucidmate")
	f.failList = true

	if _, err := b.doExists(context.Background(), f, "db-password", "org"); err == nil {
		t.Fatal("list error must propagate")
	}
}

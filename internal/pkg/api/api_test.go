package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"bws-cache/internal/pkg/metrics"
)

type fakeClient struct {
	getByID     func(ctx context.Context, id, token string) (string, error)
	getByKey    func(ctx context.Context, key, orgID, token string) (string, error)
	upsertByKey func(ctx context.Context, key, value, note, orgID, token string) (string, error)
}

func (f *fakeClient) ResetCache() {}

func (f *fakeClient) GetByID(ctx context.Context, id, token string) (string, error) {
	return f.getByID(ctx, id, token)
}
func (f *fakeClient) GetByKey(ctx context.Context, key, orgID, token string) (string, error) {
	return f.getByKey(ctx, key, orgID, token)
}
func (f *fakeClient) UpsertByKey(ctx context.Context, key, value, note, orgID, token string) (string, error) {
	return f.upsertByKey(ctx, key, value, note, orgID, token)
}

// newTestAPI builds an API with the client seam replaced by a fake,
// mirroring New() but without the live Bitwarden connection.
func newTestAPI(fc SecretClient) (http.Handler, *API) {
	a := &API{
		SecretTTL: time.Minute,
		OrgID:     "org-123",
		Client:    fc,
		Metrics:   metrics.New(),
	}
	return a.routes(time.Minute), a
}

func TestUpsertRequiresAuth(t *testing.T) {
	called := false
	h, _ := newTestAPI(&fakeClient{
		upsertByKey: func(context.Context, string, string, string, string, string) (string, error) {
			called = true
			return "{}", nil
		},
	})
	body, _ := json.Marshal(map[string]string{"value": "v"})
	req := httptest.NewRequest(http.MethodPost, "/key/my-key", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 without auth, got %d", rec.Code)
	}
	if called {
		t.Fatal("upsert must not be called without auth")
	}
}

func TestUpsertCreatesOrUpdatesByKey(t *testing.T) {
	var gotKey, gotValue, gotOrg, gotToken string
	h, _ := newTestAPI(&fakeClient{
		upsertByKey: func(_ context.Context, key, value, note, orgID, token string) (string, error) {
			gotKey, gotValue, gotOrg, gotToken = key, value, orgID, token
			return `{"key":"my-key","value":"stored"}`, nil
		},
	})
	body, _ := json.Marshal(map[string]string{"value": "new-password"})
	req := httptest.NewRequest(http.MethodPost, "/key/my-key", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer tok-1")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if gotKey != "my-key" || gotValue != "new-password" || gotOrg != "org-123" || gotToken != "tok-1" {
		t.Fatalf("unexpected upsert args: key=%q value=%q org=%q token=%q", gotKey, gotValue, gotOrg, gotToken)
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("response not JSON: %v", err)
	}
}

func TestUpsertRejectsMissingValue(t *testing.T) {
	h, _ := newTestAPI(&fakeClient{
		upsertByKey: func(context.Context, string, string, string, string, string) (string, error) {
			t.Fatal("must not call upsert without a value")
			return "", nil
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/key/my-key", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("Authorization", "Bearer tok-1")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing value, got %d", rec.Code)
	}
}

func TestUpsertRejectsInvalidJSON(t *testing.T) {
	h, _ := newTestAPI(&fakeClient{
		upsertByKey: func(context.Context, string, string, string, string, string) (string, error) {
			t.Fatal("must not call upsert on undecodable body")
			return "", nil
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/key/my-key", bytes.NewReader([]byte(`not-json`)))
	req.Header.Set("Authorization", "Bearer tok-1")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid JSON, got %d: %s", rec.Code, rec.Body.String())
	}
}

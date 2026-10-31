package client

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	sdk "github.com/bitwarden/sdk-go"

	"bws-cache/internal/pkg/cache"
)

// fakeSDK is the shared stateful fake at the SDK seam: an in-memory
// Bitwarden used by every client-layer test. One implementation,
// consistent behavior, exactly like a real org's data.
type fakeSDK struct {
	sdk.BitwardenClientInterface // embed: unused methods panic loudly if ever called

	secrets   map[string]*sdk.SecretResponse // by ID
	projects  map[string]string              // ID -> name
	nextID    int
	failGet   bool
	gotUpdate *struct {
		id         string
		projectIDs []string
	}
	gotCreateProjectIDs []string
}

func newFakeSDK() *fakeSDK {
	return &fakeSDK{secrets: map[string]*sdk.SecretResponse{}, projects: map[string]string{}}
}

func (f *fakeSDK) Secrets() sdk.SecretsInterface   { return fakeSecrets{f: f} }
func (f *fakeSDK) Projects() sdk.ProjectsInterface { return fakeProjects{f: f} }

type fakeSecrets struct {
	sdk.SecretsInterface // unused secret ops panic loudly if called
	f                    *fakeSDK
}
type fakeProjects struct {
	sdk.ProjectsInterface // unused project ops panic loudly if called
	f                     *fakeSDK
}

func (f fakeSecrets) List(orgID string) (*sdk.SecretIdentifiersResponse, error) {
	res := &sdk.SecretIdentifiersResponse{}
	for id, s := range f.f.secrets {
		res.Data = append(res.Data, sdk.SecretIdentifierResponse{ID: id, Key: s.Key, OrganizationID: orgID})
	}
	return res, nil
}
func (f fakeSecrets) Get(id string) (*sdk.SecretResponse, error) {
	if f.f.failGet {
		return nil, errBoom
	}
	s, ok := f.f.secrets[id]
	if !ok {
		return nil, errNotFound
	}
	return s, nil
}
func (f fakeSecrets) Update(id, key, value, note, orgID string, projectIDs []string) (*sdk.SecretResponse, error) {
	f.f.gotUpdate = &struct {
		id         string
		projectIDs []string
	}{id, projectIDs}
	s := f.f.secrets[id]
	s.Key, s.Value, s.Note = key, value, note
	return s, nil
}
func (f fakeSecrets) Create(key, value, note, orgID string, projectIDs []string) (*sdk.SecretResponse, error) {
	f.f.gotCreateProjectIDs = projectIDs
	f.f.nextID++
	id := strings.Repeat("0", 31) + string(rune('a'+f.f.nextID))
	var pid *string
	if len(projectIDs) == 1 {
		pid = &projectIDs[0]
	}
	s := &sdk.SecretResponse{ID: id, Key: key, Value: value, Note: note, ProjectID: pid}
	f.f.secrets[id] = s
	return s, nil
}
func (f fakeProjects) List(orgID string) (*sdk.ProjectsResponse, error) {
	res := &sdk.ProjectsResponse{}
	for id, name := range f.f.projects {
		res.Data = append(res.Data, sdk.ProjectResponse{ID: id, Name: name})
	}
	return res, nil
}

var (
	errBoom     = errorsNew("boom")
	errNotFound = errorsNew("not found")
)

func newTestBitwarden(projectName string) (*Bitwarden, *fakeSDK) {
	b := &Bitwarden{Cache: cache.New(time.Minute), projectName: projectName}
	f := newFakeSDK()
	b.Client = f
	return b, f
}

func TestUpsertCreateLandsInConfiguredProject(t *testing.T) {
	b, f := newTestBitwarden("lucidmate")
	f.projects["proj-1"] = "lucidmate"
	raw, err := b.doUpsert(context.Background(), f, "db-password", "hunter2", "", "org")
	if err != nil {
		t.Fatal(err)
	}
	if len(f.gotCreateProjectIDs) != 1 || f.gotCreateProjectIDs[0] != "proj-1" {
		t.Fatalf("create must target configured project, got %v", f.gotCreateProjectIDs)
	}
	var resp map[string]any
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("response must be the full SecretResponse JSON, got %q: %v", raw, err)
	}
	if resp["key"] != "db-password" || resp["value"] != "hunter2" || resp["projectId"] != "proj-1" {
		t.Fatalf("response fields wrong: %v", resp)
	}
	if got := b.Cache.GetID("db-password"); got != resp["id"] {
		t.Fatalf("key->ID mapping must be seeded, got %q want %v", got, resp["id"])
	}
}

func TestUpsertUpdateUsesConfiguredProjectWithoutGet(t *testing.T) {
	b, f := newTestBitwarden("lucidmate")
	f.projects["proj-1"] = "lucidmate"
	existing := &sdk.SecretResponse{ID: "s-1", Key: "db-password", Value: "old", ProjectID: strPtr("proj-OTHER")}
	f.secrets["s-1"] = existing
	b.Cache.SetID("db-password", "s-1")
	f.failGet = true // the preserve-membership Get must NOT run when a project is configured

	if _, err := b.doUpsert(context.Background(), f, "db-password", "new", "", "org"); err != nil {
		t.Fatal(err)
	}
	if f.gotUpdate == nil || f.gotUpdate.id != "s-1" {
		t.Fatal("update must target the existing secret")
	}
	if len(f.gotUpdate.projectIDs) != 1 || f.gotUpdate.projectIDs[0] != "proj-1" {
		t.Fatalf("update must move membership to configured project, got %v", f.gotUpdate.projectIDs)
	}
	if got := b.Cache.GetSecret("s-1"); got != "" {
		t.Fatalf("written value must be invalidated, got %q", got)
	}
}

func TestUpsertUnknownProjectNameFailsLoudly(t *testing.T) {
	b, _ := newTestBitwarden("does-not-exist")
	if _, err := b.doUpsert(context.Background(), b.Client, "k", "v", "", "org"); err == nil || !strings.Contains(err.Error(), "does-not-exist") {
		t.Fatalf("expected loud failure naming the project, got %v", err)
	}
}

func TestUpsertWithoutProjectPreservesMembershipViaGet(t *testing.T) {
	b, f := newTestBitwarden("")
	existing := &sdk.SecretResponse{ID: "s-1", Key: "k", Value: "old", ProjectID: strPtr("proj-KEEP")}
	f.secrets["s-1"] = existing
	b.Cache.SetID("k", "s-1")

	if _, err := b.doUpsert(context.Background(), f, "k", "new", "", "org"); err != nil {
		t.Fatal(err)
	}
	if len(f.gotUpdate.projectIDs) != 1 || f.gotUpdate.projectIDs[0] != "proj-KEEP" {
		t.Fatalf("unconfigured upsert must preserve membership, got %v", f.gotUpdate.projectIDs)
	}
}

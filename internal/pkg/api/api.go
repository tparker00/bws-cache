package api

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"bws-cache/internal/pkg/client"
	c "bws-cache/internal/pkg/config"
	"bws-cache/internal/pkg/metrics"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/httplog/v2"
	"github.com/go-chi/telemetry"
	"github.com/pkg/errors"
)

type API struct {
	SecretTTL time.Duration
	WebTTL    time.Duration
	OrgID     string
	Client    SecretClient
	Metrics   *metrics.BwsMetrics
}

func New(config *c.Config) http.Handler {
	api := API{
		SecretTTL: config.SecretTTL,
		OrgID:     config.OrgID,
		Metrics:   metrics.New(),
	}
	slog.Debug("Creating new bitwarden client connection")
	api.Client = client.New(api.SecretTTL)
	slog.Debug("Client created")
	return api.routes(config.WebTTL)
}

func (api *API) routes(webTTL time.Duration) http.Handler {
	// Logger
	logger := httplog.NewLogger("bws-cache", httplog.Options{
		JSON:             true,
		LogLevel:         slog.LevelInfo,
		Concise:          false,
		RequestHeaders:   true,
		MessageFieldName: "msg",
		TimeFieldFormat:  time.RFC3339,
		TimeFieldName:    "time",
		Tags: map[string]string{
			"version": c.Version,
			"commit":  c.Commit,
		},
		QuietDownRoutes: []string{
			"/",
			"/metrics",
			"/ping",
		},
		QuietDownPeriod: 10 * time.Minute,
	})

	router := chi.NewRouter()
	router.Use(middleware.RequestID)
	router.Use(middleware.RealIP)
	router.Use(httplog.RequestLogger(logger))
	router.Use(middleware.Recoverer)
	router.Use(middleware.Timeout(webTTL))
	// telemetry.Collector middleware mounts /metrics endpoint
	// with prometheus metrics collector.
	router.Use(telemetry.Collector(telemetry.Config{
		AllowAny: true,
	}, []string{"/"})) // path prefix filters records generic http request metrics
	router.Use(middleware.Heartbeat("/ping"))
	// Enable profiler
	router.Mount("/debug", middleware.Profiler())

	slog.Debug("Router middleware setup finished")

	router.Route("/id", func(r chi.Router) {
		r.Get("/{secret_id}", api.getSecretByID)
	})
	router.Route("/key", func(r chi.Router) {
		r.Get("/{secret_key}", api.getSecretByKey)
		// Create-or-update by key: backs PushSecret writes from
		// external-secrets (webhook provider push config).
		r.Post("/{secret_key}", api.upsertSecretByKey)
	})
	router.Get("/reset", api.resetConnection)

	return router
}

func (api *API) getSecretByID(w http.ResponseWriter, r *http.Request) {
	tag := make(map[string]string)
	tag["endpoint"] = "id"
	api.Metrics.Counter("get", tag)
	ctx := r.Context()
	slog.DebugContext(ctx, "Getting secret by ID")
	token, err := getAuthToken(r)
	slog.DebugContext(ctx, "Got auth token")
	if err != nil {
		slog.Error(fmt.Sprintf("%+v", err))
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	id := chi.URLParam(r, "secret_id")

	slog.DebugContext(ctx, fmt.Sprintf("Getting secret by ID: %s", id))
	span := api.Metrics.RecordSpan("get", tag)
	defer span.Stop()
	res, err := api.Client.GetByID(ctx, id, token)
	if err != nil {
		slog.ErrorContext(ctx, fmt.Sprintf("%+v", err))
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	slog.DebugContext(ctx, "Got secret")
	fmt.Fprint(w, res)
}

func (api *API) getSecretByKey(w http.ResponseWriter, r *http.Request) {
	tag := make(map[string]string)
	tag["endpoint"] = "key"
	api.Metrics.Counter("get", tag)
	ctx := r.Context()
	slog.DebugContext(ctx, "Getting secret by key")
	token, err := getAuthToken(r)
	if err != nil {
		slog.ErrorContext(ctx, fmt.Sprintf("%+v", err))
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	key := chi.URLParam(r, "secret_key")

	slog.DebugContext(ctx, fmt.Sprintf("Searching for key: %s", key))
	span := api.Metrics.RecordSpan("get", tag)
	defer span.Stop()
	res, err := api.Client.GetByKey(ctx, key, api.OrgID, token)
	if err != nil {
		slog.ErrorContext(ctx, fmt.Sprintf("%+v", err))
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	slog.DebugContext(ctx, "Got key")
	fmt.Fprint(w, res)
}

func (api *API) resetConnection(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	slog.InfoContext(ctx, "Resetting cache")

	api.Client.ResetCache()
	tag := make(map[string]string)
	tag["endpoint"] = "cache"
	api.Metrics.Counter("get", tag)
	slog.InfoContext(ctx, "Cache reset")
}

func getAuthToken(r *http.Request) (string, error) {
	prefix := "Bearer "
	authHeader := r.Header.Get("Authorization")
	reqToken := strings.TrimPrefix(authHeader, prefix)
	if authHeader == "" || reqToken == "" {
		return "", errors.Errorf("No token or invalid token sent")
	}
	return reqToken, nil
}

// upsertSecretByKey backs PushSecret: POST /key/{secret_key} with body
// {"value": "..."} creates the secret in the organization when absent or
// updates it in place when present, both keyed by secret name.
func (api *API) upsertSecretByKey(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	token, err := getAuthToken(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	key := chi.URLParam(r, "secret_key")

	var body struct {
		Value string `json:"value"`
		Note  string `json:"note"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if body.Value == "" {
		http.Error(w, `missing required field "value"`, http.StatusBadRequest)
		return
	}

	tag := map[string]string{"endpoint": "upsert"}
	api.Metrics.Counter("upsert", tag)
	span := api.Metrics.RecordSpan("upsert", tag)
	defer span.Stop()

	res, err := api.Client.UpsertByKey(ctx, key, body.Value, body.Note, api.OrgID, token)
	if err != nil {
		slog.ErrorContext(ctx, fmt.Sprintf("%+v", err))
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, res)
}

package api

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"bws-cache/internal/pkg/client"
	"bws-cache/internal/pkg/config"
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
	Client    *client.Bitwarden
	Metrics   *metrics.BwsMetrics
}

func New(config *config.Config) http.Handler {
	api := API{
		SecretTTL: config.SecretTTL,
		OrgID:     config.OrgID,
		Metrics:   metrics.New(),
	}

	// Logger
	logger := httplog.NewLogger("bws-cache", httplog.Options{
		// JSON:             true,
		LogLevel:         slog.LevelInfo,
		Concise:          true,
		RequestHeaders:   true,
		MessageFieldName: "message",
		TimeFieldFormat:  time.RFC850,
		Tags: map[string]string{
			"version": "v0.1.8",
			"env":     "prod",
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
	router.Use(middleware.Timeout(config.WebTTL))
	// telemetry.Collector middleware mounts /metrics endpoint
	// with prometheus metrics collector.
	router.Use(telemetry.Collector(telemetry.Config{
		AllowAny: true,
	}, []string{"/"})) // path prefix filters records generic http request metrics
	router.Use(middleware.Heartbeat("/ping"))
	// Enable profiler
	router.Mount("/debug", middleware.Profiler())

	slog.Debug("Router middleware setup finished")

	slog.Debug("Creating new bitwarden client connection")
	api.Client = client.New(api.SecretTTL)
	slog.Debug("Client created")

	router.Route("/id", func(r chi.Router) {
		r.Get("/{secret_id}", api.getSecretByID)
	})
	router.Route("/key", func(r chi.Router) {
		r.Get("/{secret_key}", api.getSecretByKey)
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

	api.Client.Cache.Reset()
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

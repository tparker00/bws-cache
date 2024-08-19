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

	router := chi.NewRouter()
	router.Use(middleware.RequestID)
	router.Use(middleware.RealIP)
	router.Use(middleware.Logger)
	router.Use(middleware.Recoverer)
	router.Use(middleware.Timeout(config.WebTTL))
	// telemetry.Collector middleware mounts /metrics endpoint
	// with prometheus metrics collector.
	router.Use(telemetry.Collector(telemetry.Config{
		AllowAny: true,
	}, []string{"/"})) // path prefix filters records generic http request metrics

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
	api.Metrics.Counter("get_id")
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
	span := api.Metrics.RecordSpan("secret_by_id", nil)
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
	api.Metrics.Counter("get_key")
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
	span := api.Metrics.RecordSpan("secret_by_key", nil)
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
	api.Metrics.Counter("cache_reset")
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

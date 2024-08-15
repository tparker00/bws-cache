package api

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"bws-cache/internal/pkg/client"
	"bws-cache/internal/pkg/config"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type API struct {
	SecretTTL time.Duration
	WebTTL    time.Duration
	OrgID     string
	Client    *client.Bitwarden
}

func New(config *config.Config) http.Handler {
	api := API{
		SecretTTL: config.SecretTTL,
		OrgID:     config.OrgID,
	}

	router := chi.NewRouter()
	router.Use(middleware.RequestID)
	router.Use(middleware.RealIP)
	router.Use(middleware.Logger)
	router.Use(middleware.Recoverer)
	router.Use(middleware.Timeout(config.WebTTL))

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
	router.Handle("/metrics", promhttp.Handler())

	return router
}

func (api *API) getSecretByID(w http.ResponseWriter, r *http.Request) {
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

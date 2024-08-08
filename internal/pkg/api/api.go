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
	slog.Debug("Getting secret by ID")
	token, err := getAuthToken(r)
	slog.Debug("Got auth token")
	if err != nil {
		slog.Error(fmt.Sprintf("%+v", err))
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	id := chi.URLParam(r, "secret_id")

	slog.Debug("Connecting to bitwarden service")
	api.Client.Connect(token)
	defer api.Client.Close()
	slog.Debug("Connected to bitwarden service")

	slog.Debug(fmt.Sprintf("Getting secret by ID: %s", id))
	res, err := api.Client.GetByID(id)
	if err != nil {
		slog.Error(fmt.Sprintf("%+v", err))
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	slog.Debug("Got secret")
	fmt.Fprint(w, res)
}

func (api *API) getSecretByKey(w http.ResponseWriter, r *http.Request) {
	slog.Debug("Getting secret by key")
	token, err := getAuthToken(r)
	if err != nil {
		slog.Error(fmt.Sprintf("%+v", err))
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	key := chi.URLParam(r, "secret_key")

	slog.Debug("Connecting to bitwarden service")
	api.Client.Connect(token)
	defer api.Client.Close()
	slog.Debug("Connected to bitwarden service")

	slog.Debug("Searching for key")
	res, err := api.Client.GetByKey(key, api.OrgID)
	if err != nil {
		slog.Error(fmt.Sprintf("%+v", err))
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	slog.Debug("Got key")
	fmt.Fprint(w, res)
}

func (api *API) resetConnection(w http.ResponseWriter, r *http.Request) {
	slog.Info("Resetting cache")

	api.Client.Cache.Reset()
	slog.Info("Cache reset")
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

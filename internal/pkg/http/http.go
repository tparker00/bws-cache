package http

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"bws-cache/internal/pkg/api"
	"bws-cache/internal/pkg/config"
)

func Start(ctx context.Context, config *config.Config) (chan error, *http.Server) {
	slog.Debug("Starting http handler")
	httpHandler := api.New(config)

	server := http.Server{
		Addr:    fmt.Sprintf(":%d", config.Port),
		Handler: httpHandler,
	}
	slog.Info(fmt.Sprintf("Server started on port: %d", config.Port))

	errCh := make(chan error)

	go func() {
		defer close(errCh)

		err := server.ListenAndServe()
		if err == http.ErrServerClosed {
			return
		}
		select {
		case errCh <- err:
		case <-ctx.Done():
		}
	}()

	return errCh, &server
}

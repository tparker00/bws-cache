package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

	c "bws-cache/internal/pkg/config"
	h "bws-cache/internal/pkg/http"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "bws-cache",
	Short: "Caching server for bitwarden",
	Long:  "bws-cache is a re-implementation of bws-cache in golang to cache credentials from a bitwarden secret store",
}

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Starts bws-cache",
	Long:  "Starts bws-cache",
	Run: func(cmd *cobra.Command, args []string) {
		start()
	},
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Display build/version",
	Long:  "Display build/version",
	Run: func(cmd *cobra.Command, args []string) {
		version()
	},
}

var loggingLevel = new(slog.LevelVar)

func init() {
	rootCmd.AddCommand(startCmd)
	rootCmd.AddCommand(versionCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		slog.Error("%v", err)
		os.Exit(1)
	}
}

func version() {
	fmt.Println(c.Version)
}

func start() {
	config := &c.Config{}
	c.LoadConfig(config)

	logger := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: loggingLevel})
	slog.SetDefault(slog.New(logger))
	loggingLevel.Set(getLoggerLevel(config.LogLevel))
	slog.Info("Starting")

	if config.OrgID == "" {
		slog.Error("Org ID must be specified")
		os.Exit(1)
	}

	ctx, cancelF := context.WithCancel(context.Background())
	defer cancelF()

	httpErrCh, server := h.Start(ctx, config)

	errCh := make(chan error)
	select {
	case err := <-httpErrCh:
		select {
		case errCh <- err:
		case <-ctx.Done():
		}
	case <-ctx.Done():
	}

	if err := server.Shutdown(ctx); err != nil {
		slog.Error("Failed to shutdown properly: %v", err)
	}
}

func getLoggerLevel(config string) slog.Level {
	switch strings.ToUpper(config) {
	case "DEBUG":
		return slog.LevelDebug
	case "WARN":
		return slog.LevelWarn
	case "ERROR":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

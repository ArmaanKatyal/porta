package main

import (
	"context"
	"crypto/tls"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/ArmaanKatyal/go-api-gateway/pkg/config"
	"github.com/ArmaanKatyal/go-api-gateway/pkg/logging"
	"github.com/ArmaanKatyal/go-api-gateway/pkg/router"
)

func main() {
	// Initialize logger
	opts := logging.PrettyHandlerOptions{
		SlogOpts: slog.HandlerOptions{
			Level: slog.LevelDebug,
		},
	}
	handler := logging.NewPrettyHandler(os.Stdout, opts)
	logger := slog.New(handler)
	slog.SetDefault(logger)

	// Load configuration
	config.LoadConf()
	// Initialize registry
	rh := router.NewRequestHandler()
	router := router.InitializeRoutes(rh)

	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
	}
	server := &http.Server{
		Addr:         ":" + config.AppConfig.Server.Port,
		Handler:      router,
		ReadTimeout:  time.Duration(config.AppConfig.Server.ReadTimeout) * time.Second,
		WriteTimeout: time.Duration(config.AppConfig.Server.WriteTimeout) * time.Second,
		TLSConfig:    tlsConfig,
	}

	slog.Info("API Gateway started", "port", config.AppConfig.Server.Port)
	go func() {
		// Start server
		if config.TLSEnabled() {
			if err := server.ListenAndServeTLS(config.GetCertFile(), config.GetKeyFile()); err != nil {
				slog.Error("Error starting server", "error", err.Error())
				os.Exit(1)
			}
		} else {
			if err := server.ListenAndServe(); err != nil {
				slog.Error("Error starting server", "error", err.Error())
				os.Exit(1)
			}
		}
	}()

	// Graceful shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt)

	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(config.AppConfig.Server.GracefulTimeout)*time.Second)
	defer cancel()
	slog.Info("Gracefully shutting down server")
	if err := server.Shutdown(ctx); err != nil {
		slog.Error("Error shutting down server", "error", err.Error())
		os.Exit(1)
	}
}

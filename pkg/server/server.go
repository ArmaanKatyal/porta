package server

import (
	"context"
	"crypto/tls"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/ArmaanKatyal/porta/pkg/config"
	"github.com/ArmaanKatyal/porta/pkg/logging"
	"github.com/ArmaanKatyal/porta/pkg/router"
)

// ServerConfig holds all the configuration required to start the HTTP server.
type ServerConfig struct {
	Addr            string
	TLSConfig       *tls.Config
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	GracefulTimeout time.Duration
	CertFile        string
	KeyFile         string
	TLSEnabled      bool
}

// NewServer creates and returns a configured *http.Server.
func NewServer(cfg ServerConfig, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:         cfg.Addr,
		Handler:      handler,
		TLSConfig:    cfg.TLSConfig,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
	}
}

// Start initializes configuration, sets up logging, creates the server, and runs it.
// It returns an error if the server fails unexpectedly.
func Start() error {
	// 1) Configure logging
	opts := logging.PrettyHandlerOptions{
		SlogOpts: slog.HandlerOptions{
			Level: slog.LevelDebug, // Adjust as needed
		},
	}
	handler := logging.NewPrettyHandler(os.Stdout, opts)
	logger := slog.New(handler)
	slog.SetDefault(logger)

	// 2) Load configuration
	conf, err := config.Load("")
	if err != nil {
		return err
	}
	config.AppConfig = conf

	// 3) Build the router
	rh := router.NewRequestHandler()
	r := router.InitializeRoutes(rh)

	// 4) Check TLS requirements
	certFile := config.GetCertFile()
	keyFile := config.GetKeyFile()
	tlsEnabled := config.TLSEnabled()

	if tlsEnabled {
		if _, err := os.Stat(certFile); os.IsNotExist(err) {
			slog.Warn("Certificate file not found, disabling TLS", "path", certFile)
			tlsEnabled = false
		}
		if _, err := os.Stat(keyFile); os.IsNotExist(err) {
			slog.Warn("Key file not found, disabling TLS", "path", keyFile)
			tlsEnabled = false
		}
	}

	// 5) Create the server config
	srvConfig := ServerConfig{
		Addr:            ":" + config.AppConfig.Server.Port,
		TLSConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
		ReadTimeout:     time.Duration(config.AppConfig.Server.ReadTimeout) * time.Second,
		WriteTimeout:    time.Duration(config.AppConfig.Server.WriteTimeout) * time.Second,
		GracefulTimeout: time.Duration(config.AppConfig.Server.GracefulTimeout) * time.Second,
		CertFile:        certFile,
		KeyFile:         keyFile,
		TLSEnabled:      tlsEnabled,
	}

	// 6) Create the http.Server
	srv := NewServer(srvConfig, r)

	// 7) Handle graceful shutdown signals
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt)

	slog.Info("API Gateway starting",
		"port", config.AppConfig.Server.Port,
		"tlsEnabled", tlsEnabled,
	)

	// 8) Run the server and wait for shutdown/interrupt/error
	if err := StartServer(srv, srvConfig, stop); err != nil && err != http.ErrServerClosed {
		slog.Error("server encountered an error", "error", err)
		return err
	}

	slog.Info("Server shutdown complete")
	return nil
}

// StartServer runs the server in a goroutine and gracefully shuts down on interrupt.
func StartServer(srv *http.Server, cfg ServerConfig, stop <-chan os.Signal) error {
	serverErrCh := make(chan error, 1)

	// Start the server in a goroutine
	go func() {
		if cfg.TLSEnabled {
			serverErrCh <- srv.ListenAndServeTLS(cfg.CertFile, cfg.KeyFile)
		} else {
			serverErrCh <- srv.ListenAndServe()
		}
	}()

	// Block until either we get a signal or the server fails
	select {
	case <-stop:
		ctx, cancel := context.WithTimeout(context.Background(), cfg.GracefulTimeout)
		defer cancel()
		return srv.Shutdown(ctx)

	case err := <-serverErrCh:
		// If the server stopped unexpectedly (e.g., port conflict)
		return err
	}
}

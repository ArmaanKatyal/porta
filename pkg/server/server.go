package server

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
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

// Start initializes configuration, sets up logging, creates both servers, and runs them.
// It will block until you hit Ctrl+C, then gracefully shut both down.
func Start() error {
	opts := logging.PrettyHandlerOptions{
		SlogOpts: slog.HandlerOptions{Level: slog.LevelDebug},
	}
	handler := logging.NewPrettyHandler(os.Stdout, opts)
	logger := slog.New(handler)
	slog.SetDefault(logger)

	conf, err := config.Load("")
	if err != nil {
		return err
	}

	rh := router.NewRequestHandler(conf)
	publicRouter := router.InitializeRoutes(rh)
	adminRouter := router.InitializeAdminRoutes(rh)

	tlsEnabled := conf.Server.TLS.Enabled
	if tlsEnabled {
		if _, err := os.Stat(conf.Server.TLS.CertFile); os.IsNotExist(err) {
			slog.Warn("Certificate not found, disabling TLS", "path", conf.Server.TLS.CertFile)
			tlsEnabled = false
		}
		if _, err := os.Stat(conf.Server.TLS.KeyFile); os.IsNotExist(err) {
			slog.Warn("Key not found, disabling TLS", "path", conf.Server.TLS.KeyFile)
			tlsEnabled = false
		}
	}

	outerCfg := ServerConfig{
		Addr:            ":" + conf.Server.Port,
		TLSConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
		ReadTimeout:     time.Duration(conf.Server.ReadTimeout) * time.Second,
		WriteTimeout:    time.Duration(conf.Server.WriteTimeout) * time.Second,
		GracefulTimeout: time.Duration(conf.Server.GracefulTimeout) * time.Second,
		CertFile:        conf.Server.TLS.CertFile,
		KeyFile:         conf.Server.TLS.KeyFile,
		TLSEnabled:      tlsEnabled,
	}

	adminTlsEnabled := conf.Admin.TLS.Enabled
	if adminTlsEnabled {
		if _, err := os.Stat(conf.Admin.TLS.CertFile); os.IsNotExist(err) {
			slog.Warn("Certificate not found, disabling TLS", "path", conf.Admin.TLS.CertFile)
			adminTlsEnabled = false
		}
		if _, err := os.Stat(conf.Admin.TLS.KeyFile); os.IsNotExist(err) {
			slog.Warn("Key not found, disabling TLS", "path", conf.Admin.TLS.KeyFile)
			adminTlsEnabled = false
		}
	}

	adminCfg := ServerConfig{
		Addr:            ":" + conf.Admin.Port,
		TLSConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
		ReadTimeout:     time.Duration(conf.Admin.ReadTimeout) * time.Second,
		WriteTimeout:    time.Duration(conf.Admin.WriteTimeout) * time.Second,
		GracefulTimeout: time.Duration(conf.Admin.GracefulTimeout) * time.Second,
		CertFile:        conf.Admin.TLS.CertFile,
		KeyFile:         conf.Admin.TLS.KeyFile,
		TLSEnabled:      adminTlsEnabled,
	}

	publicSrv := NewServer(outerCfg, publicRouter)
	adminSrv := NewServer(adminCfg, adminRouter)

	slog.Info("Starting servers",
		"public_port", conf.Server.Port,
		"admin_port", conf.Admin.Port,
		"tls", tlsEnabled,
	)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)

	stop := make(chan struct{}) // closing this will wake *all* listeners
	go func() {
		<-sigCh
		close(stop)
	}()

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		if err := runServer(publicSrv, outerCfg, stop); err != nil && err != http.ErrServerClosed {
			slog.Error("public server error", "err", err)
		}
	}()
	go func() {
		defer wg.Done()
		if err := runServer(adminSrv, adminCfg, stop); err != nil && err != http.ErrServerClosed {
			slog.Error("admin server error", "err", err)
		}
	}()

	wg.Wait()
	slog.Info("both servers shut down cleanly")
	return nil
}

// runServer starts ListenAndServe( TLS ) and shuts down when `stop` is closed.
func runServer(srv *http.Server, cfg ServerConfig, stop <-chan struct{}) error {
	errCh := make(chan error, 1)

	go func() {
		if cfg.TLSEnabled {
			errCh <- srv.ListenAndServeTLS(cfg.CertFile, cfg.KeyFile)
		} else {
			errCh <- srv.ListenAndServe()
		}
	}()

	select {
	case <-stop:
		// graceful shutdown
		ctx, cancel := context.WithTimeout(context.Background(), cfg.GracefulTimeout)
		defer cancel()
		return srv.Shutdown(ctx)
	case err := <-errCh:
		// e.g. port in use
		return fmt.Errorf("listen error on %s: %w", cfg.Addr, err)
	}
}

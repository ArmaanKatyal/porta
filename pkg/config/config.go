package config

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/sony/gobreaker/v2"
	"github.com/spf13/viper"
)

// AppConfig is the global configuration object
var AppConfig *Conf

func init() {
	serviceName := "porta"

	viper.SetDefault("server.port", "8080")
	viper.SetDefault("server.host", "localhost")
	viper.SetDefault("server.tls.port", "8433")
	viper.SetDefault("server.metrics.prefix", serviceName)
	viper.SetDefault("server.metrics.buckets", []float64{0.005, 0.01, 0.025, 0.05, 0.1})
}

type CircuitSettings struct {
	Enabled      bool    `yaml:"enabled"`
	Timeout      uint    `yaml:"timeout"`
	Interval     uint    `yaml:"interval"`
	FailureRatio float64 `yaml:"failureRatio"`
}

func (cs *CircuitSettings) Into(name string) gobreaker.Settings {
	return gobreaker.Settings{
		Name:     "cb-" + name,
		Timeout:  time.Duration(cs.Timeout) * time.Second,
		Interval: time.Duration(cs.Interval) * time.Second,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			failureRatio := float64(counts.TotalFailures) / float64(counts.Requests)
			return failureRatio >= cs.FailureRatio
		},
	}
}

type RateLimiterSettings struct {
	Enabled         bool `yaml:"enabled"`
	Rate            int  `yaml:"rate"`
	Burst           int  `yaml:"burst"`
	CleanupInterval int  `yaml:"cleanupInterval"`
}

type CacheSettings struct {
	Enabled            bool `yaml:"enabled"`
	ExpirationInterval uint `yaml:"expirationInterval"`
	CleanupInterval    uint `yaml:"cleanupInterval"`
}

type AuthSettings struct {
	Enabled bool `yaml:"enabled"`
	// Give the option to make requests with no/expired token to pas through
	Anonymous bool `yaml:"anonymous"`
	// path to the secret file
	Secret string `yaml:"secret"`
	// list of routes that require authentication
	Routes []string `yaml:"routes"`
}

type HealthCheckSettings struct {
	Enabled bool `yaml:"enabled"`
	// path to the health check endpoint
	Uri string `yaml:"uri"`
}

type ServiceConf struct {
	Name      string   `yaml:"name" validate:"required"`
	Addr      string   `yaml:"addr" validate:"required"`
	WhiteList []string `yaml:"whitelist" validate:"required"`
	// uri to redirect to if the service is down
	FallbackUri    string              `yaml:"fallbackUri"`
	Health         HealthCheckSettings `yaml:"health" validate:"required"`
	Auth           AuthSettings        `yaml:"auth"`
	Cache          CacheSettings       `yaml:"cache"`
	CircuitBreaker CircuitSettings     `yaml:"circuitBreaker"`
	RateLimiter    RateLimiterSettings `yaml:"rateLimiter"`
}

type TLSConfig struct {
	Enabled bool `yaml:"enabled"`
	// path to the certificate and key files
	CertFile string `yaml:"certFile"`
	KeyFile  string `yaml:"keyFile"`
}

type Metrics struct {
	Prefix  string    `yaml:"prefix"`
	Buckets []float64 `yaml:"buckets"`
}

type Conf struct {
	Admin struct {
		Port            string    `yaml:"port"`
		ReadTimeout     int       `yaml:"readTimeout"`
		WriteTimeout    int       `yaml:"writeTimeout"`
		GracefulTimeout int       `yaml:"gracefulTimeout"`
		TLS             TLSConfig `yaml:"tls"`
	} `yaml:"admin"`
	Server struct {
		Host      string `yaml:"host"`
		Port      string `yaml:"port"`
		AdminPort string `yaml:"adminPort"`
		// the maximum duration for reading the entire request, including the body
		ReadTimeout int `yaml:"readTimeout"`
		// the maximum duration before timing out writes of the response
		WriteTimeout int `yaml:"writeTimeout"`
		// the maximum duration before timing out the graceful shutdown
		GracefulTimeout int `yaml:"gracefulTimeout"`

		TLS     TLSConfig `yaml:"tls"`
		Metrics Metrics   `yaml:"metrics"`
	}

	Registry struct {
		// Interval (secs) at which the service will send a heartbeat to all registered services
		HeartbeatInterval int `yaml:"heartbeatInterval"`
		Services          map[string]ServiceConf
	}
}

// Marshal returns the configuration as a json byte array
func (c *Conf) Marshal() []byte {
	out, err := json.Marshal(c)
	if err != nil {
		return []byte{}
	}
	return out
}

// Load loads the configuration from the config.yaml file
func Load(configFile string) (*Conf, error) {
	if configFile != "" {
		viper.SetConfigFile(configFile)
	} else {
		viper.SetConfigName("config")
		viper.SetConfigType("yaml")
		viper.AddConfigPath("./config")
	}
	if err := viper.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("config file not found: %w", err)
	}
	var config Conf
	if err := viper.Unmarshal(&config); err != nil {
		return nil, err
	}

	return &config, nil
}

func GetCertFile() string {
	// Append path to root folder
	certPath := filepath.Join(GetWd(), AppConfig.Server.TLS.CertFile)
	return certPath
}

func GetKeyFile() string {
	certPath := filepath.Join(GetWd(), AppConfig.Server.TLS.KeyFile)
	return certPath
}

func TLSEnabled() bool {
	return AppConfig.Server.TLS.Enabled
}

func GetWd() string {
	wd, err := os.Getwd()
	if err != nil {
		slog.Error("Unable to get current working directory", "error", err.Error())
		os.Exit(1)
	}
	return wd
}

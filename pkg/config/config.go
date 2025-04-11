package config

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/sony/gobreaker/v2"

	"gopkg.in/yaml.v3"
)

// AppConfig is the global configuration object
var AppConfig Conf
var Validate *validator.Validate

func init() {
	Validate = validator.New(validator.WithRequiredStructEnabled())
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
	Server struct {
		Host string `yaml:"host"`
		Port string `yaml:"port"`
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

// GetConfMarshal returns the configuration as a json byte array
func (c *Conf) GetConfMarshal() []byte {
	out, err := json.Marshal(c)
	if err != nil {
		return []byte{}
	}
	return out
}

// Verify checks if the configuration is valid
func (c *Conf) Verify() bool {
	if c.Server.Host == "" || c.Server.Port == "" {
		return false
	}
	if c.Server.ReadTimeout == 0 {
		c.Server.ReadTimeout = 5
	}
	if c.Server.WriteTimeout == 0 {
		c.Server.WriteTimeout = 10
	}
	if c.Registry.HeartbeatInterval == 0 {
		c.Registry.HeartbeatInterval = 30
	}
	return true
}

// LoadConf loads the configuration from the config.yaml file
func LoadConf() {
	c := Conf{}
	yamlFile, err := os.ReadFile("./config/config.yaml")
	if err != nil {
		slog.Error("yamlFile.Get err", "error", err.Error())
	}
	err = yaml.Unmarshal(yamlFile, &c)
	if err != nil {
		slog.Error("yaml unmarshal error ocurred", "error", err.Error())
		os.Exit(1)
	}
	if !c.Verify() {
		slog.Error("Config verification failed")
		os.Exit(1)
	}
	AppConfig = c
	slog.Info("Config loaded successfully")
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

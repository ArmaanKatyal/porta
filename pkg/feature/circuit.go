package feature

import (
	"log/slog"

	"github.com/ArmaanKatyal/go-api-gateway/pkg/config"
	"github.com/sony/gobreaker/v2"
)

type CircuitBreaker struct {
	Settings config.CircuitSettings `json:"settings"`
	breaker  *gobreaker.CircuitBreaker[[]byte]
}

func NewCircuitBreaker(svcName string, settings config.CircuitSettings) *CircuitBreaker {
	return &CircuitBreaker{
		Settings: settings,
		breaker:  gobreaker.NewCircuitBreaker[[]byte](settings.Into(svcName)),
	}
}

func (cb *CircuitBreaker) Execute(service string, f func() ([]byte, error)) ([]byte, error) {
	slog.Info("Forwarding request using circuit breaker", "service", service, "breaker", cb.breaker.Name)
	return cb.breaker.Execute(f)
}

func (cb *CircuitBreaker) IsOpen() bool {
	return cb.breaker.State() == gobreaker.StateOpen
}

func (cb *CircuitBreaker) IsEnabled() bool {
	return cb.Settings.Enabled
}

package feature

import (
	"net"
	"sync"
	"time"

	"github.com/ArmaanKatyal/go-api-gateway/pkg/config"
	"golang.org/x/time/rate"
)

type Visitor struct {
	Limiter  *rate.Limiter
	LastSeen time.Time
}

type ServiceRateLimiter struct {
	Enabled  bool
	mu       sync.Mutex
	visitors map[string]*Visitor
	Rate     rate.Limit
	Burst    int
	Cleanup  time.Duration
}

// CleanupVisitors periodically cleans up stale visitors using a ticker.
func (srl *ServiceRateLimiter) CleanupVisitors() {
	ticker := time.NewTicker(srl.Cleanup)
	defer ticker.Stop()
	for range ticker.C {
		srl.mu.Lock()
		for ip, v := range srl.visitors {
			if time.Since(v.LastSeen) > srl.Cleanup {
				delete(srl.visitors, ip)
			}
		}
		srl.mu.Unlock()
	}
}

func (srl *ServiceRateLimiter) IsEnabled() bool {
	return srl.Enabled
}

func (srl *ServiceRateLimiter) Allow(visitorIp string) bool {
	// Attempt to extract IP; fallback to the raw string if needed.
	ip, _, err := net.SplitHostPort(visitorIp)
	if err != nil {
		ip = visitorIp
	}

	srl.mu.Lock()
	defer srl.mu.Unlock()

	visitor, found := srl.visitors[ip]
	if !found {
		visitor = &Visitor{
			Limiter:  rate.NewLimiter(srl.Rate, srl.Burst),
			LastSeen: time.Now(),
		}
		srl.visitors[ip] = visitor
	}

	visitor.LastSeen = time.Now()
	return visitor.Limiter.Allow()
}

func NewServiceRateLimiter(conf *config.RateLimiterSettings) *ServiceRateLimiter {
	srl := &ServiceRateLimiter{
		Enabled:  conf.Enabled,
		visitors: make(map[string]*Visitor),
		Rate:     rate.Limit(conf.Rate),
		Burst:    conf.Burst,
		Cleanup:  time.Duration(conf.CleanupInterval) * time.Second,
	}
	go srl.CleanupVisitors()
	return srl
}

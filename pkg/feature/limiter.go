package feature

import (
	"log/slog"
	"sync"
	"time"

	"github.com/ArmaanKatyal/go-api-gateway/pkg/config"
	"golang.org/x/time/rate"
)

type LimiterType string

const (
	GlobalLimiter  LimiterType = "Global"
	ServiceLimiter LimiterType = "Service"
)

type Visitor struct {
	Limiter  *rate.Limiter
	LastSeen time.Time
}

type BaseRateLimiter struct {
	limitertype LimiterType
	Enabled     bool
	mu          sync.Mutex
	visitors    map[string]*Visitor
	Rate        rate.Limit
	Burst       int
	Cleanup     int
}

// CleanupVisitors periodically cleans up visitors which inturn reset the limits
func (rl *BaseRateLimiter) CleanupVisitors() {
	for {
		time.Sleep(time.Minute)
		rl.mu.Lock()
		switch rl.limitertype {
		case GlobalLimiter:
			slog.Info("cleaning up global visitors")
		case ServiceLimiter:
			slog.Info("cleaning up service visitors")
		}
		for ip, v := range rl.visitors {
			if time.Since(v.LastSeen) > time.Duration(rl.Cleanup)*time.Second {
				delete(rl.visitors, ip)
			}
		}
		rl.mu.Unlock()
	}
}

func (rl *BaseRateLimiter) AddIP(ip string) *Visitor {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	v := &Visitor{
		Limiter:  rate.NewLimiter(rl.Rate, rl.Burst),
		LastSeen: time.Now(),
	}

	rl.visitors[ip] = v
	return v
}

func (rl *BaseRateLimiter) GetVisitor(ip string) *Visitor {
	rl.mu.Lock()
	v, exists := rl.visitors[ip]
	if !exists {
		rl.mu.Unlock()
		return rl.AddIP(ip)
	}
	rl.mu.Unlock()
	return v
}

func (rl *BaseRateLimiter) IsEnabled() bool {
	return rl.Enabled
}

type ServiceRateLimiter struct {
	BaseRateLimiter
}

func NewServiceRateLimiter(conf *config.RateLimiterSettings) *ServiceRateLimiter {
	rl := &ServiceRateLimiter{
		BaseRateLimiter: BaseRateLimiter{
			limitertype: ServiceLimiter,
			Enabled:     conf.Enabled,
			mu:          sync.Mutex{},
			visitors:    make(map[string]*Visitor),
			Rate:        rate.Limit(conf.Rate),
			Burst:       conf.Burst,
			Cleanup:     conf.CleanupInterval,
		},
	}
	go rl.CleanupVisitors()
	return rl
}

type GlobalRateLimiter struct {
	BaseRateLimiter
}

func NewGlobalRateLimiter() *GlobalRateLimiter {
	rl := &GlobalRateLimiter{
		BaseRateLimiter: BaseRateLimiter{
			limitertype: GlobalLimiter,
			Enabled:     config.AppConfig.Server.RateLimiter.Enabled,
			mu:          sync.Mutex{},
			visitors:    make(map[string]*Visitor),
			Rate:        rate.Limit(config.AppConfig.Server.RateLimiter.Rate),
			Burst:       config.AppConfig.Server.RateLimiter.Burst,
			Cleanup:     config.AppConfig.Server.RateLimiter.CleanupInterval,
		},
	}
	go rl.CleanupVisitors()
	return rl
}

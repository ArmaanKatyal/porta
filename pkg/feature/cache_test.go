package feature

import (
	"testing"
	"time"

	"github.com/ArmaanKatyal/porta/pkg/config"
	"github.com/patrickmn/go-cache"
	"github.com/stretchr/testify/assert"
)

func TestCacheHandler(t *testing.T) {
	tests := []struct {
		name     string
		given    config.CacheSettings
		expected config.CacheSettings
	}{
		{
			name:     "default values",
			given:    config.CacheSettings{Enabled: false, ExpirationInterval: 0, CleanupInterval: 0},
			expected: config.CacheSettings{Enabled: false, ExpirationInterval: 5, CleanupInterval: 10},
		},
		{
			name:     "custom values",
			given:    config.CacheSettings{Enabled: true, ExpirationInterval: 10, CleanupInterval: 20},
			expected: config.CacheSettings{Enabled: true, ExpirationInterval: 10, CleanupInterval: 20},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cacheHandler := NewCacheHandler(&tt.given)
			assert.Equal(t, tt.expected.Enabled, cacheHandler.Enabled)
			assert.Equal(t, tt.expected.ExpirationInterval, cacheHandler.ExpirationInterval)
			assert.Equal(t, tt.expected.CleanupInterval, cacheHandler.CleanupInterval)
		})
	}
}

func TestCacheGet(t *testing.T) {
	t.Run("success get value", func(t *testing.T) {
		cacheHandler := NewCacheHandler(&config.CacheSettings{Enabled: true, ExpirationInterval: 5, CleanupInterval: 10})
		cacheHandler.Set("test", "value", DefaultExpiration)
		value, found := cacheHandler.Get("test")
		assert.True(t, found)
		assert.Equal(t, "value", value)
	})
	t.Run("fail get value", func(t *testing.T) {
		cacheHandler := NewCacheHandler(&config.CacheSettings{Enabled: true, ExpirationInterval: 5, CleanupInterval: 10})
		value, found := cacheHandler.Get("test")
		assert.False(t, found)
		assert.Nil(t, value)
	})
	t.Run("override value", func(t *testing.T) {
		cacheHandler := NewCacheHandler(&config.CacheSettings{Enabled: true, ExpirationInterval: 5, CleanupInterval: 10})
		cacheHandler.Set("test", "value", DefaultExpiration)
		cacheHandler.Set("test", "new value", DefaultExpiration)
		value, found := cacheHandler.Get("test")
		assert.True(t, found)
		assert.Equal(t, "new value", value)
	})
	t.Run("expired value", func(t *testing.T) {
		cacheHandler := CacheHandler{
			Enabled:            true,
			ExpirationInterval: 1,
			CleanupInterval:    1,
			cache:              cache.New(time.Duration(1)*time.Millisecond, time.Duration(1)*time.Millisecond),
		}
		cacheHandler.cache.Set("test", "value", time.Duration(1)*time.Millisecond)
		time.Sleep(10 * time.Millisecond)
		value, found := cacheHandler.Get("test")
		assert.False(t, found)
		assert.Nil(t, value)
	})
}

func TestCacheSet(t *testing.T) {
	t.Run("success set value", func(t *testing.T) {
		cacheHandler := NewCacheHandler(&config.CacheSettings{Enabled: true, ExpirationInterval: 5, CleanupInterval: 10})
		cacheHandler.Set("test", "value", DefaultExpiration)
		value, found := cacheHandler.Get("test")
		assert.True(t, found)
		assert.Equal(t, "value", value)
	})
	t.Run("override value", func(t *testing.T) {
		cacheHandler := NewCacheHandler(&config.CacheSettings{Enabled: true, ExpirationInterval: 5, CleanupInterval: 10})
		cacheHandler.Set("test", "value", DefaultExpiration)
		cacheHandler.Set("test", "new value", DefaultExpiration)
		value, found := cacheHandler.Get("test")
		assert.True(t, found)
		assert.Equal(t, "new value", value)
	})
}

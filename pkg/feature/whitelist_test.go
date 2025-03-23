package feature

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewIPWhiteList(t *testing.T) {
	w := NewIPWhiteList()
	assert.IsType(t, w, &IPWhiteList{})
}

func TestPopulateWhiteList(t *testing.T) {
	t.Run("ip list empty", func(t *testing.T) {
		w := NewIPWhiteList()
		PopulateIPWhiteList(w, []string{})
		assert.Len(t, w.Whitelist, 0)
	})
	t.Run("global allow at index 0", func(t *testing.T) {
		w := NewIPWhiteList()
		PopulateIPWhiteList(w, []string{"ALL", "1.1.1.1", "2.2.2.2"})
		assert.Len(t, w.Whitelist, 1)
		assert.True(t, w.Allowed("ALL"))
	})
	t.Run("global allow not at index 0", func(t *testing.T) {
		w := NewIPWhiteList()
		PopulateIPWhiteList(w, []string{"1.1.1.1", "ALL", "2.2.2.2"})
		assert.Len(t, w.Whitelist, 2)
		assert.False(t, w.Allowed("ALL"))
		assert.True(t, w.Allowed("1.1.1.1"))
	})
}

func TestAllowed(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
		setup    func() *IPWhiteList
	}{
		{
			name:     "global allow filter",
			input:    "ALL",
			expected: true,
			setup: func() *IPWhiteList {
				w := NewIPWhiteList()
				w.Whitelist["ALL"] = true
				return w
			},
		},
		{
			name:     "ip doesn't exist",
			input:    "1.1.1.1",
			expected: false,
			setup: func() *IPWhiteList {
				return NewIPWhiteList()
			},
		},
		{
			name:     "ip exists",
			input:    "1.1.1.1",
			expected: true,
			setup: func() *IPWhiteList {
				w := NewIPWhiteList()
				w.Whitelist["1.1.1.1"] = true
				return w
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := tt.setup()
			assert.Equal(t, w.Allowed(tt.input), tt.expected)
		})
	}
}

func TestGetWhiteList(t *testing.T) {
	w := NewIPWhiteList()
	assert.Equal(t, w.GetWhitelist(), w.Whitelist)
}

func TestUpdateWhiteList(t *testing.T) {
	w := NewIPWhiteList()
	w.Whitelist["ALL"] = true
	newList := map[string]bool{
		"ALL": false,
	}
	w.UpdateWhitelist(newList)
	assert.False(t, w.GetWhitelist()["ALL"])
}

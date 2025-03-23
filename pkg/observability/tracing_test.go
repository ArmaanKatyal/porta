package observability

import (
	"testing"

	"github.com/ArmaanKatyal/go-api-gateway/pkg/config"

	"github.com/stretchr/testify/assert"
)

func TestTracingToList(t *testing.T) {
	m := MetricsInput{
		Code:   "test-code",
		Method: "test-method",
		Route:  "test-route",
	}
	assert.Equal(t, []string{"test-code", "test-method", "test-route"}, m.ToList())
}

func TestTracingNewPromMetrics(t *testing.T) {
	t.Run("observability prefix match", func(t *testing.T) {
		config.AppConfig.Server.Metrics.Prefix = "testing"
		p := NewPromMetrics()
		assert.Equal(t, "testing", p.prefix)
	})
}

func TestTracingGetLabels(t *testing.T) {
	assert.Equal(t, []string{"Code", "Method", "Route"}, getLabels())
}

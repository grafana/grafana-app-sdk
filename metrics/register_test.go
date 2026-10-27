package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
)

func counter() *prometheus.CounterVec {
	return prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "informer_live_events_total",
		Help: "help",
	}, []string{"kind"})
}

func TestRegister(t *testing.T) {
	t.Run("a nil registerer leaves the collector unregistered", func(t *testing.T) {
		c := counter()
		assert.Same(t, c, Register(nil, c))
	})

	// Building the same collectors twice on one registry is what a process wiring two components does. The second
	// must keep updating the series the registry actually exports, not a vector nothing scrapes.
	t.Run("the already-registered collector is returned and stays the exported one", func(t *testing.T) {
		reg := prometheus.NewRegistry()
		first := Register(reg, counter())
		second := Register(reg, counter())

		assert.Same(t, first, second)
		second.WithLabelValues("Widget").Inc()
		assert.Equal(t, float64(1), testutil.ToFloat64(first.WithLabelValues("Widget")))
		assert.Equal(t, 1, testutil.CollectAndCount(reg, "informer_live_events_total"))
	})

	t.Run("a descriptor conflict leaves the collector usable and unexported", func(t *testing.T) {
		reg := prometheus.NewRegistry()
		Register(reg, counter())

		conflicting := prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "informer_live_events_total",
			Help: "a different help string is a different descriptor",
		}, []string{"kind"})
		got := Register(reg, conflicting)

		assert.Same(t, conflicting, got)
		got.WithLabelValues("Widget").Inc()
	})
}

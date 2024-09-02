package metrics

import (
	"github.com/go-chi/telemetry"
)

type BwsMetrics struct {
	*telemetry.Scope
}

func (b *BwsMetrics) Counter(metric string, tags map[string]string) {
	b.RecordHit(metric, tags)
}

func (b *BwsMetrics) Gauge(metric string, tags map[string]string, value float64) {
	b.RecordGauge(metric, tags, value)
}

func New() *BwsMetrics {
	return &BwsMetrics{telemetry.NewScope("bws-cache")}
}

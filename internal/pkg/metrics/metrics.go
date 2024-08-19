package metrics

import (
	"github.com/go-chi/telemetry"
)

type BwsMetrics struct {
	*telemetry.Scope
}

func (b *BwsMetrics) Counter(metric string) {
	b.RecordHit(metric, nil)
}

func (b *BwsMetrics) Gauge(metric string, value float64) {
	b.RecordGauge(metric, nil, value)
}

func New() *BwsMetrics {
	return &BwsMetrics{telemetry.NewScope("bws-cache")}
}

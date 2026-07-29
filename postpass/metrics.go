package postpass

import "github.com/prometheus/client_golang/prometheus"

type Metrics struct {
}

func NewMetrics(reg prometheus.Registerer, cfg PostpassConfig) *Metrics {
	m := &Metrics{
	}

	return m
}

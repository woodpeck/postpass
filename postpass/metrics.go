package postpass

import "github.com/prometheus/client_golang/prometheus"

type Metrics struct {
	ReqRecv  prometheus.Counter
	RespSent *prometheus.CounterVec
}

func NewMetrics(reg prometheus.Registerer, cfg PostpassConfig) *Metrics {
	m := &Metrics{
		ReqRecv: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "postpass",
			Name:      "request_recv_count",
			Help:      "Total number of legit received requests",
		}),
		RespSent: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "postpass",
			Name:      "response_sent_count",
			Help:      "Total number of responses sent",
		},
			[]string{"queue_name"},
		),
	}

	reg.MustRegister(m.ReqRecv, m.RespSent)

	// “Initialize” the labels here. This ensures the metric is always giving a 0 for the metric
	for _, name := range []string{"quick", "medium", "slow"} {
		m.RespSent.WithLabelValues(name)
	}

	return m
}

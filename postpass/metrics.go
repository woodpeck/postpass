package postpass

import "github.com/prometheus/client_golang/prometheus"

type Metrics struct {
	ReqRecv  prometheus.Counter
}

func NewMetrics(reg prometheus.Registerer, cfg PostpassConfig) *Metrics {
	m := &Metrics{
		ReqRecv: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "postpass",
			Name:      "request_recv_count",
			Help:      "Total number of legit received requests",
		}),
	}

	reg.MustRegister(m.ReqRecv)


	return m
}

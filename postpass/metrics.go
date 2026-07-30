package postpass

import "github.com/prometheus/client_golang/prometheus"

type Metrics struct {
	ReqRecv  prometheus.Counter
	RespSent *prometheus.CounterVec

	EstCost prometheus.Histogram

	QueryDuration     *prometheus.HistogramVec
	TaskQueueDuration *prometheus.HistogramVec
	TotalDuration     *prometheus.HistogramVec

	EstCostDurationRatio *prometheus.HistogramVec
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
		EstCost: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: "postpass",
			Name:      "est_cost",
			Help:      "Estimated costs",
			Buckets:   cfg.Metrics.EstCostBuckets,
		}),
		QueryDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "postpass",
			Name:      "query_duration_seconds",
			Help:      "How long do queries take to run (excl. time in queue)",
			Buckets:   cfg.Metrics.QueryDurationBuckets,
		},
			[]string{"queue_name"},
		),
		TaskQueueDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "postpass",
			Name:      "task_queue_seconds",
			Help:      "How long do tasks spend in the queue before being processed",
			Buckets:   cfg.Metrics.TaskQueueDurationBuckets,
		},
			[]string{"queue_name"},
		),
		TotalDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "postpass",
			Name:      "task_duration_seconds",
			Help:      "How long does a task task (incl. queue & querying)",
			Buckets:   cfg.Metrics.TotalDurationBuckets,
		},
			[]string{"queue_name"},
		),

		EstCostDurationRatio: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "postpass",
			Name:      "est_cost_query_duration_ratio",
			Help:      "What's the estimated cost:actual query duration ratio",
			Buckets:   cfg.Metrics.EstCostDurationRatioBuckets,
		},
			[]string{"queue_name"},
		),
	}

	reg.MustRegister(m.ReqRecv, m.RespSent, m.EstCost, m.QueryDuration, m.TaskQueueDuration, m.TotalDuration, m.EstCostDurationRatio)

	// “Initialize” the labels here. This ensures the metric is always giving a 0 for the metric
	for _, name := range []string{"quick", "medium", "slow"} {
		m.RespSent.WithLabelValues(name)
		m.QueryDuration.WithLabelValues(name)
		m.TaskQueueDuration.WithLabelValues(name)
		m.TotalDuration.WithLabelValues(name)
		m.EstCostDurationRatio.WithLabelValues(name)
	}

	return m
}

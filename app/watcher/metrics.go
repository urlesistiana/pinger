package watcher

import (
	"github.com/prometheus/client_golang/prometheus"
	_ "github.com/prometheus/client_golang/prometheus"
)

var (
	labels     = []string{"job"}
	latencyVac = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "latency_seconds",
		Help:    "The ping latency in second",
		Buckets: []float64{0.005, 0.01, 0.02, 0.04, 0.06, 0.1, 0.15, 0.2, 0.25, 0.3, 0.35, 0.4, 0.5, 0.75, 1, 2},
	}, labels)

	errorTotalVac = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "error_total",
		Help: "The total number of pings failed",
	}, labels)
)

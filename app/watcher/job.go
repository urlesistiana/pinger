package watcher

import (
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
	"pinger/pkg/mlog"
	"sync"
	"time"
)

type job struct {
	logger     *zap.Logger
	pingFunc   func() (time.Duration, error)
	ob         prometheus.Observer
	errCounter prometheus.Counter

	closeOnce   sync.Once
	closeNotify chan struct{}
}

func newJob(name string, interval time.Duration, pingFunc func() (time.Duration, error)) *job {
	lb := prometheus.Labels{"job": name}
	j := &job{
		logger:      mlog.L().With(zap.String("job", name)),
		pingFunc:    pingFunc,
		ob:          latencyVac.With(lb),
		errCounter:  errorTotalVac.With(lb),
		closeOnce:   sync.Once{},
		closeNotify: make(chan struct{}),
	}
	go j.pingLoop(interval)
	return j
}

func (j *job) close() {
	j.closeOnce.Do(func() {
		close(j.closeNotify)
	})
}

func (j *job) pingLoop(interval time.Duration) {
	const defaultInterval = time.Minute
	if interval <= 0 {
		interval = defaultInterval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			go func() {
				d, err := j.pingFunc()
				if err != nil {
					j.logger.Warn("ping err", zap.Error(err))
					j.errCounter.Inc()
				} else {
					j.logger.Info("ping successfully", zap.Duration("latency", d))
					j.ob.Observe(d.Seconds())
				}
			}()
		case <-j.closeNotify:
			return
		}
	}
}

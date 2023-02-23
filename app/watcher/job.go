package watcher

import (
	"context"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
	"io"
	"pinger/pkg/mlog"
	"sync"
	"time"
)

type Job interface {
	Ping(ctx context.Context) (time.Duration, error)
	io.Closer
}

type jobRunner struct {
	job Job

	logger     *zap.Logger
	latencyOb  prometheus.Observer
	errCounter prometheus.Counter

	closeOnce   sync.Once
	closeNotify chan struct{}
}

func (w *Watcher) newJobRunner(name string, interval, timeout time.Duration, j Job) *jobRunner {
	lb := prometheus.Labels{"job": name}
	jr := &jobRunner{
		job: j,

		logger:      mlog.L().With(zap.String("job", name)),
		latencyOb:   w.latencyVac.With(lb),
		errCounter:  w.errorTotalVac.With(lb),
		closeOnce:   sync.Once{},
		closeNotify: make(chan struct{}),
	}
	go jr.pingLoop(interval, timeout)
	return jr
}

func (j *jobRunner) stop() {
	j.closeOnce.Do(func() {
		close(j.closeNotify)
		_ = j.job.Close()
	})
}

func (j *jobRunner) pingLoop(interval, timeout time.Duration) {
	const (
		defaultInterval = time.Minute
		defaultTimeout  = time.Second * 5
	)
	if interval <= 0 {
		interval = defaultInterval
	}
	if timeout <= 0 {
		timeout = defaultTimeout
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), timeout)
				defer cancel()
				d, err := j.job.Ping(ctx)
				if err != nil {
					j.logger.Warn("ping err", zap.Error(err))
					j.errCounter.Inc()
				} else {
					j.logger.Info("ping successfully", zap.Duration("latency", d))
					j.latencyOb.Observe(d.Seconds())
				}
			}()
		case <-j.closeNotify:
			return
		}
	}
}

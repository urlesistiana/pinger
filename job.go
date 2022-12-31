package main

import (
	"crypto/tls"
	"fmt"
	"github.com/prometheus/client_golang/prometheus"
	"net"
	"sync"
	"time"
)

type job struct {
	name       string
	pingFunc   func() (time.Duration, error)
	ob         prometheus.Observer
	errCounter prometheus.Counter

	closeOnce   sync.Once
	closeNotify chan struct{}
}

func newJob(name string, cfg JobConfig) (*job, error) {
	j := new(job)
	j.name = name
	j.ob = latencyVac.With(prometheus.Labels{"job": name})
	j.errCounter = errorTotalVac.With(prometheus.Labels{"job": name})
	j.closeNotify = make(chan struct{})

	switch cfg.Type {
	case "", "tcp":
		j.pingFunc = newPingFunc(cfg.Addr, false)
	case "tls":
		j.pingFunc = newPingFunc(cfg.Addr, true)
	default:
		return nil, fmt.Errorf("unknown ping type %s", cfg.Type)
	}
	go j.pingLoop(cfg.Interval)
	return j, nil
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
			d, err := j.pingFunc()
			if err != nil {
				logWarn.Printf("job: %s, error: %s", j.name, err)
				j.errCounter.Inc()
			} else {
				logInfo.Printf("job: %s, latency: %.4fs", j.name, d.Seconds())
				j.ob.Observe(d.Seconds())
			}
		case <-j.closeNotify:
			return
		}
	}
}

func newPingFunc(addr string, tlsPing bool) func() (time.Duration, error) {
	return func() (time.Duration, error) {
		tcpAddr, err := net.ResolveTCPAddr("tcp", addr)
		if err != nil {
			return 0, fmt.Errorf("failed to resolve address, %w", err)
		}
		start := time.Now()
		c, err := net.DialTCP("tcp", nil, tcpAddr)
		if err != nil {
			return 0, err
		}
		defer c.Close()
		if tlsPing {
			tc := tls.Client(c, &tls.Config{InsecureSkipVerify: true})
			if err := tc.Handshake(); err != nil {
				return 0, err
			}
			_ = tc.Close()
		}
		return time.Since(start), nil
	}
}

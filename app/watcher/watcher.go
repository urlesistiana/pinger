package watcher

import (
	"bytes"
	"fmt"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
	"net"
	"net/http"
	"net/netip"
	"os"
	"os/signal"
	"pinger/app"
	"pinger/pkg/udp_pinger"
	"sync"

	"pinger/pkg/mlog"
	"syscall"
)

func init() {
	app.AddSubCmd(NewCmd())
}

func NewCmd() *cobra.Command {
	a := new(args)
	cmd := &cobra.Command{
		Use:   "watcher [-c config_file]",
		Args:  cobra.NoArgs,
		Short: "Start a ping watcher.",
		Run: func(cmd *cobra.Command, args []string) {
			run(a)
		},
		DisableFlagsInUseLine: true,
	}
	cmd.Flags().StringVarP(&a.config, "config", "c", "pinger-watcher.yaml", "Path of the config file")
	return cmd
}

type args struct {
	config string
}

var logger = mlog.L()

type Watcher struct {
	pc *udp_pinger.PingConn

	m    sync.Mutex
	jobs map[string]*jobRunner

	// metrics
	latencyVac    *prometheus.HistogramVec
	errorTotalVac *prometheus.CounterVec
}

type WatcherOpts struct {
	Conn           *net.UDPConn // required
	LatencyBuckets []float64    // default is defaultBuckets
}

var (
	defaultBuckets = []float64{0.005, 0.01, 0.02, 0.04, 0.06, 0.1, 0.15, 0.2, 0.25, 0.3, 0.35, 0.4, 0.5, 0.75, 1, 2}
)

func NewWatcher(opts WatcherOpts) *Watcher {
	if opts.Conn == nil {
		panic("watcher: nil udp conn")
	}

	buckets := opts.LatencyBuckets
	if len(buckets) == 0 {
		buckets = defaultBuckets
	}

	labels := []string{"job"}
	return &Watcher{
		pc: udp_pinger.NewPingConn(opts.Conn),

		jobs: make(map[string]*jobRunner),
		latencyVac: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "latency_seconds",
			Help:    "The ping latency in second",
			Buckets: buckets,
		}, labels),
		errorTotalVac: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "error_total",
			Help: "The total number of pings failed",
		}, labels),
	}
}

func (w *Watcher) AddJob(name string, jc JobConfig, updateIfExisted bool) error {
	w.m.Lock()
	defer w.m.Unlock()

	if oldJob, dup := w.jobs[name]; dup {
		if updateIfExisted {
			oldJob.stop()
		} else {
			return fmt.Errorf("failed to start job, duplicated job name %s", name)
		}
	}

	j, err := w.runJob(name, jc)
	if err != nil {
		return fmt.Errorf("failed to start job, %w", err)
	}
	w.jobs[name] = j
	return nil
}

func (w *Watcher) DelJob(name string) bool {
	w.m.Lock()
	defer w.m.Unlock()

	if jr := w.jobs[name]; jr != nil {
		jr.stop()
		delete(w.jobs, name)
		lb := prometheus.Labels{"job": name}
		w.latencyVac.Delete(lb)
		w.errorTotalVac.Delete(lb)
		return true
	}
	return false
}

func (w *Watcher) DelAllJobs() {
	w.m.Lock()
	defer w.m.Unlock()

	for name, jr := range w.jobs {
		jr.stop()
		delete(w.jobs, name)
		lb := prometheus.Labels{"job": name}
		w.latencyVac.Delete(lb)
		w.errorTotalVac.Delete(lb)
	}
}

func (w *Watcher) RegisterTo(r prometheus.Registerer) error {
	if err := r.Register(w.latencyVac); err != nil {
		return err
	}
	if err := r.Register(w.errorTotalVac); err != nil {
		return err
	}
	return nil
}

func (w *Watcher) runJob(name string, jc JobConfig) (*jobRunner, error) {
	addr, err := netip.ParseAddrPort(jc.Addr)
	if err != nil {
		return nil, fmt.Errorf("invalid address, %w", err)
	}
	return w.newJobRunner(name, jc.Interval, jc.Timeout, NewPingJob(w.pc, addr, jc.Auth)), nil
}

func run(a *args) {
	c, err := loadConfig(a.config)
	if err != nil {
		logger.Fatal("failed to load config", zap.Error(err))
	}

	udpConn, err := net.ListenUDP("udp", nil)
	if err != nil {
		logger.Fatal("failed to open socket", zap.Error(err))
	}
	defer udpConn.Close()

	w := NewWatcher(WatcherOpts{
		Conn: udpConn,
	})

	for name, jc := range c.Jobs {
		err := w.AddJob(name, jc, false)
		if err != nil {
			logger.Fatal("failed to start job", zap.String("name", name), zap.Error(err))
		}
	}

	r := prometheus.NewRegistry()
	if err := w.RegisterTo(prometheus.WrapRegistererWithPrefix("pinger_watcher_", r)); err != nil {
		logger.Fatal("failed to register metrics", zap.Error(err))
	}
	s := &http.Server{
		Handler: promhttp.HandlerFor(r, promhttp.HandlerOpts{}),
	}
	l, err := net.Listen("tcp", c.Listen)
	if err != nil {
		logger.Fatal("failed to listen on socket", zap.Error(err))
	}
	defer l.Close()

	go func() {
		logger.Info("starting metrics http server", zap.Stringer("addr", l.Addr()))
		err := s.Serve(l)
		if err != http.ErrServerClosed {
			logger.Fatal("metrics http server exited", zap.Error(err))
		}
	}()

	sc := make(chan os.Signal, 1)
	signal.Notify(sc, os.Interrupt, syscall.SIGTERM)
	sig := <-sc
	logger.Info("watcher is exiting", zap.Stringer("signal", sig))
	w.DelAllJobs()
	_ = s.Close()
}

func loadConfig(s string) (*Config, error) {
	b, err := os.ReadFile(s)
	if err != nil {
		return nil, fmt.Errorf("failed to read config, %w", err)
	}
	c := new(Config)

	decoder := yaml.NewDecoder(bytes.NewReader(b))
	decoder.KnownFields(true)
	if err := decoder.Decode(c); err != nil {
		return nil, fmt.Errorf("failed to decode yaml, %w", err)
	}
	return c, nil
}

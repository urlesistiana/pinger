package watcher

import (
	"bytes"
	"context"
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
	"os/exec"
	"os/signal"
	"pinger/app"
	"strings"

	"pinger/pkg/mlog"
	"pinger/pkg/udp_pinger"
	"syscall"
	"time"
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

type Config struct {
	Listen string               `yaml:"listen"`
	Jobs   map[string]JobConfig `yaml:"jobs"`
}

type JobConfig struct {
	Type     string        `yaml:"type"`
	Addr     string        `yaml:"addr"`
	Auth     string        `yaml:"auth"`
	Exec     string        `yaml:"exec"`
	Args     []string      `yaml:"args"`
	Interval time.Duration `yaml:"interval"`
}

func run(a *args) {
	c, err := loadConfig(a.config)
	if err != nil {
		logger.Fatal("failed to load config", zap.Error(err))
	}

	s := newMetricsServer()
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

	var js []*job
	for name, jc := range c.Jobs {
		j, err := runJob(name, jc)
		if err != nil {
			logger.Fatal("failed to start job", zap.String("job", name), zap.Error(err))
		}
		js = append(js, j)
	}
	logger.Info("watcher is started", zap.Int("started_jobs", len(js)))

	sc := make(chan os.Signal, 1)
	signal.Notify(sc, os.Interrupt, syscall.SIGTERM)
	sig := <-sc
	logger.Info("watcher is exiting", zap.Stringer("signal", sig))
	for _, j := range js {
		j.close()
	}
	_ = s.Close()
}

var pinger = udp_pinger.New()

func runJob(name string, jc JobConfig) (*job, error) {
	addr, err := netip.ParseAddrPort(jc.Addr)
	if err != nil {
		return nil, fmt.Errorf("invalid address, %w", err)
	}

	var pingFunc func() (time.Duration, error)
	switch jc.Type {
	case "tcp":
		pingFunc = newTCPPingFunc(addr)
	case "udp":
		authHeader := udp_pinger.AuthBytes(jc.Auth)
		pingFunc = func() (time.Duration, error) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
			defer cancel()
			return pinger.Ping(ctx, authHeader, addr)
		}
	case "cmd":
		pingFunc = func() (time.Duration, error) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
			defer cancel()
			cmd := exec.CommandContext(ctx, jc.Exec, jc.Args...)
			buf := new(bytes.Buffer)
			cmd.Stdout = buf
			if err := cmd.Run(); err != nil {
				return 0, err
			}
			res := buf.String()
			res = strings.TrimSpace(res)
			d, err := time.ParseDuration(res)
			if err != nil {
				return 0, fmt.Errorf("invalid stdout result [%s], %w", res, err)
			}
			return d, nil
		}
	default:
		return nil, fmt.Errorf("invalid job type %s", jc.Type)
	}
	return newJob(name, jc.Interval, pingFunc), nil
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

func newMetricsServer() *http.Server {
	r := prometheus.NewRegistry()
	prometheus.WrapRegistererWithPrefix("pinger_", r).MustRegister(latencyVac)
	s := &http.Server{
		Handler: promhttp.HandlerFor(r, promhttp.HandlerOpts{}),
	}
	return s
}

func newTCPPingFunc(addr netip.AddrPort) func() (time.Duration, error) {
	return func() (time.Duration, error) {
		start := time.Now()
		c, err := net.DialTCP("tcp", nil, net.TCPAddrFromAddrPort(addr))
		if err != nil {
			return 0, err
		}
		defer c.Close()
		return time.Since(start), nil
	}
}

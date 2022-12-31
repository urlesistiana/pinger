package main

import (
	"bytes"
	"flag"
	"fmt"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"gopkg.in/yaml.v3"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

var config = flag.String("c", "config.yaml", "config file path")

type Config struct {
	Listen string               `yaml:"listen"`
	Jobs   map[string]JobConfig `yaml:"jobs"`
}

type JobConfig struct {
	Type     string        `yaml:"type"`
	Addr     string        `yaml:"addr"`
	Interval time.Duration `yaml:"interval"`
}

func main() {
	flag.Parse()

	c, err := loadConfig()
	if err != nil {
		log.Fatal(err)
	}
	s := newServer()
	l, err := net.Listen("tcp", c.Listen)
	if err != nil {
		log.Fatalf("failed to listen on socket, %s", err)
	}
	defer l.Close()

	go func() {
		logInfo.Printf("starting server at %s", l.Addr())
		err := s.Serve(l)
		if err != http.ErrServerClosed {
			log.Fatalf("http server exited, %s", err)
		}
	}()

	var js []*job
	for s, jobConfig := range c.Jobs {
		j, err := newJob(s, jobConfig)
		if err != nil {
			log.Fatalf("failed to start job %s, %s", s, err)
		}
		js = append(js, j)
	}
	logInfo.Printf("pinger is started with %d job(s)", len(c.Jobs))

	sc := make(chan os.Signal, 1)
	signal.Notify(sc, os.Interrupt, syscall.SIGTERM)
	sig := <-sc
	logInfo.Printf("received signal %s, exiting, bye", sig.String())
	for _, j := range js {
		j.close()
	}
	_ = s.Close()
}

func loadConfig() (*Config, error) {
	b, err := os.ReadFile(*config)
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

func newServer() *http.Server {
	r := prometheus.NewRegistry()
	prometheus.WrapRegistererWithPrefix("pinger_", r).MustRegister(latencyVac)
	s := &http.Server{
		Handler: promhttp.HandlerFor(r, promhttp.HandlerOpts{}),
	}
	return s
}

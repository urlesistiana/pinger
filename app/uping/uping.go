package uping

import (
	"context"
	"fmt"
	"github.com/spf13/cobra"
	"math"
	"net"
	"os"
	"os/signal"
	"pinger/app"
	"pinger/pkg/udp_pinger"
	"sync"
	"syscall"
	"time"
)

func init() {
	app.AddSubCmd(NewCmd())
}

func NewCmd() *cobra.Command {
	a := new(args)
	cmd := &cobra.Command{
		Use:   "uping [-a server_auth] [-n number_of_ping] [-i interval] server_addr:port",
		Args:  cobra.ExactArgs(1),
		Short: "Ping server using UDP.",
		Run: func(cmd *cobra.Command, args []string) {
			a.target = args[0]
			ping(a)
		},
		DisableFlagsInUseLine: true,
	}
	cmd.Flags().StringVarP(&a.auth, "auth", "a", "", "userver auth password")
	cmd.Flags().IntVarP(&a.times, "number", "n", 4, "Number of echo requests to send")
	cmd.Flags().DurationVarP(&a.interval, "interval", "i", time.Second, "Interval of each ping request")
	cmd.Flags().BoolVarP(&a.flood, "flood", "f", false, "flood ping (50 ping/s) until interrupted")
	return cmd
}

type args struct {
	target   string
	auth     string
	flood    bool
	times    int
	interval time.Duration
}

func ping(a *args) {
	ua, err := net.ResolveUDPAddr("udp", a.target)
	if err != nil {
		fmt.Printf("failed to resolve address %s, %s\n", a.target, err)
		return
	}
	addr := ua.AddrPort()

	fmt.Printf("pinging %s\n", ua)
	stats := new(stats)

	mainDone := make(chan struct{})
	go func() {
		sc := make(chan os.Signal, 1)
		signal.Notify(sc, os.Interrupt, syscall.SIGTERM)
		select {
		case <-sc:
		case <-mainDone:
		}
		sent, received, lost, avg := stats.summary()
		lostPercent := (float64(lost) / float64(sent)) * 100
		fmt.Printf(
			"summary: packets: sent = %d, received = %d, lost = %d (%.1f%% loss), avg = %dms",
			sent,
			received,
			lost,
			lostPercent,
			avg.Milliseconds(),
		)
		os.Exit(0)
	}()

	authHeader := udp_pinger.AuthBytes(a.auth)
	p := udp_pinger.New()

	if a.flood {
		a.interval = time.Millisecond * 20
	}
	if a.times <= 0 || a.flood {
		a.times = math.MaxInt
	}
	ticker := time.NewTicker(a.interval)
	wg := new(sync.WaitGroup)
	for i := 0; i < a.times; i++ {
		<-ticker.C
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
			defer cancel()
			stats.sendInc()
			d, err := p.Ping(ctx, authHeader, addr)
			if err != nil {
				if !a.flood {
					fmt.Printf("#%d failure: %s\n", i, err)
				} else {
					print(".")
				}
			} else {
				stats.observe(d)
				if !a.flood {
					fmt.Printf("#%d reply received latency=%.3fms\n", i, float64(d)/1e6)
				}
			}
		}()
	}
	wg.Wait()
	close(mainDone)
	select {}
}

type stats struct {
	m    sync.Mutex
	sent int
	r    []time.Duration
}

func (s *stats) sendInc() {
	s.m.Lock()
	defer s.m.Unlock()
	s.sent++
}

func (s *stats) observe(d time.Duration) {
	s.m.Lock()
	defer s.m.Unlock()
	s.r = append(s.r, d)
}

func (s *stats) summary() (int, int, int, time.Duration) {
	s.m.Lock()
	defer s.m.Unlock()
	var sum time.Duration
	for _, d := range s.r {
		if d >= 0 {
			sum += d
		}
	}
	success := len(s.r)
	failure := s.sent - success
	avg := sum / time.Duration(success)
	return len(s.r), success, failure, avg
}

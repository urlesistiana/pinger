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
		os.Exit(1)
	}
	addr := ua.AddrPort()

	fmt.Printf("pinging %s\n", ua)

	uc, err := net.ListenUDP("udp", nil)
	if err != nil {
		fmt.Printf("failed to open socket, %s\n", err)
		os.Exit(1)
	}
	pc := udp_pinger.NewPingConn(uc)

	ag := udp_pinger.AuthGenerator{Key: udp_pinger.Key32(a.auth)}

	if a.flood {
		a.interval = time.Millisecond * 20
	}
	if a.times <= 0 || a.flood {
		a.times = math.MaxInt
	}
	ticker := time.NewTicker(a.interval)
	pingWg := new(sync.WaitGroup)
	stats := new(stats)
	stopPingLoop := make(chan struct{})
	pingWg.Add(1)
	go func() {
		defer pingWg.Done()
	pingLoop:
		for i := 0; i < a.times; i++ {
			select {
			case <-stopPingLoop:
				break pingLoop
			case <-ticker.C:
				i := i
				pingWg.Add(1)
				go func() {
					defer pingWg.Done()
					ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
					defer cancel()
					stats.sendInc()
					d, err := pc.Ping(ctx, addr, ag.PutAuthHeader)
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
		}
	}()
	pingLoopDone := make(chan struct{})
	go func() {
		pingWg.Wait()
		close(pingLoopDone)
	}()

	sc := make(chan os.Signal, 1)
	signal.Notify(sc, os.Interrupt, syscall.SIGTERM)
	select {
	case sig := <-sc: // interrupted
		close(stopPingLoop) // stop sending
		// Wait more 500ms to receive more replies.
		// Otherwise, some replies will be "lost" if its request was just sent.
		fmt.Printf("received signal: %s, exiting\n", sig)
		wait := time.NewTimer(time.Millisecond * 500)
		select {
		case <-wait.C:
		case <-pingLoopDone:
		}
	case <-pingLoopDone:
	}

	sm := stats.summary()
	var lostPercentage float64
	if sm.sent == 0 {
		lostPercentage = 0
	} else {
		lostPercentage = (float64(sm.sent-sm.received) / float64(sm.sent)) * 100
	}
	fmt.Printf(
		"summary: packets: sent = %d, received = %d, lost = %d (%.1f%% loss)\n",
		sm.sent,
		sm.received,
		sm.sent-sm.received,
		lostPercentage,
	)
	fmt.Printf("rtt: min = %.3fms, max = %.3fms, avg = %.3fms\n",
		float64(sm.min)/1e6,
		float64(sm.max)/1e6,
		float64(sm.avg)/1e6,
	)
	os.Exit(0)
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

type summary struct {
	sent     int
	received int
	min      time.Duration
	max      time.Duration
	avg      time.Duration
}

func (s *stats) summary() summary {
	var sm summary
	s.m.Lock()
	defer s.m.Unlock()

	sm.sent = s.sent
	sm.received = len(s.r)

	var sum time.Duration
	for _, d := range s.r {
		sum += d
		if sm.min == 0 || d < sm.min {
			sm.min = d
		}
		if d > sm.max {
			sm.max = d
		}
	}

	if len(s.r) == 0 {
		sm.avg = 0
	} else {
		sm.avg = sum / time.Duration(len(s.r))
	}
	return sm
}

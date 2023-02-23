package watcher

import (
	"context"
	"net/netip"
	"pinger/pkg/udp_pinger"
	"time"
)

type pingJob struct {
	dst     netip.AddrPort
	pc      *udp_pinger.PingConn
	authGen *udp_pinger.AuthGenerator
}

func NewPingJob(pc *udp_pinger.PingConn, dst netip.AddrPort, key string) Job {
	return &pingJob{
		dst: dst,
		pc:  pc,
		authGen: &udp_pinger.AuthGenerator{
			Key: udp_pinger.Key32(key),
		},
	}
}

func (p *pingJob) Ping(ctx context.Context) (time.Duration, error) {
	return p.pc.Ping(ctx, p.dst, p.authGen.PutAuthHeader)
}

func (p *pingJob) Close() error {
	return nil
}

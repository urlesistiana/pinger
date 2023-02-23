package udp_pinger

import (
	"context"
	"encoding/binary"
	"net"
	"net/netip"
	"sync"
	"time"
)

const (
	headerLen = 48            // 8 bytes epoch + 8 bytes random + 32 bytes hash
	packetLen = headerLen + 4 // header + 4 bytes id
)

type PingConn struct {
	uc *net.UDPConn

	m           sync.Mutex
	closeNotify chan struct{}
	closeErr    error
	pid         uint32
	queue       map[uint32]chan time.Time // Channel must have buffer.
}

// NewPingConn takes the control of uc and will close it automatically.
func NewPingConn(uc *net.UDPConn) *PingConn {
	p := &PingConn{
		uc:    uc,
		queue: make(map[uint32]chan time.Time),
	}
	go p.readLoop()
	return p
}

func (p *PingConn) Ping(ctx context.Context, d netip.AddrPort, putHeader func([]byte) error) (time.Duration, error) {
	rtc := make(chan time.Time, 1) // received time channel
	p.m.Lock()
	p.pid++
	pid := p.pid
	p.queue[pid] = rtc
	p.m.Unlock()
	defer func() {
		p.m.Lock()
		delete(p.queue, pid)
		p.m.Unlock()
	}()

	v := getPacketBuf()
	defer releasePacketBuf(v)
	b := *v

	if err := putHeader(b); err != nil {
		return 0, err
	}
	binary.BigEndian.PutUint32(b[headerLen:], pid)

	_, err := p.uc.WriteToUDPAddrPort(b, d)
	if err != nil {
		return 0, err
	}
	sent := time.Now()
	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	case <-p.closeNotify:
		return 0, p.closeErr
	case rt := <-rtc:
		return rt.Sub(sent), nil
	}
}

// readLoop exits when there is a fatal read error and call closeWithErr
// with that error.
func (p *PingConn) readLoop() {
	buf := make([]byte, 4)
	for {
		n, err := p.uc.Read(buf)
		if err != nil {
			if n != 0 {
				continue // fragment error
			}
			// zero read error is probably a socket error
			p.closeWithErr(err)
			return
		}
		if n != 4 {
			continue // invalid length
		}

		id := binary.BigEndian.Uint32(buf)
		rxTime := time.Now()
		p.m.Lock()
		rtc := p.queue[id]
		p.m.Unlock()
		if rtc != nil {
			select {
			case rtc <- rxTime:
			default:
			}
		}
	}
}

// closeWithErr closes under socket.
// It can cause a read error and stop the readLoop.
func (p *PingConn) closeWithErr(err error) {
	if err == nil {
		err = net.ErrClosed
	}

	p.m.Lock()
	defer p.m.Unlock()

	select {
	case <-p.closeNotify:
		return
	default:
		p.closeErr = err
		close(p.closeNotify)
	}
	_ = p.uc.Close()
}

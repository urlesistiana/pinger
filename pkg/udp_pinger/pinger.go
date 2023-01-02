package udp_pinger

import (
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"net/netip"
	"sync"
	"time"
)

type Pinger struct {
	m      sync.Mutex
	closed bool
	pc     *pingConn
}

func New() *Pinger {
	return &Pinger{}
}

func (p *Pinger) Ping(ctx context.Context, auth [16]byte, d netip.AddrPort) (time.Duration, error) {
	// Get a pingConn or dial a new one.
	var pc *pingConn
	p.m.Lock()
	if p.closed {
		p.m.Unlock()
		return 0, net.ErrClosed
	}
	if p.pc == nil {
		uc, err := net.ListenUDP("udp", nil)
		if err != nil {
			p.m.Unlock()
			return 0, fmt.Errorf("failed to open socket, %w", err)
		}
		p.pc = newPingConn(uc)
	}
	pc = p.pc
	p.m.Unlock()

	rc, cancel, err := pc.sendPing(auth, d)
	if err != nil {
		p.closeSubConn(pc, err)
		return 0, err
	}
	defer cancel()

	sent := time.Now()
	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	case res := <-rc:
		rt, err := res.receivedTime, res.err
		if err != nil {
			p.closeSubConn(pc, err)
			return 0, err
		}
		return rt.Sub(sent), nil
	}
}

func (p *Pinger) closeSubConn(pc *pingConn, withErr error) {
	p.m.Lock()
	defer p.m.Unlock()

	if p.pc == pc { // first call
		pc.closeWithErr(withErr)
		p.pc = nil
	}
	// p.pc has been closed and replaced
}

func (p *Pinger) Close() error {
	p.m.Lock()
	defer p.m.Unlock()
	p.closed = true
	if pc := p.pc; pc != nil {
		p.pc = nil
		pc.closeWithErr(net.ErrClosed)
	}
	return nil
}

type pingConn struct {
	uc     *net.UDPConn
	m      sync.Mutex
	closed bool
	pid    uint64
	queue  map[[24]byte]chan pingRes // 16 bytes auth header + 8 bytes pid. Channel must have buffer.
}

func newPingConn(uc *net.UDPConn) *pingConn {
	p := &pingConn{
		uc:    uc,
		queue: make(map[[24]byte]chan pingRes),
	}
	go p.readLoop()
	return p
}

type pingRes struct {
	receivedTime time.Time
	err          error
}

func (p *pingConn) sendPing(auth [16]byte, d netip.AddrPort) (<-chan pingRes, func(), error) {
	var wb [24]byte
	copy(wb[:], auth[:])
	rc := make(chan pingRes, 1)

	p.m.Lock()
	if p.closed {
		p.m.Unlock()
		return nil, nil, net.ErrClosed
	}
	p.pid++
	binary.BigEndian.PutUint64(wb[16:], p.pid)
	p.queue[wb] = rc
	p.m.Unlock()

	_, err := p.uc.WriteToUDPAddrPort(wb[:], d)
	if err != nil {
		return nil, nil, err
	}
	cancel := func() {
		p.m.Lock()
		delete(p.queue, wb)
		p.m.Unlock()
	}

	return rc, cancel, nil
}

// readLoop returns when there is a read error.
// It always returns an error.
func (p *pingConn) readLoop() {
	buf := make([]byte, 1480)
	for {
		n, err := p.uc.Read(buf)
		if err != nil {
			if n != 0 {
				continue // fragment error
			}
			p.closeWithErr(err)
			return
		}

		b := buf[:n]
		if len(b) != 24 {
			continue // invalid length
		}
		var d [24]byte
		copy(d[:], b)
		p.m.Lock()
		rc := p.queue[d]
		p.m.Unlock()
		if rc != nil {
			select {
			case rc <- pingRes{receivedTime: time.Now()}:
			default:
			}
		}
	}
}

func (p *pingConn) closeWithErr(err error) {
	p.m.Lock()
	p.closed = true
	for _, rc := range p.queue {
		select {
		case rc <- pingRes{err: err}:
		default:
		}
	}
	p.m.Unlock()
	_ = p.uc.Close()
}

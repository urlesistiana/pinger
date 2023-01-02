package udp_pinger

import (
	"bytes"
	"net"
)

// Server starts a ping pinger-server at uc.
// It always returns an error.
func Server(uc *net.UDPConn, auth string) error {
	authHeader := AuthBytes(auth)
	readBuf := make([]byte, 1480)
	for {
		n, addr, err := uc.ReadFromUDPAddrPort(readBuf)
		if err != nil {
			if n != 0 {
				continue // fragment error
			}
			return err
		}
		b := readBuf[:n]

		if len(b) < 16 {
			continue // invalid auth length
		}
		if !bytes.Equal(b[:16], authHeader[:]) {
			continue // invalid auth
		}
		_, _ = uc.WriteToUDPAddrPort(b, addr)
	}
}

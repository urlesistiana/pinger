package udp_pinger

import (
	"net"
)

// Server starts a ping pinger-server at uc.
// It always returns an error.
func Server(uc *net.UDPConn, headerChecker func([]byte) error) error {
	b := make([]byte, packetLen)
	for {
		n, addr, err := uc.ReadFromUDPAddrPort(b)
		if err != nil {
			if n != 0 {
				continue // fragment error
			}
			return err
		}
		if n != packetLen {
			continue // invalid length
		}

		if checkErr := headerChecker(b[:headerLen]); checkErr != nil {
			continue // invalid auth
		}
		_, _ = uc.WriteToUDPAddrPort(b[headerLen:], addr)
	}
}

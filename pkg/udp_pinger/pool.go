package udp_pinger

import (
	"sync"
)

var packetPool = sync.Pool{}

func getPacketBuf() *[]byte {
	v, _ := packetPool.Get().(*[]byte)
	if v == nil {
		b := make([]byte, packetLen)
		v = &b
	}
	return v
}

func releasePacketBuf(b *[]byte) {
	packetPool.Put(b)
}

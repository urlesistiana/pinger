package udp_pinger

import (
	"crypto/md5"
)

const (
	salt = "d677111cd3833f562c31d235df04d035255d0305c264b728fe0"
)

func AuthBytes(passwd string) [16]byte {
	return md5.Sum(append(([]byte)(nil), salt+passwd...))
}

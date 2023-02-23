package udp_pinger

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"golang.org/x/crypto/pbkdf2"
	"math/rand"
	"time"
)

var (
	salt = []byte("d677111cd3833f562c31d235df04d035255d0305c264b728fe0")
)

func Key32(s string) []byte {
	return pbkdf2.Key([]byte(s), salt, 32, 32, sha256.New)
}

type AuthGenerator struct {
	Key    []byte
	Epoch  func() (uint64, error)
	Rand64 func() (uint64, error)
}

func (g *AuthGenerator) PutAuthHeader(b []byte) error {
	epochF := g.Epoch
	if epochF == nil {
		epochF = minuteEpoch
	}
	r64F := g.Rand64
	if r64F == nil {
		r64F = rand64
	}
	epoch, err := epochF()
	if err != nil {
		return err
	}
	r64, err := r64F()
	if err != nil {
		return err
	}

	binary.BigEndian.PutUint64(b[:8], epoch)
	binary.BigEndian.PutUint64(b[8:16], r64)
	n := copy(b[16:48], g.Key) // reuse b as a buffer.
	h := sha256.Sum256(b[:16+n])
	copy(b[16:48], h[:])
	return nil
}

func minuteEpoch() (uint64, error) {
	return uint64(time.Now().Unix() / 60), nil
}

func rand64() (uint64, error) {
	return rand.Uint64(), nil
}

type AuthChecker struct {
	Key         []byte
	Epoch       func() (uint64, error) // default is minuteEpoch
	AllowOffset uint64                 // default is 5
}

var (
	ErrInvalidHash = errors.New("invalid hash")
)

func (c *AuthChecker) CheckHeader(b []byte) error {
	epochF := c.Epoch
	if epochF == nil {
		epochF = minuteEpoch
	}
	epochNow, err := epochF()
	if err != nil {
		return err
	}
	allowOffset := c.AllowOffset
	if allowOffset <= 0 {
		allowOffset = 5
	}

	if l := len(b); l < 48 {
		return fmt.Errorf("header length %d is invalid", l)
	}

	epoch := binary.BigEndian.Uint64(b[:8])

	// check epoch offset
	lb := epochNow - allowOffset
	if lb > epochNow {
		lb = 0
	}
	hb := epochNow + allowOffset
	if hb < epochNow {
		hb = 1<<64 - 1
	}
	if epoch < lb || epoch > hb {
		return fmt.Errorf("invalid epoch %d, want %d ~ %d", epoch, lb, hb)
	}

	// check hash
	h := sha256.New()
	h.Write(b[:16])
	h.Write(c.Key)
	if !bytes.Equal(h.Sum(nil), b[16:48]) {
		return ErrInvalidHash
	}
	return nil
}

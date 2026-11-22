package rfb

import (
	"bytes"
	"crypto/des"
	"encoding/binary"
	"io"
	"math/bits"
	"net"
	"testing"
)

func TestProxyHandshake_RFB38VNCAuthServerSide(t *testing.T) {
	const vncPass = "secret12"
	browserA, browserB := net.Pipe()
	targetA, targetB := net.Pipe()
	defer browserA.Close()
	defer browserB.Close()
	defer targetA.Close()
	defer targetB.Close()

	// Mock remote VNC server requiring SecurityType 2 (VNCAuth)
	targetErr := make(chan error, 1)
	go func() {
		_, _ = targetB.Write([]byte("RFB 003.008\n"))
		var ver [12]byte
		if _, err := io.ReadFull(targetB, ver[:]); err != nil {
			targetErr <- err
			return
		}
		// Offer only SecurityType 2 (VNCAuth)
		_, _ = targetB.Write([]byte{1, secTypeVNCAuth})
		var sel [1]byte
		if _, err := io.ReadFull(targetB, sel[:]); err != nil {
			targetErr <- err
			return
		}
		if sel[0] != secTypeVNCAuth {
			targetErr <- io.ErrUnexpectedEOF
			return
		}

		challenge := []byte("0123456789abcdef")
		_, _ = targetB.Write(challenge)

		var resp [16]byte
		if _, err := io.ReadFull(targetB, resp[:]); err != nil {
			targetErr <- err
			return
		}

		// Verify DES response using bit-reversed key
		var key [8]byte
		copy(key[:], []byte(vncPass))
		for i := range key {
			key[i] = bits.Reverse8(key[i])
		}
		block, _ := des.NewCipher(key[:])
		var want [16]byte
		block.Encrypt(want[0:8], challenge[0:8])
		block.Encrypt(want[8:16], challenge[8:16])

		var result uint32 = 0
		if !bytes.Equal(resp[:], want[:]) {
			result = 1
		}
		_ = binary.Write(targetB, binary.BigEndian, result)
		targetErr <- nil
	}()

	// Mock browser noVNC client expecting SecurityType 1 (None) and NEVER receiving password
	browserErr := make(chan error, 1)
	go func() {
		var ver [12]byte
		if _, err := io.ReadFull(browserB, ver[:]); err != nil {
			browserErr <- err
			return
		}
		_, _ = browserB.Write([]byte("RFB 003.008\n"))
		var sec [2]byte
		if _, err := io.ReadFull(browserB, sec[:]); err != nil {
			browserErr <- err
			return
		}
		if sec[0] != 1 || sec[1] != secTypeNone {
			browserErr <- io.ErrUnexpectedEOF
			return
		}
		_, _ = browserB.Write([]byte{secTypeNone})
		var res uint32
		if err := binary.Read(browserB, binary.BigEndian, &res); err != nil {
			browserErr <- err
			return
		}
		if res != 0 {
			browserErr <- io.ErrUnexpectedEOF
			return
		}
		browserErr <- nil
	}()

	if err := ProxyHandshake(browserA, targetA, vncPass); err != nil {
		t.Fatalf("ProxyHandshake failed: %v", err)
	}
	if err := <-targetErr; err != nil {
		t.Fatalf("target mock error: %v", err)
	}
	if err := <-browserErr; err != nil {
		t.Fatalf("browser mock error: %v", err)
	}
}

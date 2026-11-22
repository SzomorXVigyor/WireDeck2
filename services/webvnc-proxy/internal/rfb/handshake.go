package rfb

import (
	"crypto/des"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math/bits"
	"slices"
	"strings"
)

const (
	rfbVersion38 = "RFB 003.008\n"

	secTypeInvalid = 0
	secTypeNone    = 1
	secTypeVNCAuth = 2
)

// ProxyHandshake terminates the RFB 3.x security handshake server-side (D25):
//   - Authenticates against the remote VNC target using plaintext password (if configured)
//     via RFB DES challenge-response (SecurityType 2) or None (SecurityType 1).
//   - Presents SecurityType 1 (None) to the browser noVNC client so VNC passwords
//     never leave the server or reach the browser.
//
// Once ProxyHandshake returns nil, both streams are aligned right before ClientInit
// and can be bridged with raw full-duplex byte copying.
func ProxyHandshake(browser io.ReadWriter, target io.ReadWriter, password string) error {
	// 1. Read target server's 12-byte RFB version banner
	var srvVerBuf [12]byte
	if _, err := io.ReadFull(target, srvVerBuf[:]); err != nil {
		return fmt.Errorf("read target RFB banner: %w", err)
	}
	srvVer := string(srvVerBuf[:])
	if !strings.HasPrefix(srvVer, "RFB ") || len(srvVer) != 12 {
		return fmt.Errorf("invalid target RFB banner: %q", srvVer)
	}
	minor := parseMinor(srvVer)

	// 2. Negotiate RFB 3.8 with browser (noVNC)
	if _, err := browser.Write([]byte(rfbVersion38)); err != nil {
		return fmt.Errorf("write browser RFB banner: %w", err)
	}
	var cliVerBuf [12]byte
	if _, err := io.ReadFull(browser, cliVerBuf[:]); err != nil {
		return fmt.Errorf("read browser RFB banner: %w", err)
	}

	// 3. Reply to target with supported protocol version (3.8, 3.7, or 3.3)
	targetVer := rfbVersion38
	if minor < 7 {
		targetVer = "RFB 003.003\n"
	} else if minor == 7 {
		targetVer = "RFB 003.007\n"
	}
	if _, err := target.Write([]byte(targetVer)); err != nil {
		WriteBrowserFailure(browser, true, "Failed to negotiate RFB version with VNC host")
		return fmt.Errorf("write target RFB version: %w", err)
	}

	// 4. Authenticate with target VNC server
	if minor < 7 {
		if err := authTargetRFB33(target, password); err != nil {
			WriteBrowserFailure(browser, true, err.Error())
			return err
		}
	} else {
		if err := authTargetRFB37Plus(target, minor, password); err != nil {
			WriteBrowserFailure(browser, true, err.Error())
			return err
		}
	}

	// 5. Complete RFB 3.8 SecurityType=None (1) handshake with browser noVNC
	if _, err := browser.Write([]byte{1, secTypeNone}); err != nil {
		return fmt.Errorf("write browser security types: %w", err)
	}
	var chosen [1]byte
	if _, err := io.ReadFull(browser, chosen[:]); err != nil {
		return fmt.Errorf("read browser security choice: %w", err)
	}
	if chosen[0] != secTypeNone {
		return fmt.Errorf("browser selected unexpected security type %d", chosen[0])
	}
	var okResult [4]byte // 0 = OK
	if _, err := browser.Write(okResult[:]); err != nil {
		return fmt.Errorf("write browser security result: %w", err)
	}

	return nil
}

// WriteBrowserFailure sends a standard RFB 3.8 SecurityType=0 (connection failed)
// frame with a UTF-8 reason string so noVNC surfaces the exact error in securityfailure.
func WriteBrowserFailure(browser io.ReadWriter, bannerAlreadySent bool, reason string) {
	if !bannerAlreadySent {
		if _, err := browser.Write([]byte(rfbVersion38)); err != nil {
			return
		}
		var cliVer [12]byte
		if _, err := io.ReadFull(browser, cliVer[:]); err != nil {
			return
		}
	}
	if len(reason) > 512 {
		reason = reason[:512]
	}
	msg := make([]byte, 1+4+len(reason))
	msg[0] = 0 // 0 security types => failure reason follows
	binary.BigEndian.PutUint32(msg[1:5], uint32(len(reason)))
	copy(msg[5:], reason)
	_, _ = browser.Write(msg)
}

func authTargetRFB33(target io.ReadWriter, password string) error {
	var secType uint32
	if err := binary.Read(target, binary.BigEndian, &secType); err != nil {
		return fmt.Errorf("read RFB 3.3 security type: %w", err)
	}
	switch secType {
	case secTypeInvalid:
		reason := readReason(target)
		return fmt.Errorf("target rejected connection: %s", reason)
	case secTypeNone:
		return nil
	case secTypeVNCAuth:
		if password == "" {
			return errors.New("target requires VNC password, but none is configured on this edge point")
		}
		if err := doVNCChallengeResponse(target, password); err != nil {
			return err
		}
		return readSecurityResult(target, 3)
	default:
		return fmt.Errorf("unsupported RFB 3.3 security type %d", secType)
	}
}

func authTargetRFB37Plus(target io.ReadWriter, minor int, password string) error {
	var countBuf [1]byte
	if _, err := io.ReadFull(target, countBuf[:]); err != nil {
		return fmt.Errorf("read target security type count: %w", err)
	}
	n := int(countBuf[0])
	if n == 0 {
		reason := readReason(target)
		return fmt.Errorf("target refused connection: %s", reason)
	}
	types := make([]byte, n)
	if _, err := io.ReadFull(target, types); err != nil {
		return fmt.Errorf("read target security types: %w", err)
	}

	// Prefer VNCAuth when a password is configured and supported; otherwise None if offered.
	var selected byte
	switch {
	case password != "" && slices.Contains(types, secTypeVNCAuth):
		selected = secTypeVNCAuth
	case slices.Contains(types, secTypeNone):
		selected = secTypeNone
	case slices.Contains(types, secTypeVNCAuth):
		return errors.New("target requires VNC password, but none is configured on this edge point")
	default:
		return fmt.Errorf("target offered no supported RFB security types: %v", types)
	}

	if _, err := target.Write([]byte{selected}); err != nil {
		return fmt.Errorf("send chosen security type: %w", err)
	}

	if selected == secTypeVNCAuth {
		if err := doVNCChallengeResponse(target, password); err != nil {
			return err
		}
		return readSecurityResult(target, minor)
	}

	// In RFB 3.8+, even SecurityType=None sends a 4-byte SecurityResult.
	if minor >= 8 {
		return readSecurityResult(target, minor)
	}
	return nil
}

// doVNCChallengeResponse performs standard RFB VNC Authentication (DES-ECB with bit-reversed key bytes).
func doVNCChallengeResponse(target io.ReadWriter, password string) error {
	var challenge [16]byte
	if _, err := io.ReadFull(target, challenge[:]); err != nil {
		return fmt.Errorf("read VNC auth challenge: %w", err)
	}

	var key [8]byte
	copy(key[:], []byte(password))
	for i := range key {
		key[i] = bits.Reverse8(key[i])
	}

	block, err := des.NewCipher(key[:])
	if err != nil {
		return fmt.Errorf("init DES cipher: %w", err)
	}

	var response [16]byte
	block.Encrypt(response[0:8], challenge[0:8])
	block.Encrypt(response[8:16], challenge[8:16])

	if _, err := target.Write(response[:]); err != nil {
		return fmt.Errorf("write VNC auth response: %w", err)
	}
	return nil
}

func readSecurityResult(r io.Reader, minor int) error {
	var res uint32
	if err := binary.Read(r, binary.BigEndian, &res); err != nil {
		return fmt.Errorf("read VNC security result: %w", err)
	}
	if res == 0 {
		return nil
	}
	if minor >= 8 {
		reason := readReason(r)
		if reason != "" {
			return fmt.Errorf("VNC authentication failed: %s", reason)
		}
	}
	return errors.New("VNC authentication failed (check target password)")
}

func readReason(r io.Reader) string {
	var length uint32
	if err := binary.Read(r, binary.BigEndian, &length); err != nil || length == 0 || length > 4096 {
		return ""
	}
	buf := make([]byte, length)
	if _, err := io.ReadFull(r, buf); err != nil {
		return ""
	}
	return string(buf)
}

func parseMinor(banner string) int {
	// Format: "RFB 003.008\n"
	parts := strings.Split(strings.TrimSpace(strings.TrimPrefix(banner, "RFB ")), ".")
	if len(parts) != 2 {
		return 8
	}
	v := 0
	for _, ch := range parts[1] {
		if ch >= '0' && ch <= '9' {
			v = v*10 + int(ch-'0')
		}
	}
	return v
}

package wireguard

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"strings"

	"golang.org/x/crypto/curve25519"
)

// CatchAllAllowedIPsError is returned when an uploaded config contains 0.0.0.0/0
// or ::/0 and no narrowed LAN subnets were provided yet. The API surfaces this so
// the UI can prompt the admin with the narrow-don't-reject workflow.
type CatchAllAllowedIPsError struct {
	Detected []string
}

func (e *CatchAllAllowedIPsError) Error() string {
	return fmt.Sprintf("AllowedIPs contains catch-all route (%s); specify explicit site LAN subnets to narrow it", strings.Join(e.Detected, ", "))
}

// Meta holds the non-secret metadata derived from a normalized WireGuard client config.
type Meta struct {
	PublicKey           string   `json:"publicKey"`
	ClientPublicKey     string   `json:"clientPublicKey"`
	EndpointHost        string   `json:"endpointHost"`
	EndpointPort        int      `json:"endpointPort"`
	LanSubnets          []string `json:"lanSubnets"`
	ConfFingerprint     string   `json:"confFingerprint"`
	MTU                 int      `json:"mtu,omitempty"`
	PersistentKeepalive int      `json:"persistentKeepalive"`
}

// ParseResult contains the normalized config text and extracted non-secret metadata.
type ParseResult struct {
	NormalizedConf string
	Meta           Meta
	Warnings       []string
}

// Resolver abstracts DNS lookup for unit testing.
type Resolver interface {
	LookupIP(ctx context.Context, network, host string) ([]net.IP, error)
}

// Options configures WireGuard config validation and normalization.
type Options struct {
	ReservedCIDRs      []netip.Prefix
	OverrideAllowedIPs []string
	Resolver           Resolver
	SkipDNSCheck       bool
}

// ParseReservedCIDRs parses a comma-separated list of CIDRs from RESERVED_CIDRS.
func ParseReservedCIDRs(csv string) ([]netip.Prefix, error) {
	var out []netip.Prefix
	for _, raw := range strings.Split(csv, ",") {
		s := strings.TrimSpace(raw)
		if s == "" {
			continue
		}
		p, err := netip.ParsePrefix(s)
		if err != nil {
			return nil, fmt.Errorf("invalid reserved CIDR %q: %w", s, err)
		}
		out = append(out, p.Masked())
	}
	return out, nil
}

// ParseAndNormalize validates an uploaded WireGuard client .conf per docs/WIREGUARD-CONF.md,
// strips DNS=, enforces single [Peer], checks CIDR overlaps, injects PersistentKeepalive = 25
// if missing, and computes the canonical SHA-256 fingerprint.
func ParseAndNormalize(ctx context.Context, rawConf string, opts Options) (*ParseResult, error) {
	if strings.TrimSpace(rawConf) == "" {
		return nil, errors.New("wireguard config is empty")
	}

	var (
		inInterface bool
		inPeer      bool
		ifaceCount  int
		peerCount   int

		privateKey  string
		addresses   []string
		mtu         int
		hasMTU      bool
		warnings    []string

		peerPubKey   string
		presharedKey string
		endpoint     string
		allowedIPs   []string
		keepalive    int
		hasKeepalive bool
	)

	scanner := bufio.NewScanner(strings.NewReader(rawConf))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		// Strip inline comments
		if idx := strings.IndexAny(line, "#;"); idx >= 0 {
			line = strings.TrimSpace(line[:idx])
			if line == "" {
				continue
			}
		}

		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			sec := strings.ToLower(strings.TrimSpace(line[1 : len(line)-1]))
			inInterface = sec == "interface"
			inPeer = sec == "peer"
			switch sec {
			case "interface":
				ifaceCount++
			case "peer":
				peerCount++
			default:
				return nil, fmt.Errorf("unsupported WireGuard section [%s]", sec)
			}
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("malformed config line: %q", line)
		}
		key := strings.ToLower(strings.TrimSpace(parts[0]))
		val := strings.TrimSpace(parts[1])

		if inInterface {
			switch key {
			case "privatekey":
				privateKey = val
			case "address":
				for _, item := range strings.Split(val, ",") {
					if s := strings.TrimSpace(item); s != "" {
						addresses = append(addresses, s)
					}
				}
			case "mtu":
				n, err := strconv.Atoi(val)
				if err != nil {
					return nil, fmt.Errorf("invalid MTU %q", val)
				}
				if n < 1280 || n > 1420 {
					return nil, fmt.Errorf("MTU %d out of valid range (1280-1420)", n)
				}
				mtu = n
				hasMTU = true
			case "dns":
				warnings = append(warnings, "stripped DNS directive (unsupported inside runner containers)")
			case "listenport", "table", "fwmark", "preup", "postup", "predown", "postdown", "saveconfig":
				return nil, fmt.Errorf("directive %q is not permitted in uploaded client configs", key)
			}
		} else if inPeer {
			switch key {
			case "publickey":
				peerPubKey = val
			case "presharedkey":
				presharedKey = val
			case "endpoint":
				endpoint = val
			case "allowedips":
				for _, item := range strings.Split(val, ",") {
					if s := strings.TrimSpace(item); s != "" {
						allowedIPs = append(allowedIPs, s)
					}
				}
			case "persistentkeepalive":
				n, err := strconv.Atoi(val)
				if err != nil || n < 0 || n > 65535 {
					return nil, fmt.Errorf("invalid PersistentKeepalive %q", val)
				}
				keepalive = n
				hasKeepalive = true
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan config: %w", err)
	}

	if ifaceCount != 1 {
		return nil, fmt.Errorf("config must contain exactly one [Interface] section, found %d", ifaceCount)
	}
	if peerCount != 1 {
		return nil, fmt.Errorf("config must contain exactly one [Peer] section, found %d", peerCount)
	}

	privBytes, err := validateWGKey("PrivateKey", privateKey)
	if err != nil {
		return nil, err
	}
	if _, err := validateWGKey("Peer PublicKey", peerPubKey); err != nil {
		return nil, err
	}
	if presharedKey != "" {
		if _, err := validateWGKey("PresharedKey", presharedKey); err != nil {
			return nil, err
		}
	}
	clientPubBytes, err := curve25519.X25519(privBytes, curve25519.Basepoint)
	if err != nil {
		return nil, fmt.Errorf("derive client public key: %w", err)
	}
	clientPubKey := base64.StdEncoding.EncodeToString(clientPubBytes)

	if len(addresses) == 0 {
		return nil, errors.New("[Interface] Address is required")
	}
	var normalizedAddresses []string
	for _, rawAddr := range addresses {
		if !strings.Contains(rawAddr, "/") {
			ip, err := netip.ParseAddr(rawAddr)
			if err != nil {
				return nil, fmt.Errorf("invalid Address %q: %w", rawAddr, err)
			}
			rawAddr = netip.PrefixFrom(ip, ip.BitLen()).String()
		}
		pfx, err := netip.ParsePrefix(rawAddr)
		if err != nil {
			return nil, fmt.Errorf("invalid Address CIDR %q: %w", rawAddr, err)
		}
		for _, res := range opts.ReservedCIDRs {
			if pfx.Overlaps(res) {
				return nil, fmt.Errorf("Address %s overlaps reserved platform CIDR %s", pfx, res)
			}
		}
		normalizedAddresses = append(normalizedAddresses, pfx.String())
	}

	if endpoint == "" {
		return nil, errors.New("[Peer] Endpoint is required")
	}
	epHost, epPortStr, err := net.SplitHostPort(endpoint)
	if err != nil || epHost == "" {
		return nil, fmt.Errorf("invalid Endpoint %q (expected host:port)", endpoint)
	}
	epPort, err := strconv.Atoi(epPortStr)
	if err != nil || epPort < 1 || epPort > 65535 {
		return nil, fmt.Errorf("invalid Endpoint port %q", epPortStr)
	}

	// Resolve Endpoint IPs (to fail at upload and detect AllowedIPs overlap)
	var endpointIPs []netip.Addr
	if ip, err := netip.ParseAddr(epHost); err == nil {
		endpointIPs = append(endpointIPs, ip.Unmap())
	} else if !opts.SkipDNSCheck {
		res := opts.Resolver
		if res == nil {
			res = net.DefaultResolver
		}
		ips, err := res.LookupIP(ctx, "ip4", epHost)
		if err != nil || len(ips) == 0 {
			ips, err = res.LookupIP(ctx, "ip", epHost)
		}
		if err != nil || len(ips) == 0 {
			return nil, fmt.Errorf("endpoint host %q could not be resolved: %v", epHost, err)
		}
		for _, ip := range ips {
			if addr, ok := netip.AddrFromSlice(ip); ok {
				endpointIPs = append(endpointIPs, addr.Unmap())
			}
		}
	}

	// Narrow-don't-reject AllowedIPs flow
	effectiveRawAllowed := allowedIPs
	if len(opts.OverrideAllowedIPs) > 0 {
		effectiveRawAllowed = opts.OverrideAllowedIPs
	} else {
		var catchAll []string
		for _, raw := range allowedIPs {
			pfx, err := netip.ParsePrefix(raw)
			if err == nil && pfx.Bits() == 0 {
				catchAll = append(catchAll, raw)
			}
		}
		if len(catchAll) > 0 {
			return nil, &CatchAllAllowedIPsError{Detected: catchAll}
		}
	}

	if len(effectiveRawAllowed) == 0 {
		return nil, errors.New("[Peer] AllowedIPs must contain at least one site LAN subnet")
	}

	var lanSubnets []string
	seenSubnets := make(map[string]struct{})
	for _, raw := range effectiveRawAllowed {
		s := strings.TrimSpace(raw)
		if s == "" {
			continue
		}
		pfx, err := netip.ParsePrefix(s)
		if err != nil {
			return nil, fmt.Errorf("invalid AllowedIPs CIDR %q: %w", s, err)
		}
		if pfx.Bits() == 0 {
			return nil, &CatchAllAllowedIPsError{Detected: []string{s}}
		}
		masked := pfx.Masked()
		for _, res := range opts.ReservedCIDRs {
			if masked.Overlaps(res) {
				return nil, fmt.Errorf("AllowedIPs %s overlaps reserved platform CIDR %s", masked, res)
			}
		}
		for _, epIP := range endpointIPs {
			if masked.Contains(epIP) {
				return nil, fmt.Errorf("AllowedIPs %s overlaps Endpoint IP %s (would cause routing loop)", masked, epIP)
			}
		}
		canon := masked.String()
		if _, dup := seenSubnets[canon]; !dup {
			seenSubnets[canon] = struct{}{}
			lanSubnets = append(lanSubnets, canon)
		}
	}

	if !hasKeepalive || keepalive == 0 {
		keepalive = 25
		warnings = append(warnings, "injected PersistentKeepalive = 25 to maintain tunnel health checks")
	}

	// Build canonical normalized config
	var b strings.Builder
	b.WriteString("[Interface]\n")
	fmt.Fprintf(&b, "PrivateKey = %s\n", privateKey)
	fmt.Fprintf(&b, "Address = %s\n", strings.Join(normalizedAddresses, ", "))
	if hasMTU {
		fmt.Fprintf(&b, "MTU = %d\n", mtu)
	}
	b.WriteString("\n[Peer]\n")
	fmt.Fprintf(&b, "PublicKey = %s\n", peerPubKey)
	if presharedKey != "" {
		fmt.Fprintf(&b, "PresharedKey = %s\n", presharedKey)
	}
	fmt.Fprintf(&b, "Endpoint = %s\n", net.JoinHostPort(epHost, strconv.Itoa(epPort)))
	fmt.Fprintf(&b, "AllowedIPs = %s\n", strings.Join(lanSubnets, ", "))
	fmt.Fprintf(&b, "PersistentKeepalive = %d\n", keepalive)

	normalized := b.String()
	fp := sha256.Sum256([]byte(normalized))

	return &ParseResult{
		NormalizedConf: normalized,
		Meta: Meta{
			PublicKey:           peerPubKey,
			ClientPublicKey:     clientPubKey,
			EndpointHost:        epHost,
			EndpointPort:        epPort,
			LanSubnets:          lanSubnets,
			ConfFingerprint:     hex.EncodeToString(fp[:]),
			MTU:                 mtu,
			PersistentKeepalive: keepalive,
		},
		Warnings: warnings,
	}, nil
}

// ContainsHost reports whether hostIP falls inside any of the site's lanSubnets.
func ContainsHost(hostIP string, lanSubnets []string) bool {
	addr, err := netip.ParseAddr(strings.TrimSpace(hostIP))
	if err != nil {
		return false
	}
	addr = addr.Unmap()
	for _, s := range lanSubnets {
		pfx, err := netip.ParsePrefix(s)
		if err == nil && pfx.Contains(addr) {
			return true
		}
	}
	return false
}

func validateWGKey(name, b64 string) ([]byte, error) {
	if b64 == "" {
		return nil, fmt.Errorf("%s is required", name)
	}
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil || len(raw) != 32 {
		return nil, fmt.Errorf("%s must be a valid 32-byte base64 WireGuard key", name)
	}
	return raw, nil
}

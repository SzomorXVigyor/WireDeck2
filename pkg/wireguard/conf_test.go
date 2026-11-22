package wireguard

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"net"
	"strings"
	"testing"
)

type stubResolver struct {
	ips map[string][]net.IP
}

func (r stubResolver) LookupIP(_ context.Context, _, host string) ([]net.IP, error) {
	if ips, ok := r.ips[host]; ok {
		return ips, nil
	}
	return nil, errors.New("nxdomain")
}

func sampleKey(b byte) string {
	return base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{b}, 32))
}

func TestParseAndNormalize_CatchAllAndNarrow(t *testing.T) {
	reserved, err := ParseReservedCIDRs("172.30.248.0/24,172.30.252.0/22")
	if err != nil {
		t.Fatal(err)
	}
	res := stubResolver{ips: map[string][]net.IP{"site.example.com": {net.ParseIP("203.0.113.10")}}}

	raw := `
[Interface]
PrivateKey = ` + sampleKey(1) + `
Address = 10.200.0.2/24
DNS = 1.1.1.1

[Peer]
PublicKey = ` + sampleKey(2) + `
Endpoint = site.example.com:51820
AllowedIPs = 0.0.0.0/0, ::/0
`
	// Without override, returns CatchAllAllowedIPsError for UI narrowing wizard
	_, err = ParseAndNormalize(context.Background(), raw, Options{
		ReservedCIDRs: reserved,
		Resolver:      res,
	})
	var catchAll *CatchAllAllowedIPsError
	if !errors.As(err, &catchAll) {
		t.Fatalf("expected CatchAllAllowedIPsError, got %v", err)
	}

	// With overrideAllowedIPs, normalizes cleanly, strips DNS, injects keepalive=25
	parsed, err := ParseAndNormalize(context.Background(), raw, Options{
		ReservedCIDRs:      reserved,
		Resolver:           res,
		OverrideAllowedIPs: []string{"192.168.10.0/24"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(parsed.NormalizedConf, "DNS") {
		t.Fatal("DNS directive must be stripped")
	}
	if parsed.Meta.PersistentKeepalive != 25 {
		t.Fatalf("expected keepalive=25, got %d", parsed.Meta.PersistentKeepalive)
	}
	if len(parsed.Meta.LanSubnets) != 1 || parsed.Meta.LanSubnets[0] != "192.168.10.0/24" {
		t.Fatalf("unexpected lanSubnets: %+v", parsed.Meta.LanSubnets)
	}
	if !ContainsHost("192.168.10.50", parsed.Meta.LanSubnets) {
		t.Fatal("expected 192.168.10.50 inside lanSubnets")
	}
	if ContainsHost("10.0.0.5", parsed.Meta.LanSubnets) {
		t.Fatal("expected 10.0.0.5 outside lanSubnets")
	}
}

func TestParseAndNormalize_RejectsReservedAndEndpointOverlap(t *testing.T) {
	reserved, _ := ParseReservedCIDRs("172.30.248.0/24,172.30.252.0/22")
	res := stubResolver{ips: map[string][]net.IP{"site.example.com": {net.ParseIP("198.51.100.5")}}}

	overlapDocker := `
[Interface]
PrivateKey = ` + sampleKey(1) + `
Address = 10.200.0.2/32
[Peer]
PublicKey = ` + sampleKey(2) + `
Endpoint = site.example.com:51820
AllowedIPs = 172.30.253.0/24
`
	if _, err := ParseAndNormalize(context.Background(), overlapDocker, Options{ReservedCIDRs: reserved, Resolver: res}); err == nil {
		t.Fatal("expected reserved CIDR overlap error")
	}

	overlapEndpoint := `
[Interface]
PrivateKey = ` + sampleKey(1) + `
Address = 10.200.0.2/32
[Peer]
PublicKey = ` + sampleKey(2) + `
Endpoint = site.example.com:51820
AllowedIPs = 198.51.100.0/24
`
	if _, err := ParseAndNormalize(context.Background(), overlapEndpoint, Options{ReservedCIDRs: reserved, Resolver: res}); err == nil {
		t.Fatal("expected endpoint overlap error")
	}
}

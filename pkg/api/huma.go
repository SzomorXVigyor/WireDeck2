package api

import (
	"fmt"
	"net"
	"net/netip"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
)

// NewHumaAPI creates a Huma v2 API backed by chi with strict JSON Schema validation,
// OpenAPI 3.1 metadata, BearerAuth security scheme, and optional Swagger/Scalar UI.
func NewHumaAPI(r chi.Router, title, version, basePath string, enableDocs bool) huma.API {
	cfg := huma.DefaultConfig(title, version)
	cfg.OpenAPIPath = strings.TrimRight(basePath, "/") + "/openapi"
	cfg.SchemasPath = strings.TrimRight(basePath, "/") + "/schemas"
	if enableDocs {
		cfg.DocsPath = strings.TrimRight(basePath, "/") + "/docs"
	} else {
		cfg.DocsPath = ""
	}
	cfg.Components.SecuritySchemes = map[string]*huma.SecurityScheme{
		"BearerAuth": {
			Type:         "http",
			Scheme:       "bearer",
			BearerFormat: "JWT",
			Description:  "RS256 access token issued by core-iam",
		},
	}
	return humachi.New(r, cfg)
}

// ListenOnSubnet resolves the local interface IP inside subnetCIDR and binds addr's port
// strictly on that IP (used by runner-orchestrator so its HTTP API is unreachable from
// wiredeck-runners). If subnetCIDR is empty, falls back to binding addr directly.
func ListenOnSubnet(addr, subnetCIDR string) (net.Listener, error) {
	if strings.TrimSpace(subnetCIDR) == "" {
		return net.Listen("tcp", addr)
	}
	pfx, err := netip.ParsePrefix(strings.TrimSpace(subnetCIDR))
	if err != nil {
		return nil, fmt.Errorf("parse BIND_SUBNET %q: %w", subnetCIDR, err)
	}
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, fmt.Errorf("parse INTERNAL_ADDR %q: %w", addr, err)
	}

	ifaces, err := net.InterfaceAddrs()
	if err != nil {
		return nil, fmt.Errorf("list interface addrs: %w", err)
	}
	for _, a := range ifaces {
		ipNet, ok := a.(*net.IPNet)
		if !ok {
			continue
		}
		nip, ok := netip.AddrFromSlice(ipNet.IP)
		if !ok {
			continue
		}
		nip = nip.Unmap()
		if pfx.Contains(nip) {
			return net.Listen("tcp", net.JoinHostPort(nip.String(), port))
		}
	}
	return nil, fmt.Errorf("no local network interface found inside BIND_SUBNET %s", subnetCIDR)
}

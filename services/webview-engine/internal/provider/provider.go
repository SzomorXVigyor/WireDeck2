package provider

import (
	"context"
	"fmt"
	"sync"

	"github.com/google/uuid"

	"wiredeck/services/webview-engine/internal/models"
)

// ReadResult is returned by a protocol Provider for a single register mapping.
type ReadResult struct {
	MappingID uuid.UUID
	Value     *float64
	Err       error
}

// Provider abstracts protocol-specific device connections and register read/write
// operations (e.g., MODBUS_TCP today, extensible to OPC-UA / MQTT / S7 later).
type Provider interface {
	Protocol() string
	ValidateAttributes(attrs map[string]any) (normalized map[string]any, summary RegisterSummary, err error)
	ReadBatch(ctx context.Context, dev models.Device, mappings []models.RegisterMapping) ([]ReadResult, bool)
	ReadOne(ctx context.Context, dev models.Device, mapping models.RegisterMapping) (float64, error)
	WriteOne(ctx context.Context, dev models.Device, mapping models.RegisterMapping, value float64) error
	DrainSite(siteID uuid.UUID)
	ReapIdle()
	Close()
}

// RegisterSummary holds normalized metadata extracted from protocolAttributes for DB indexing.
type RegisterSummary struct {
	FunctionCode    string
	RegisterAddress int
	RegisterCount   int
	DataType        string
	WordOrder       string
	AccessMode      string
}

// Registry maps Device.Protocol strings to their Provider implementations.
type Registry struct {
	mu        sync.RWMutex
	providers map[string]Provider
}

func NewRegistry(providers ...Provider) *Registry {
	r := &Registry{providers: make(map[string]Provider, len(providers))}
	for _, p := range providers {
		r.providers[p.Protocol()] = p
	}
	return r
}

func (r *Registry) Get(protocol string) (Provider, error) {
	r.mu.RLock()
	p, ok := r.providers[protocol]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unsupported device protocol %q", protocol)
	}
	return p, nil
}

func (r *Registry) DrainSite(siteID uuid.UUID) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, p := range r.providers {
		p.DrainSite(siteID)
	}
}

func (r *Registry) ReapIdle() {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, p := range r.providers {
		p.ReapIdle()
	}
}

func (r *Registry) Close() {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, p := range r.providers {
		p.Close()
	}
}

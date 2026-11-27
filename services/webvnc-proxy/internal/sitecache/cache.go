package sitecache

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"

	"wiredeck/pkg/internalclient"
	"wiredeck/pkg/valkey"
)

type authzEntry struct {
	resp      internalclient.SiteAuthzResponse
	fetchedAt time.Time
}

type Cache struct {
	coreIAM      *internalclient.HTTPClient
	orchestrator *internalclient.HTTPClient
	vk           *valkey.Client

	mu       sync.RWMutex
	states   map[uuid.UUID]internalclient.SiteRuntimeState
	authz    map[string]authzEntry
	authzTTL time.Duration
	onChange func(siteID uuid.UUID, prev, cur internalclient.SiteRuntimeState)
}

func New(coreIAM, orchestrator *internalclient.HTTPClient, onChange func(uuid.UUID, internalclient.SiteRuntimeState, internalclient.SiteRuntimeState)) *Cache {
	return &Cache{
		coreIAM:      coreIAM,
		orchestrator: orchestrator,
		states:       make(map[uuid.UUID]internalclient.SiteRuntimeState),
		authz:        make(map[string]authzEntry),
		authzTTL:     valkey.TTLAuthzCache,
		onChange:     onChange,
	}
}

// SetValkey enables shared Valkey caching with immediate invalidation from core-iam.
func (c *Cache) SetValkey(vk *valkey.Client) {
	c.vk = vk
}

func (c *Cache) SyncRuntimeStates(ctx context.Context) {
	var list []internalclient.SiteRuntimeState
	var got bool

	if c.vk != nil {
		if cached, ok, err := valkey.GetJSON[[]internalclient.SiteRuntimeState](ctx, c.vk, valkey.KeySitesRuntime()); err == nil && ok {
			list = cached
			got = true
		}
	}

	if !got {
		if c.coreIAM == nil || c.coreIAM.BaseURL == "" {
			return
		}
		if _, err := c.coreIAM.DoJSON(ctx, http.MethodGet, "/internal/sites/runtime-state", nil, &list); err != nil {
			slog.Warn("runtime_state_sync_failed", "error", err)
			return
		}
		if c.vk != nil {
			_ = c.vk.SetJSON(ctx, valkey.KeySitesRuntime(), list, valkey.TTLSitesRuntime)
		}
	}

	// Overlay freshest live runner statuses directly from Valkey.
	if c.vk != nil && len(list) > 0 {
		keys := make([]string, len(list))
		for i, st := range list {
			keys[i] = valkey.KeyRunnerRuntime(st.SiteID)
		}
		if rtMap, err := valkey.MGetJSON[valkey.RunnerRuntime](ctx, c.vk, keys); err == nil {
			for i := range list {
				if !list[i].IsActive {
					list[i].Status = "OFFLINE"
					continue
				}
				if rt, ok := rtMap[keys[i]]; ok && rt.Status != "" {
					list[i].Status = rt.Status
				}
			}
		}
	}

	next := make(map[uuid.UUID]internalclient.SiteRuntimeState, len(list))
	for _, st := range list {
		next[st.SiteID] = st
	}

	c.mu.Lock()
	prevMap := c.states
	c.states = next
	c.mu.Unlock()

	if c.onChange != nil {
		for id, cur := range next {
			prev := prevMap[id]
			if prev.IsActive != cur.IsActive || prev.VncEnabled != cur.VncEnabled || prev.Status != cur.Status {
				c.onChange(id, prev, cur)
			}
		}
	}
}

func (c *Cache) GetRuntimeState(siteID uuid.UUID) (internalclient.SiteRuntimeState, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	st, ok := c.states[siteID]
	return st, ok
}

func (c *Cache) CheckAuthz(ctx context.Context, siteID, userID uuid.UUID) (*internalclient.SiteAuthzResponse, error) {
	if c.vk != nil {
		if resp, ok, err := valkey.GetJSON[internalclient.SiteAuthzResponse](ctx, c.vk, valkey.KeyAuthz(siteID, userID)); err == nil && ok {
			return &resp, nil
		}
	} else {
		key := siteID.String() + ":" + userID.String()
		c.mu.RLock()
		if e, ok := c.authz[key]; ok && time.Since(e.fetchedAt) < c.authzTTL {
			resp := e.resp
			c.mu.RUnlock()
			return &resp, nil
		}
		c.mu.RUnlock()
	}

	var resp internalclient.SiteAuthzResponse
	path := fmt.Sprintf("/internal/sites/%s/authz?user_id=%s", siteID.String(), userID.String())
	if _, err := c.coreIAM.DoJSON(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return nil, err
	}

	if c.vk != nil {
		_ = c.vk.SetJSON(ctx, valkey.KeyAuthz(siteID, userID), resp, c.authzTTL)
	} else {
		key := siteID.String() + ":" + userID.String()
		c.mu.Lock()
		c.authz[key] = authzEntry{resp: resp, fetchedAt: time.Now()}
		c.mu.Unlock()
	}
	return &resp, nil
}

func (c *Cache) GetRunnerAuth(ctx context.Context, siteID uuid.UUID) (*internalclient.RunnerAuthResponse, error) {
	var resp internalclient.RunnerAuthResponse
	if _, err := c.orchestrator.DoJSON(ctx, http.MethodGet, "/runners/"+siteID.String()+"/token", nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

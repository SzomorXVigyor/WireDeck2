package sitecache

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/SzomorXVigyor/WireDeck2/pkg/internalclient"
)

type authzEntry struct {
	resp      internalclient.SiteAuthzResponse
	fetchedAt time.Time
}

type userSitesEntry struct {
	resp      internalclient.UserSitesResponse
	fetchedAt time.Time
}

type Cache struct {
	coreIAM      *internalclient.HTTPClient
	orchestrator *internalclient.HTTPClient

	mu        sync.RWMutex
	states    map[uuid.UUID]internalclient.SiteRuntimeState
	authz     map[string]authzEntry
	userSites map[uuid.UUID]userSitesEntry
	authzTTL  time.Duration
	onChange  func(siteID uuid.UUID, prev, cur internalclient.SiteRuntimeState)
}

func New(coreIAM, orchestrator *internalclient.HTTPClient, onChange func(uuid.UUID, internalclient.SiteRuntimeState, internalclient.SiteRuntimeState)) *Cache {
	return &Cache{
		coreIAM:      coreIAM,
		orchestrator: orchestrator,
		states:       make(map[uuid.UUID]internalclient.SiteRuntimeState),
		authz:        make(map[string]authzEntry),
		userSites:    make(map[uuid.UUID]userSitesEntry),
		authzTTL:     30 * time.Second,
		onChange:     onChange,
	}
}

func (c *Cache) SyncRuntimeStates(ctx context.Context) {
	if c.coreIAM == nil || c.coreIAM.BaseURL == "" {
		return
	}
	var list []internalclient.SiteRuntimeState
	if _, err := c.coreIAM.DoJSON(ctx, http.MethodGet, "/internal/sites/runtime-state", nil, &list); err != nil {
		slog.Warn("runtime_state_sync_failed", "error", err)
		return
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
			if prev.IsActive != cur.IsActive || prev.WebviewEnabled != cur.WebviewEnabled || prev.Status != cur.Status {
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

func (c *Cache) SiteName(siteID uuid.UUID) string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if st, ok := c.states[siteID]; ok && st.Name != "" {
		return st.Name
	}
	return siteID.String()[:8]
}

func (c *Cache) GetUserSites(ctx context.Context, userID uuid.UUID) (*internalclient.UserSitesResponse, error) {
	c.mu.RLock()
	if e, ok := c.userSites[userID]; ok && time.Since(e.fetchedAt) < 15*time.Second {
		resp := e.resp
		c.mu.RUnlock()
		return &resp, nil
	}
	c.mu.RUnlock()

	var resp internalclient.UserSitesResponse
	path := fmt.Sprintf("/internal/users/%s/sites", userID.String())
	if _, err := c.coreIAM.DoJSON(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return nil, err
	}

	c.mu.Lock()
	c.userSites[userID] = userSitesEntry{resp: resp, fetchedAt: time.Now()}
	c.mu.Unlock()
	return &resp, nil
}

func (c *Cache) CheckAuthz(ctx context.Context, siteID, userID uuid.UUID) (*internalclient.SiteAuthzResponse, error) {
	key := siteID.String() + ":" + userID.String()
	c.mu.RLock()
	if e, ok := c.authz[key]; ok && time.Since(e.fetchedAt) < c.authzTTL {
		resp := e.resp
		c.mu.RUnlock()
		return &resp, nil
	}
	c.mu.RUnlock()

	var resp internalclient.SiteAuthzResponse
	path := fmt.Sprintf("/internal/sites/%s/authz?user_id=%s", siteID.String(), userID.String())
	if _, err := c.coreIAM.DoJSON(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return nil, err
	}

	c.mu.Lock()
	c.authz[key] = authzEntry{resp: resp, fetchedAt: time.Now()}
	c.mu.Unlock()
	return &resp, nil
}

func (c *Cache) GetRunnerAuth(ctx context.Context, siteID uuid.UUID) (string, string, error) {
	if c.orchestrator == nil || c.orchestrator.BaseURL == "" {
		return "", "", fmt.Errorf("runner-orchestrator URL is not configured")
	}
	var resp internalclient.RunnerAuthResponse
	if _, err := c.orchestrator.DoJSON(ctx, http.MethodGet, "/runners/"+siteID.String()+"/token", nil, &resp); err != nil {
		return "", "", err
	}
	return resp.Address, resp.AuthToken, nil
}

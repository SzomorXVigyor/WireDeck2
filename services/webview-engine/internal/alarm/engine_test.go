package alarm

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"wiredeck/services/webview-engine/internal/models"
)

type memRepo struct {
	mu      sync.Mutex
	rules   []models.AlarmRule
	events  map[uuid.UUID]models.AlarmEvent
	pending map[uuid.UUID]models.AlarmPendingState
}

func newMemRepo(rules ...models.AlarmRule) *memRepo {
	return &memRepo{
		rules:   rules,
		events:  make(map[uuid.UUID]models.AlarmEvent),
		pending: make(map[uuid.UUID]models.AlarmPendingState),
	}
}

func (m *memRepo) LoadActiveRules(context.Context) ([]models.AlarmRule, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]models.AlarmRule(nil), m.rules...), nil
}

func (m *memRepo) LoadActiveEvents(context.Context) ([]models.AlarmEvent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []models.AlarmEvent
	for _, ev := range m.events {
		if ev.State == models.AlarmStateActive {
			out = append(out, ev)
		}
	}
	return out, nil
}

func (m *memRepo) LoadPendingStates(context.Context) ([]models.AlarmPendingState, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []models.AlarmPendingState
	for _, ps := range m.pending {
		out = append(out, ps)
	}
	return out, nil
}

func (m *memRepo) UpsertPendingState(_ context.Context, ps models.AlarmPendingState) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pending[ps.RuleID] = ps
	return nil
}

func (m *memRepo) DeletePendingState(_ context.Context, ruleID uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.pending, ruleID)
	return nil
}

func (m *memRepo) DeleteSitePendingStates(_ context.Context, siteID uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, ps := range m.pending {
		if ps.SiteID == siteID {
			delete(m.pending, id)
		}
	}
	return nil
}

func (m *memRepo) InsertEvent(_ context.Context, evt *models.AlarmEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events[evt.ID] = *evt
	return nil
}

func (m *memRepo) ClearEvent(_ context.Context, eventID uuid.UUID, clearedAt time.Time, clearedVal *float64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if ev, ok := m.events[eventID]; ok {
		ev.State = models.AlarmStateCleared
		ev.ClearedAt = &clearedAt
		ev.ClearedValue = clearedVal
		ev.IsStale = false
		m.events[eventID] = ev
	}
	return nil
}

func (m *memRepo) MarkEventStale(_ context.Context, eventID uuid.UUID, stale bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if ev, ok := m.events[eventID]; ok {
		ev.IsStale = stale
		m.events[eventID] = ev
	}
	return nil
}

func (m *memRepo) MarkSiteEventsStale(_ context.Context, siteID uuid.UUID) ([]models.AlarmEvent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []models.AlarmEvent
	for id, ev := range m.events {
		if ev.SiteID == siteID && ev.State == models.AlarmStateActive && !ev.IsStale {
			ev.IsStale = true
			m.events[id] = ev
			out = append(out, ev)
		}
	}
	return out, nil
}

func (m *memRepo) UpdateEventNotification(_ context.Context, eventID uuid.UUID, notifiedAt *time.Time, failed bool, errMsg string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if ev, ok := m.events[eventID]; ok {
		ev.NotifiedAt = notifiedAt
		ev.NotifyFailed = failed
		ev.NotifyError = errMsg
		m.events[eventID] = ev
	}
	return nil
}

type fakeMailer struct {
	mu   sync.Mutex
	sent []string
}

func (f *fakeMailer) Send(_ context.Context, _ []string, subject, _ string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, subject)
	return nil
}

func ptr(v float64) *float64 { return &v }

func TestAlarmLatchHoldDeadbandAndScaledUnits(t *testing.T) {
	ctx := context.Background()
	siteID := uuid.New()
	mapID := uuid.New()
	ruleID := uuid.New()

	// Register has precision=1 (raw 850 -> 85.0 C). Rule triggers > 80.0 C, hold 10s, deadband 5.0 C (clears <= 75.0 C).
	rule := models.AlarmRule{
		ID:                ruleID,
		SiteID:            siteID,
		RegisterMappingID: mapID,
		Name:              "Boiler Overtemp",
		Condition:         models.AlarmCondGT,
		Threshold:         80.0,
		ClearDeadband:     5.0,
		HoldSeconds:       10,
		Severity:          models.AlarmSeverityCritical,
		NotifyEmails:      []string{"ops@example.com"},
		NotifyOnClear:     true,
		IsActive:          true,
	}
	mapping := models.RegisterMapping{
		ID:     mapID,
		SiteID: siteID,
		Name:   "Boiler Temp",
		ProtocolAttributes: map[string]any{
			"unit":      "C",
			"precision": 1,
		},
	}

	repo := newMemRepo(rule)
	mailer := &fakeMailer{}
	var published []models.AlarmEvent
	eng := NewEngine(repo, mailer, func(uuid.UUID) string { return "Plant A" }, func(_ uuid.UUID, ev models.AlarmEvent) {
		published = append(published, ev)
	})
	if err := eng.Start(ctx); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer eng.Shutdown(ctx)

	t0 := time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)

	// 79.0 C -> NORMAL
	eng.EvaluateSample(ctx, mapping, "GOOD", ptr(790), t0)
	if st := eng.RuleRuntimeStatus(ruleID); st != "NORMAL" {
		t.Fatalf("expected NORMAL, got %s", st)
	}

	// 85.0 C at t0 -> PENDING (hold 10s)
	eng.EvaluateSample(ctx, mapping, "GOOD", ptr(850), t0)
	if st := eng.RuleRuntimeStatus(ruleID); st != "PENDING" {
		t.Fatalf("expected PENDING, got %s", st)
	}

	// 86.0 C at t0+5s -> still PENDING
	eng.EvaluateSample(ctx, mapping, "GOOD", ptr(860), t0.Add(5*time.Second))
	if st := eng.RuleRuntimeStatus(ruleID); st != "PENDING" {
		t.Fatalf("expected PENDING at +5s, got %s", st)
	}

	// 85.5 C at t0+10s -> ACTIVE!
	eng.EvaluateSample(ctx, mapping, "GOOD", ptr(855), t0.Add(10*time.Second))
	if st := eng.RuleRuntimeStatus(ruleID); st != "ACTIVE" {
		t.Fatalf("expected ACTIVE at +10s, got %s", st)
	}
	if len(published) != 1 || published[0].TriggeredValue != 85.5 || published[0].Unit != "C" {
		t.Fatalf("unexpected active event: %+v", published)
	}

	// Still 86.0 C -> latched (no duplicate event)
	eng.EvaluateSample(ctx, mapping, "GOOD", ptr(860), t0.Add(15*time.Second))
	if len(published) != 1 {
		t.Fatalf("expected latch (1 event), got %d", len(published))
	}

	// 78.0 C -> below threshold 80.0 but still inside deadband (must reach <= 75.0 C)
	eng.EvaluateSample(ctx, mapping, "GOOD", ptr(780), t0.Add(20*time.Second))
	if st := eng.RuleRuntimeStatus(ruleID); st != "ACTIVE" {
		t.Fatalf("expected ACTIVE inside deadband, got %s", st)
	}

	// 75.0 C -> CLEARED & re-armed!
	eng.EvaluateSample(ctx, mapping, "GOOD", ptr(750), t0.Add(25*time.Second))
	if st := eng.RuleRuntimeStatus(ruleID); st != "NORMAL" {
		t.Fatalf("expected NORMAL after deadband clear, got %s", st)
	}
	if len(published) != 2 || published[1].State != models.AlarmStateCleared {
		t.Fatalf("expected CLEARED event, got %+v", published)
	}

	eng.Shutdown(ctx)
	mailer.mu.Lock()
	defer mailer.mu.Unlock()
	if len(mailer.sent) != 2 {
		t.Fatalf("expected 2 emails (trigger + clear), got %d: %v", len(mailer.sent), mailer.sent)
	}
}

func TestSiteOfflineFreezeD12AndRestartRehydration(t *testing.T) {
	ctx := context.Background()
	siteID := uuid.New()
	mapID := uuid.New()
	rActive := models.AlarmRule{
		ID: uuid.New(), SiteID: siteID, RegisterMappingID: mapID,
		Name: "Active Rule", Condition: models.AlarmCondGTE, Threshold: 10,
		HoldSeconds: 0, Severity: models.AlarmSeverityWarning, IsActive: true,
	}
	rPending := models.AlarmRule{
		ID: uuid.New(), SiteID: siteID, RegisterMappingID: mapID,
		Name: "Pending Rule", Condition: models.AlarmCondGTE, Threshold: 10,
		HoldSeconds: 30, Severity: models.AlarmSeverityInfo, IsActive: true,
	}
	mapping := models.RegisterMapping{ID: mapID, SiteID: siteID, Name: "Pressure"}
	repo := newMemRepo(rActive, rPending)

	eng := NewEngine(repo, nil, nil, nil)
	if err := eng.Start(ctx); err != nil {
		t.Fatalf("start: %v", err)
	}

	t0 := time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)
	eng.EvaluateSample(ctx, mapping, "GOOD", ptr(12), t0)

	if eng.RuleRuntimeStatus(rActive.ID) != "ACTIVE" {
		t.Fatalf("expected rActive ACTIVE")
	}
	if eng.RuleRuntimeStatus(rPending.ID) != "PENDING" {
		t.Fatalf("expected rPending PENDING")
	}

	// Simulate restart before OFFLINE: new engine rehydrates both active event and pending hold!
	eng2 := NewEngine(repo, nil, nil, nil)
	if err := eng2.Start(ctx); err != nil {
		t.Fatalf("rehydrate start: %v", err)
	}
	defer eng2.Shutdown(ctx)
	if eng2.RuleRuntimeStatus(rActive.ID) != "ACTIVE" || eng2.RuleRuntimeStatus(rPending.ID) != "PENDING" {
		t.Fatalf("rehydration failed: active=%s pending=%s", eng2.RuleRuntimeStatus(rActive.ID), eng2.RuleRuntimeStatus(rPending.ID))
	}

	// Site goes OFFLINE (D12): pending cancelled, active frozen as is_stale=true
	eng2.FreezeSiteOffline(ctx, siteID)
	if eng2.RuleRuntimeStatus(rPending.ID) != "NORMAL" {
		t.Fatalf("expected pending cancelled on offline, got %s", eng2.RuleRuntimeStatus(rPending.ID))
	}
	if eng2.RuleRuntimeStatus(rActive.ID) != "ACTIVE" {
		t.Fatalf("expected active kept frozen on offline, got %s", eng2.RuleRuntimeStatus(rActive.ID))
	}
	events, _ := repo.LoadActiveEvents(ctx)
	if len(events) != 1 || !events[0].IsStale {
		t.Fatalf("expected active event marked stale=true, got %+v", events)
	}
}

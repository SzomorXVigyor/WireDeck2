package alarm

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"

	"wiredeck/pkg/valkey"
	"wiredeck/services/webview-engine/internal/models"
)

// Mailer is satisfied by *mailgopher.Client.
type Mailer interface {
	Send(ctx context.Context, recipients []string, subject, content string) error
}

// Repository abstracts OLTP persistence for the Alarm Engine (enabling pure unit tests).
type Repository interface {
	LoadActiveRules(ctx context.Context) ([]models.AlarmRule, error)
	LoadActiveEvents(ctx context.Context) ([]models.AlarmEvent, error)
	LoadPendingStates(ctx context.Context) ([]models.AlarmPendingState, error)
	UpsertPendingState(ctx context.Context, ps models.AlarmPendingState) error
	DeletePendingState(ctx context.Context, ruleID uuid.UUID) error
	DeleteSitePendingStates(ctx context.Context, siteID uuid.UUID) error
	InsertEvent(ctx context.Context, evt *models.AlarmEvent) error
	UpdateEventMetrics(ctx context.Context, eventID uuid.UUID, lastVal float64, peakDelta float64, stale bool) error
	ClearEvent(ctx context.Context, eventID uuid.UUID, clearedAt time.Time, clearedVal *float64) error
	DeleteSiteActiveEvents(ctx context.Context, siteID uuid.UUID) (int64, error)
	MarkEventStale(ctx context.Context, eventID uuid.UUID, stale bool) error
	MarkSiteEventsStale(ctx context.Context, siteID uuid.UUID) ([]models.AlarmEvent, error)
	UpdateEventNotification(ctx context.Context, eventID uuid.UUID, notifiedAt *time.Time, failed bool, errMsg string) error
}

// BunRepository implements Repository on *bun.DB (durable rules & events) and *valkey.Client (transient pending hold timers).
type BunRepository struct {
	db *bun.DB
	vk *valkey.Client
}

func NewBunRepository(db *bun.DB, vk *valkey.Client) *BunRepository {
	return &BunRepository{db: db, vk: vk}
}

func (r *BunRepository) LoadActiveRules(ctx context.Context) ([]models.AlarmRule, error) {
	var rules []models.AlarmRule
	err := r.db.NewSelect().Model(&rules).Where("is_active = TRUE").Scan(ctx)
	return rules, err
}

func (r *BunRepository) LoadActiveEvents(ctx context.Context) ([]models.AlarmEvent, error) {
	var events []models.AlarmEvent
	err := r.db.NewSelect().Model(&events).Where("state = ?", models.AlarmStateActive).Scan(ctx)
	return events, err
}

func (r *BunRepository) LoadPendingStates(ctx context.Context) ([]models.AlarmPendingState, error) {
	if r.vk == nil {
		return nil, nil
	}
	records, err := valkey.HGetAllJSON[valkey.AlarmPendingRecord](ctx, r.vk, valkey.KeyAlarmPending())
	if err != nil {
		return nil, err
	}
	out := make([]models.AlarmPendingState, 0, len(records))
	for _, rec := range records {
		out = append(out, models.AlarmPendingState{
			RuleID:    rec.RuleID,
			SiteID:    rec.SiteID,
			SinceAt:   rec.SinceAt,
			LastValue: rec.LastValue,
		})
	}
	return out, nil
}

func (r *BunRepository) UpsertPendingState(ctx context.Context, ps models.AlarmPendingState) error {
	if r.vk == nil {
		return nil
	}
	return r.vk.HSetJSON(ctx, valkey.KeyAlarmPending(), ps.RuleID.String(), valkey.AlarmPendingRecord{
		RuleID:    ps.RuleID,
		SiteID:    ps.SiteID,
		SinceAt:   ps.SinceAt,
		LastValue: ps.LastValue,
	}, valkey.TTLAlarmPending)
}

func (r *BunRepository) DeletePendingState(ctx context.Context, ruleID uuid.UUID) error {
	if r.vk == nil {
		return nil
	}
	return r.vk.HDel(ctx, valkey.KeyAlarmPending(), ruleID.String())
}

func (r *BunRepository) DeleteSitePendingStates(ctx context.Context, siteID uuid.UUID) error {
	if r.vk == nil {
		return nil
	}
	records, err := valkey.HGetAllJSON[valkey.AlarmPendingRecord](ctx, r.vk, valkey.KeyAlarmPending())
	if err != nil {
		return err
	}
	var fields []string
	for field, rec := range records {
		if rec.SiteID == siteID {
			fields = append(fields, field)
		}
	}
	if len(fields) == 0 {
		return nil
	}
	return r.vk.HDel(ctx, valkey.KeyAlarmPending(), fields...)
}

func (r *BunRepository) InsertEvent(ctx context.Context, evt *models.AlarmEvent) error {
	_, err := r.db.NewInsert().Model(evt).Exec(ctx)
	return err
}

func (r *BunRepository) UpdateEventMetrics(ctx context.Context, eventID uuid.UUID, lastVal float64, peakDelta float64, stale bool) error {
	_, err := r.db.NewUpdate().Model((*models.AlarmEvent)(nil)).
		Set("last_value = ?", lastVal).
		Set("peak_delta = ?", peakDelta).
		Set("is_stale = ?", stale).
		Where("id = ?", eventID).
		Exec(ctx)
	return err
}

func (r *BunRepository) ClearEvent(ctx context.Context, eventID uuid.UUID, clearedAt time.Time, clearedVal *float64) error {
	q := r.db.NewUpdate().Model((*models.AlarmEvent)(nil)).
		Set("state = ?", models.AlarmStateCleared).
		Set("cleared_at = ?", clearedAt).
		Set("cleared_value = ?", clearedVal).
		Set("is_stale = FALSE")
	if clearedVal != nil {
		q = q.Set("last_value = ?", *clearedVal)
	}
	_, err := q.Where("id = ?", eventID).Exec(ctx)
	return err
}

func (r *BunRepository) DeleteSiteActiveEvents(ctx context.Context, siteID uuid.UUID) (int64, error) {
	res, err := r.db.NewDelete().Model((*models.AlarmEvent)(nil)).
		Where("site_id = ? AND state = ?", siteID, models.AlarmStateActive).
		Exec(ctx)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

func (r *BunRepository) MarkEventStale(ctx context.Context, eventID uuid.UUID, stale bool) error {
	_, err := r.db.NewUpdate().Model((*models.AlarmEvent)(nil)).
		Set("is_stale = ?", stale).
		Where("id = ?", eventID).
		Exec(ctx)
	return err
}

func (r *BunRepository) MarkSiteEventsStale(ctx context.Context, siteID uuid.UUID) ([]models.AlarmEvent, error) {
	var updated []models.AlarmEvent
	_, err := r.db.NewUpdate().Model((*models.AlarmEvent)(nil)).
		Set("is_stale = TRUE").
		Where("site_id = ? AND state = ? AND is_stale = FALSE", siteID, models.AlarmStateActive).
		Returning("*").
		Exec(ctx, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return updated, err
}

func (r *BunRepository) UpdateEventNotification(ctx context.Context, eventID uuid.UUID, notifiedAt *time.Time, failed bool, errMsg string) error {
	_, err := r.db.NewUpdate().Model((*models.AlarmEvent)(nil)).
		Set("notified_at = ?", notifiedAt).
		Set("notify_failed = ?", failed).
		Set("notify_error = ?", errMsg).
		Where("id = ?", eventID).
		Exec(ctx)
	return err
}

// --- Pure Scaling & Evaluation Helpers ---

// ExtractUnitAndPrecision extracts optional unit and precision (-5..5) from protocolAttributes.
func ExtractUnitAndPrecision(attrs map[string]any) (string, int) {
	if attrs == nil {
		return "", 0
	}
	unit := ""
	if u, ok := attrs["unit"].(string); ok {
		unit = strings.TrimSpace(u)
	}
	prec := 0
	switch p := attrs["precision"].(type) {
	case float64:
		prec = int(p)
	case int:
		prec = p
	case int64:
		prec = int(p)
	}
	if prec < -5 {
		prec = -5
	} else if prec > 5 {
		prec = 5
	}
	return unit, prec
}

// ScaleValue converts a raw register reading into scaled engineering units using 10^(-precision).
func ScaleValue(raw float64, precision int) float64 {
	if precision < -5 {
		precision = -5
	} else if precision > 5 {
		precision = 5
	}
	if precision == 0 {
		return raw
	}
	return raw * math.Pow10(-precision)
}

const floatEps = 1e-9

// IsViolated reports whether scaled value v violates the rule condition against threshold.
func IsViolated(cond string, v, threshold float64) bool {
	switch cond {
	case models.AlarmCondGT:
		return v > threshold
	case models.AlarmCondGTE:
		return v >= threshold
	case models.AlarmCondLT:
		return v < threshold
	case models.AlarmCondLTE:
		return v <= threshold
	case models.AlarmCondEQ:
		return math.Abs(v-threshold) <= floatEps
	case models.AlarmCondNEQ:
		return math.Abs(v-threshold) > floatEps
	default:
		return false
	}
}

// IsCleared reports whether scaled value v has returned past the clear deadband to re-arm the latch.
func IsCleared(cond string, v, threshold, deadband float64) bool {
	if deadband < 0 {
		deadband = 0
	}
	switch cond {
	case models.AlarmCondGT:
		return v <= threshold-deadband
	case models.AlarmCondGTE:
		if deadband > 0 {
			return v <= threshold-deadband
		}
		return v < threshold
	case models.AlarmCondLT:
		return v >= threshold+deadband
	case models.AlarmCondLTE:
		if deadband > 0 {
			return v >= threshold+deadband
		}
		return v > threshold
	case models.AlarmCondEQ:
		return math.Abs(v-threshold) > floatEps
	case models.AlarmCondNEQ:
		return math.Abs(v-threshold) <= floatEps
	default:
		return true
	}
}

// ViolationDelta computes the signed delta from threshold when v is on the violating side of the condition.
func ViolationDelta(cond string, v, threshold float64) (float64, bool) {
	switch cond {
	case models.AlarmCondGT, models.AlarmCondGTE:
		if v >= threshold {
			return v - threshold, true
		}
		return 0, false
	case models.AlarmCondLT, models.AlarmCondLTE:
		if v <= threshold {
			return v - threshold, true
		}
		return 0, false
	case models.AlarmCondEQ:
		return 0, true
	case models.AlarmCondNEQ:
		return v - threshold, true
	default:
		return v - threshold, true
	}
}

func ConditionSymbol(cond string) string {
	switch cond {
	case models.AlarmCondGT:
		return ">"
	case models.AlarmCondGTE:
		return ">="
	case models.AlarmCondLT:
		return "<"
	case models.AlarmCondLTE:
		return "<="
	case models.AlarmCondEQ:
		return "=="
	case models.AlarmCondNEQ:
		return "!="
	default:
		return cond
	}
}

func formatVal(v float64, unit string) string {
	s := strconv.FormatFloat(v, 'f', -1, 64)
	if unit != "" {
		return s + " " + unit
	}
	return s
}

type emailJob struct {
	eventID      uuid.UUID
	siteID       uuid.UUID
	recipients   []string
	isClear      bool
	severity     string
	ruleName     string
	registerName string
	condition    string
	threshold    float64
	value        float64
	unit         string
	timestamp    time.Time
}

// Engine evaluates incoming telemetry samples against active alarm rules using a
// persistent latch/re-arm state machine (D10) and site-OFFLINE freeze semantics (D12).
type Engine struct {
	repo       Repository
	mailer     Mailer
	siteNameFn func(siteID uuid.UUID) string
	publishFn  func(siteID uuid.UUID, evt models.AlarmEvent)

	mu             sync.RWMutex
	rulesByID      map[uuid.UUID]models.AlarmRule
	rulesByMapping map[uuid.UUID][]models.AlarmRule
	activeByRule   map[uuid.UUID]models.AlarmEvent
	pendingByRule  map[uuid.UUID]models.AlarmPendingState

	emailCh chan emailJob
	wg      sync.WaitGroup
	started bool
}

func NewEngine(
	repo Repository,
	mailer Mailer,
	siteNameFn func(siteID uuid.UUID) string,
	publishFn func(siteID uuid.UUID, evt models.AlarmEvent),
) *Engine {
	return &Engine{
		repo:           repo,
		mailer:         mailer,
		siteNameFn:     siteNameFn,
		publishFn:      publishFn,
		rulesByID:      make(map[uuid.UUID]models.AlarmRule),
		rulesByMapping: make(map[uuid.UUID][]models.AlarmRule),
		activeByRule:   make(map[uuid.UUID]models.AlarmEvent),
		pendingByRule:  make(map[uuid.UUID]models.AlarmPendingState),
		emailCh:        make(chan emailJob, 128),
	}
}

// Start rehydrates active rules, active latched events, and pending hold timers from OLTP,
// and starts the async MailGopher notification worker.
func (e *Engine) Start(ctx context.Context) error {
	rules, err := e.repo.LoadActiveRules(ctx)
	if err != nil {
		return fmt.Errorf("load active alarm rules: %w", err)
	}
	events, err := e.repo.LoadActiveEvents(ctx)
	if err != nil {
		return fmt.Errorf("load active alarm events: %w", err)
	}
	pending, err := e.repo.LoadPendingStates(ctx)
	if err != nil {
		return fmt.Errorf("load alarm pending states: %w", err)
	}

	e.mu.Lock()
	e.setRulesLocked(rules)
	e.activeByRule = make(map[uuid.UUID]models.AlarmEvent, len(events))
	for _, ev := range events {
		if ev.RuleID != nil {
			e.activeByRule[*ev.RuleID] = ev
		}
	}
	e.pendingByRule = make(map[uuid.UUID]models.AlarmPendingState, len(pending))
	for _, ps := range pending {
		e.pendingByRule[ps.RuleID] = ps
	}
	if !e.started {
		e.started = true
		e.wg.Add(1)
		go e.emailWorker()
	}
	e.mu.Unlock()

	slog.Info("alarm_engine_started", "rules", len(rules), "active_events", len(events), "pending_holds", len(pending))
	return nil
}

// Shutdown drains pending notification emails gracefully.
func (e *Engine) Shutdown(ctx context.Context) {
	e.mu.Lock()
	if !e.started {
		e.mu.Unlock()
		return
	}
	e.started = false
	close(e.emailCh)
	e.mu.Unlock()

	done := make(chan struct{})
	go func() {
		e.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
	}
}

func (e *Engine) ReloadRules(ctx context.Context) error {
	rules, err := e.repo.LoadActiveRules(ctx)
	if err != nil {
		return err
	}
	e.mu.Lock()
	e.setRulesLocked(rules)
	e.mu.Unlock()
	return nil
}

func (e *Engine) setRulesLocked(rules []models.AlarmRule) {
	byID := make(map[uuid.UUID]models.AlarmRule, len(rules))
	byMap := make(map[uuid.UUID][]models.AlarmRule)
	for _, r := range rules {
		byID[r.ID] = r
		byMap[r.RegisterMappingID] = append(byMap[r.RegisterMappingID], r)
	}
	e.rulesByID = byID
	e.rulesByMapping = byMap
}

// RuleRuntimeStatus returns "ACTIVE", "PENDING", or "NORMAL" for UI display.
func (e *Engine) RuleRuntimeStatus(ruleID uuid.UUID) string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if _, ok := e.activeByRule[ruleID]; ok {
		return "ACTIVE"
	}
	if _, ok := e.pendingByRule[ruleID]; ok {
		return "PENDING"
	}
	return "NORMAL"
}

// SyncAckEvent keeps the in-memory active event cache consistent after a user acknowledges it.
func (e *Engine) SyncAckEvent(evt models.AlarmEvent) {
	if evt.RuleID == nil || evt.State != models.AlarmStateActive {
		return
	}
	e.mu.Lock()
	if cur, ok := e.activeByRule[*evt.RuleID]; ok && cur.ID == evt.ID {
		e.activeByRule[*evt.RuleID] = evt
	}
	e.mu.Unlock()
}

// EvaluateSample processes a single polled register sample against all active alarm rules for that mapping.
func (e *Engine) EvaluateSample(ctx context.Context, mapping models.RegisterMapping, quality string, rawVal *float64, ts time.Time) {
	e.mu.RLock()
	rules := append([]models.AlarmRule(nil), e.rulesByMapping[mapping.ID]...)
	e.mu.RUnlock()

	if len(rules) == 0 {
		return
	}

	// Non-GOOD sample: cancel pending holds and mark any active latched events stale (D12).
	if quality != "GOOD" || rawVal == nil {
		for _, rule := range rules {
			e.handleBadSampleForRule(ctx, rule)
		}
		return
	}

	unit, prec := ExtractUnitAndPrecision(mapping.ProtocolAttributes)
	scaledVal := ScaleValue(*rawVal, prec)

	for _, rule := range rules {
		e.evaluateRuleSample(ctx, rule, mapping.Name, unit, scaledVal, ts.UTC())
	}
}

func (e *Engine) handleBadSampleForRule(ctx context.Context, rule models.AlarmRule) {
	e.mu.Lock()
	_, hasPending := e.pendingByRule[rule.ID]
	if hasPending {
		delete(e.pendingByRule, rule.ID)
	}
	activeEvt, hasActive := e.activeByRule[rule.ID]
	needMarkStale := hasActive && !activeEvt.IsStale
	if needMarkStale {
		activeEvt.IsStale = true
		e.activeByRule[rule.ID] = activeEvt
	}
	e.mu.Unlock()

	if hasPending {
		if err := e.repo.DeletePendingState(ctx, rule.ID); err != nil {
			slog.Warn("alarm_delete_pending_failed", "rule_id", rule.ID, "error", err)
		}
	}
	if needMarkStale {
		if err := e.repo.MarkEventStale(ctx, activeEvt.ID, true); err != nil {
			slog.Warn("alarm_mark_stale_failed", "event_id", activeEvt.ID, "error", err)
		}
		e.emit(activeEvt.SiteID, activeEvt)
	}
}

func (e *Engine) evaluateRuleSample(ctx context.Context, rule models.AlarmRule, regName, unit string, val float64, ts time.Time) {
	e.mu.Lock()
	activeEvt, isActive := e.activeByRule[rule.ID]
	pending, isPending := e.pendingByRule[rule.ID]
	e.mu.Unlock()

	if isActive {
		if IsCleared(rule.Condition, val, rule.Threshold, rule.ClearDeadband) {
			clearedVal := val
			if err := e.repo.ClearEvent(ctx, activeEvt.ID, ts, &clearedVal); err != nil {
				slog.Warn("alarm_clear_event_failed", "event_id", activeEvt.ID, "error", err)
				return
			}
			activeEvt.State = models.AlarmStateCleared
			activeEvt.ClearedAt = &ts
			activeEvt.ClearedValue = &clearedVal
			activeEvt.LastValue = &clearedVal
			activeEvt.IsStale = false

			e.mu.Lock()
			delete(e.activeByRule, rule.ID)
			e.mu.Unlock()

			e.emit(activeEvt.SiteID, activeEvt)
			if rule.NotifyOnClear && len(rule.NotifyEmails) > 0 {
				e.enqueueEmail(emailJob{
					eventID:      activeEvt.ID,
					siteID:       rule.SiteID,
					recipients:   append([]string(nil), rule.NotifyEmails...),
					isClear:      true,
					severity:     rule.Severity,
					ruleName:     rule.Name,
					registerName: regName,
					condition:    rule.Condition,
					threshold:    rule.Threshold,
					value:        val,
					unit:         unit,
					timestamp:    ts,
				})
			}
			return
		}

		// Still active and violating (or inside deadband): update current value, peak delta, and un-stale.
		curLast := activeEvt.TriggeredValue
		if activeEvt.LastValue != nil {
			curLast = *activeEvt.LastValue
		}
		curPeak := activeEvt.TriggeredValue - rule.Threshold
		if activeEvt.PeakDelta != nil {
			curPeak = *activeEvt.PeakDelta
		}
		nextPeak := curPeak
		if d, ok := ViolationDelta(rule.Condition, val, rule.Threshold); ok && math.Abs(d) > math.Abs(curPeak) {
			nextPeak = d
		}
		valChanged := activeEvt.LastValue == nil || math.Abs(val-curLast) > floatEps
		peakChanged := activeEvt.PeakDelta == nil || math.Abs(nextPeak-curPeak) > floatEps
		wasStale := activeEvt.IsStale
		if wasStale || valChanged || peakChanged {
			if err := e.repo.UpdateEventMetrics(ctx, activeEvt.ID, val, nextPeak, false); err != nil {
				slog.Warn("alarm_update_metrics_failed", "event_id", activeEvt.ID, "error", err)
				return
			}
			vCopy := val
			pCopy := nextPeak
			activeEvt.LastValue = &vCopy
			activeEvt.PeakDelta = &pCopy
			activeEvt.IsStale = false
			e.mu.Lock()
			e.activeByRule[rule.ID] = activeEvt
			e.mu.Unlock()
			if wasStale {
				e.emit(activeEvt.SiteID, activeEvt)
			}
		}
		return
	}

	// Currently NORMAL or PENDING
	if !IsViolated(rule.Condition, val, rule.Threshold) {
		if isPending {
			e.mu.Lock()
			delete(e.pendingByRule, rule.ID)
			e.mu.Unlock()
			if err := e.repo.DeletePendingState(ctx, rule.ID); err != nil {
				slog.Warn("alarm_delete_pending_failed", "rule_id", rule.ID, "error", err)
			}
		}
		return
	}

	// Violated! Check hold time.
	if rule.HoldSeconds > 0 {
		if !isPending {
			ps := models.AlarmPendingState{
				RuleID:    rule.ID,
				SiteID:    rule.SiteID,
				SinceAt:   ts,
				LastValue: val,
			}
			if err := e.repo.UpsertPendingState(ctx, ps); err != nil {
				slog.Warn("alarm_upsert_pending_failed", "rule_id", rule.ID, "error", err)
				return
			}
			e.mu.Lock()
			e.pendingByRule[rule.ID] = ps
			e.mu.Unlock()
			return
		}
		if ts.Sub(pending.SinceAt) < time.Duration(rule.HoldSeconds)*time.Second {
			// Hold timer still running
			pending.LastValue = val
			e.mu.Lock()
			e.pendingByRule[rule.ID] = pending
			e.mu.Unlock()
			return
		}
	}

	// Latch ACTIVE!
	ruleID := rule.ID
	mapID := rule.RegisterMappingID
	initLast := val
	initPeak := val - rule.Threshold
	evt := models.AlarmEvent{
		ID:                uuid.New(),
		SiteID:            rule.SiteID,
		RuleID:            &ruleID,
		RegisterMappingID: &mapID,
		RuleName:          rule.Name,
		RegisterName:      regName,
		Condition:         rule.Condition,
		Threshold:         rule.Threshold,
		Unit:              unit,
		Severity:          rule.Severity,
		State:             models.AlarmStateActive,
		IsStale:           false,
		TriggeredAt:       ts,
		TriggeredValue:    val,
		LastValue:         &initLast,
		PeakDelta:         &initPeak,
	}
	if err := e.repo.InsertEvent(ctx, &evt); err != nil {
		slog.Warn("alarm_insert_event_failed", "rule_id", rule.ID, "error", err)
		return
	}
	if isPending {
		_ = e.repo.DeletePendingState(ctx, rule.ID)
	}

	e.mu.Lock()
	delete(e.pendingByRule, rule.ID)
	e.activeByRule[rule.ID] = evt
	e.mu.Unlock()

	e.emit(evt.SiteID, evt)

	if len(rule.NotifyEmails) > 0 {
		e.enqueueEmail(emailJob{
			eventID:      evt.ID,
			siteID:       rule.SiteID,
			recipients:   append([]string(nil), rule.NotifyEmails...),
			isClear:      false,
			severity:     rule.Severity,
			ruleName:     rule.Name,
			registerName: regName,
			condition:    rule.Condition,
			threshold:    rule.Threshold,
			value:        val,
			unit:         unit,
			timestamp:    ts,
		})
	}
}

// FreezeSiteOffline implements D12: freeze all ACTIVE alarms (is_stale=true) and cancel all PENDING timers.
func (e *Engine) FreezeSiteOffline(ctx context.Context, siteID uuid.UUID) {
	if err := e.repo.DeleteSitePendingStates(ctx, siteID); err != nil {
		slog.Warn("alarm_freeze_delete_pending_failed", "site_id", siteID, "error", err)
	}
	updated, err := e.repo.MarkSiteEventsStale(ctx, siteID)
	if err != nil {
		slog.Warn("alarm_freeze_mark_stale_failed", "site_id", siteID, "error", err)
	}

	e.mu.Lock()
	for ruleID, ps := range e.pendingByRule {
		if ps.SiteID == siteID {
			delete(e.pendingByRule, ruleID)
		}
	}
	for _, ev := range updated {
		if ev.RuleID != nil {
			e.activeByRule[*ev.RuleID] = ev
		}
	}
	e.mu.Unlock()

	for _, ev := range updated {
		e.emit(siteID, ev)
	}
}

// HandleRuleDeactivated clears pending holds and auto-clears any open ACTIVE event when a rule is disabled or deleted.
func (e *Engine) HandleRuleDeactivated(ctx context.Context, ruleID uuid.UUID) {
	e.mu.Lock()
	_, hasPending := e.pendingByRule[ruleID]
	delete(e.pendingByRule, ruleID)
	activeEvt, hasActive := e.activeByRule[ruleID]
	delete(e.activeByRule, ruleID)
	e.mu.Unlock()

	if hasPending {
		_ = e.repo.DeletePendingState(ctx, ruleID)
	}
	if hasActive {
		now := time.Now().UTC()
		if err := e.repo.ClearEvent(ctx, activeEvt.ID, now, nil); err != nil {
			slog.Warn("alarm_autoclear_on_deactivate_failed", "event_id", activeEvt.ID, "error", err)
			return
		}
		activeEvt.State = models.AlarmStateCleared
		activeEvt.ClearedAt = &now
		activeEvt.IsStale = false
		e.emit(activeEvt.SiteID, activeEvt)
	}
}

// HasSiteActiveOrPending reports whether the engine currently tracks any ACTIVE event or PENDING hold for siteID.
func (e *Engine) HasSiteActiveOrPending(siteID uuid.UUID) bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	for _, ev := range e.activeByRule {
		if ev.SiteID == siteID {
			return true
		}
	}
	for _, ps := range e.pendingByRule {
		if ps.SiteID == siteID {
			return true
		}
	}
	return false
}

// DropSiteActiveAlarms deletes all ACTIVE alarm events and PENDING hold timers for a site
// (used when the WebView module is disabled on the site). Historical CLEARED events and
// alarm rule definitions are preserved intact.
func (e *Engine) DropSiteActiveAlarms(ctx context.Context, siteID uuid.UUID) {
	if err := e.repo.DeleteSitePendingStates(ctx, siteID); err != nil {
		slog.Warn("alarm_drop_delete_pending_failed", "site_id", siteID, "error", err)
	}
	deleted, err := e.repo.DeleteSiteActiveEvents(ctx, siteID)
	if err != nil {
		slog.Warn("alarm_drop_delete_active_failed", "site_id", siteID, "error", err)
	}

	e.mu.Lock()
	for ruleID, ps := range e.pendingByRule {
		if ps.SiteID == siteID {
			delete(e.pendingByRule, ruleID)
		}
	}
	for ruleID, ev := range e.activeByRule {
		if ev.SiteID == siteID {
			delete(e.activeByRule, ruleID)
		}
	}
	e.mu.Unlock()

	if deleted > 0 {
		slog.Info("alarm_dropped_active_on_webview_disable", "site_id", siteID, "deleted_active", deleted)
	}
}

// PurgeSiteMemory removes cached rules/events/pending states for a deleted site.
func (e *Engine) PurgeSiteMemory(siteID uuid.UUID) {
	e.mu.Lock()
	for id, r := range e.rulesByID {
		if r.SiteID == siteID {
			delete(e.rulesByID, id)
		}
	}
	for mid, list := range e.rulesByMapping {
		kept := list[:0]
		for _, r := range list {
			if r.SiteID != siteID {
				kept = append(kept, r)
			}
		}
		if len(kept) == 0 {
			delete(e.rulesByMapping, mid)
		} else {
			e.rulesByMapping[mid] = kept
		}
	}
	for id, ev := range e.activeByRule {
		if ev.SiteID == siteID {
			delete(e.activeByRule, id)
		}
	}
	for id, ps := range e.pendingByRule {
		if ps.SiteID == siteID {
			delete(e.pendingByRule, id)
		}
	}
	e.mu.Unlock()
}

func (e *Engine) emit(siteID uuid.UUID, evt models.AlarmEvent) {
	if e.publishFn != nil {
		e.publishFn(siteID, evt)
	}
}

func (e *Engine) enqueueEmail(job emailJob) {
	if e.mailer == nil || len(job.recipients) == 0 {
		return
	}
	e.mu.RLock()
	running := e.started
	e.mu.RUnlock()
	if !running {
		return
	}
	select {
	case e.emailCh <- job:
	default:
		slog.Warn("alarm_email_queue_full", "event_id", job.eventID)
	}
}

func (e *Engine) emailWorker() {
	defer e.wg.Done()
	for job := range e.emailCh {
		siteName := job.siteID.String()[:8]
		if e.siteNameFn != nil {
			siteName = e.siteNameFn(job.siteID)
		}

		var subject, body string
		if job.isClear {
			subject = fmt.Sprintf("[WireDeck][CLEARED] %s - %s", siteName, job.ruleName)
			body = fmt.Sprintf(
				"Alarm Cleared\n\nSite: %s\nRule: %s\nRegister: %s\nSeverity: %s\nCondition: %s %s\nRecovered Value: %s\nCleared At (UTC): %s\n",
				siteName,
				job.ruleName,
				job.registerName,
				job.severity,
				ConditionSymbol(job.condition),
				formatVal(job.threshold, job.unit),
				formatVal(job.value, job.unit),
				job.timestamp.UTC().Format(time.RFC3339),
			)
		} else {
			subject = fmt.Sprintf("[WireDeck][%s] %s - %s", job.severity, siteName, job.ruleName)
			body = fmt.Sprintf(
				"Alarm Triggered\n\nSite: %s\nRule: %s\nRegister: %s\nSeverity: %s\nCondition: %s %s\nTriggered Value: %s\nTriggered At (UTC): %s\n",
				siteName,
				job.ruleName,
				job.registerName,
				job.severity,
				ConditionSymbol(job.condition),
				formatVal(job.threshold, job.unit),
				formatVal(job.value, job.unit),
				job.timestamp.UTC().Format(time.RFC3339),
			)
		}

		sendCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		err := e.mailer.Send(sendCtx, job.recipients, subject, body)
		cancel()

		if !job.isClear {
			updCtx, updCancel := context.WithTimeout(context.Background(), 5*time.Second)
			if err != nil {
				slog.Warn("alarm_email_send_failed", "event_id", job.eventID, "error", err)
				_ = e.repo.UpdateEventNotification(updCtx, job.eventID, nil, true, err.Error())
			} else {
				now := time.Now().UTC()
				_ = e.repo.UpdateEventNotification(updCtx, job.eventID, &now, false, "")
			}
			updCancel()
		} else if err != nil {
			slog.Warn("alarm_clear_email_send_failed", "event_id", job.eventID, "error", err)
		}
	}
}

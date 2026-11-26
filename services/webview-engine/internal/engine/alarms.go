package engine

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"

	"wiredeck/services/webview-engine/internal/alarm"
	"wiredeck/services/webview-engine/internal/models"
)

type AlarmRuleView struct {
	ID                  uuid.UUID `json:"id"`
	SiteID              uuid.UUID `json:"siteId"`
	RegisterMappingID   uuid.UUID `json:"registerMappingId"`
	RegisterName        string    `json:"registerName"`
	DeviceName          string    `json:"deviceName"`
	Unit                string    `json:"unit"`
	Precision           int       `json:"precision"`
	Name                string    `json:"name"`
	Condition           string    `json:"condition"`
	Threshold           float64   `json:"threshold"`
	ClearDeadband       float64   `json:"clearDeadband"`
	HoldSeconds         int       `json:"holdSeconds"`
	PollIntervalSeconds int       `json:"pollIntervalSeconds"`
	Severity            string    `json:"severity"`
	NotifyEmails        []string  `json:"notifyEmails"`
	NotifyOnClear       bool      `json:"notifyOnClear"`
	IsActive            bool      `json:"isActive"`
	RuntimeStatus       string    `json:"runtimeStatus"` // NORMAL | PENDING | ACTIVE
	CreatedAt           time.Time `json:"createdAt"`
	UpdatedAt           time.Time `json:"updatedAt"`
}

type AlarmEventView struct {
	ID                   uuid.UUID  `json:"id"`
	SiteID               uuid.UUID  `json:"siteId"`
	SiteName             string     `json:"siteName"`
	RuleID               *uuid.UUID `json:"ruleId,omitempty"`
	RegisterMappingID    *uuid.UUID `json:"registerMappingId,omitempty"`
	RuleName             string     `json:"ruleName"`
	RegisterName         string     `json:"registerName"`
	Condition            string     `json:"condition"`
	Threshold            float64    `json:"threshold"`
	Unit                 string     `json:"unit"`
	Severity             string     `json:"severity"`
	State                string     `json:"state"`
	IsStale              bool       `json:"isStale"`
	TriggeredAt          time.Time  `json:"triggeredAt"`
	TriggeredValue       float64    `json:"triggeredValue"`
	CurrentValue         *float64   `json:"currentValue,omitempty"`
	MaxDelta             *float64   `json:"maxDelta,omitempty"`
	ClearedAt            *time.Time `json:"clearedAt,omitempty"`
	ClearedValue         *float64   `json:"clearedValue,omitempty"`
	AcknowledgedAt       *time.Time `json:"acknowledgedAt,omitempty"`
	AcknowledgedByUserID *uuid.UUID `json:"acknowledgedByUserId,omitempty"`
	AcknowledgedByEmail  string     `json:"acknowledgedByEmail,omitempty"`
	AckNote              string     `json:"ackNote,omitempty"`
	NotifiedAt           *time.Time `json:"notifiedAt,omitempty"`
	NotifyFailed         bool       `json:"notifyFailed"`
	NotifyError          string     `json:"notifyError,omitempty"`
}

type AlarmSummary struct {
	ActiveCount         int `json:"activeCount"`
	ActiveCritical      int `json:"activeCritical"`
	ActiveWarning       int `json:"activeWarning"`
	ActiveInfo          int `json:"activeInfo"`
	UnacknowledgedCount int `json:"unacknowledgedCount"`
}

type CreateAlarmRuleInput struct {
	RegisterMappingID   uuid.UUID
	Name                string
	Condition           string
	Threshold           float64
	ClearDeadband       float64
	HoldSeconds         int
	PollIntervalSeconds int
	Severity            string
	NotifyEmails        []string
	NotifyOnClear       bool
	IsActive            *bool
}

type UpdateAlarmRuleInput struct {
	RegisterMappingID   *uuid.UUID
	Name                *string
	Condition           *string
	Threshold           *float64
	ClearDeadband       *float64
	HoldSeconds         *int
	PollIntervalSeconds *int
	Severity            *string
	NotifyEmails        *[]string
	NotifyOnClear       *bool
	IsActive            *bool
}

type AlarmEventFilter struct {
	SiteID       *uuid.UUID
	State        string // ACTIVE | CLEARED | ALL
	Severity     string // INFO | WARNING | CRITICAL | ""
	Acknowledged string // true | false | ""
	Limit        int
	Offset       int
}

func (s *Service) ToAlarmEventView(ev models.AlarmEvent) AlarmEventView {
	siteName := ev.SiteID.String()[:8]
	if s.cache != nil {
		siteName = s.cache.SiteName(ev.SiteID)
	}
	curVal := ev.LastValue
	if curVal == nil {
		v := ev.TriggeredValue
		curVal = &v
	}
	maxDelta := ev.PeakDelta
	if maxDelta == nil {
		d := ev.TriggeredValue - ev.Threshold
		maxDelta = &d
	}
	return AlarmEventView{
		ID:                   ev.ID,
		SiteID:               ev.SiteID,
		SiteName:             siteName,
		RuleID:               ev.RuleID,
		RegisterMappingID:    ev.RegisterMappingID,
		RuleName:             ev.RuleName,
		RegisterName:         ev.RegisterName,
		Condition:            ev.Condition,
		Threshold:            ev.Threshold,
		Unit:                 ev.Unit,
		Severity:             ev.Severity,
		State:                ev.State,
		IsStale:              ev.IsStale,
		TriggeredAt:          ev.TriggeredAt,
		TriggeredValue:       ev.TriggeredValue,
		CurrentValue:         curVal,
		MaxDelta:             maxDelta,
		ClearedAt:            ev.ClearedAt,
		ClearedValue:         ev.ClearedValue,
		AcknowledgedAt:       ev.AcknowledgedAt,
		AcknowledgedByUserID: ev.AcknowledgedByUserID,
		AcknowledgedByEmail:  ev.AcknowledgedByEmail,
		AckNote:              ev.AckNote,
		NotifiedAt:           ev.NotifiedAt,
		NotifyFailed:         ev.NotifyFailed,
		NotifyError:          ev.NotifyError,
	}
}

func normalizeEmails(raw []string) ([]string, error) {
	if len(raw) == 0 {
		return []string{}, nil
	}
	seen := make(map[string]struct{}, len(raw))
	out := make([]string, 0, len(raw))
	for _, entry := range raw {
		e := strings.ToLower(strings.TrimSpace(entry))
		if e == "" {
			continue
		}
		addr, err := mail.ParseAddress(e)
		if err != nil || addr.Address != e || !strings.Contains(e, "@") {
			return nil, fmt.Errorf("invalid notification email %q", entry)
		}
		if _, dup := seen[e]; dup {
			continue
		}
		seen[e] = struct{}{}
		out = append(out, e)
		if len(out) > 50 {
			return nil, fmt.Errorf("at most 50 notification emails are allowed per rule")
		}
	}
	return out, nil
}

func validateCondition(c string) (string, error) {
	c = strings.ToUpper(strings.TrimSpace(c))
	switch c {
	case models.AlarmCondGT, models.AlarmCondGTE, models.AlarmCondLT, models.AlarmCondLTE, models.AlarmCondEQ, models.AlarmCondNEQ:
		return c, nil
	default:
		return "", fmt.Errorf("invalid condition %q", c)
	}
}

func validateSeverity(sev string) (string, error) {
	sev = strings.ToUpper(strings.TrimSpace(sev))
	if sev == "" {
		return models.AlarmSeverityWarning, nil
	}
	switch sev {
	case models.AlarmSeverityInfo, models.AlarmSeverityWarning, models.AlarmSeverityCritical:
		return sev, nil
	default:
		return "", fmt.Errorf("invalid severity %q", sev)
	}
}

func (s *Service) enrichAlarmRules(ctx context.Context, siteID uuid.UUID, rules []models.AlarmRule) ([]AlarmRuleView, error) {
	if len(rules) == 0 {
		return []AlarmRuleView{}, nil
	}
	mappings, err := s.ListMappings(ctx, siteID)
	if err != nil {
		return nil, err
	}
	devices, err := s.ListDevices(ctx, siteID)
	if err != nil {
		return nil, err
	}
	devByID := make(map[uuid.UUID]models.Device, len(devices))
	for _, d := range devices {
		devByID[d.ID] = d
	}
	mapByID := make(map[uuid.UUID]models.RegisterMapping, len(mappings))
	for _, m := range mappings {
		mapByID[m.ID] = m
	}

	out := make([]AlarmRuleView, 0, len(rules))
	for _, r := range rules {
		regName := ""
		devName := ""
		unit := ""
		prec := 0
		if m, ok := mapByID[r.RegisterMappingID]; ok {
			regName = m.Name
			devName = devByID[m.DeviceID].Name
			unit, prec = alarm.ExtractUnitAndPrecision(m.ProtocolAttributes)
		}
		emails := r.NotifyEmails
		if emails == nil {
			emails = []string{}
		}
		rtStatus := "NORMAL"
		if s.alarmEng != nil {
			rtStatus = s.alarmEng.RuleRuntimeStatus(r.ID)
		}
		out = append(out, AlarmRuleView{
			ID:                  r.ID,
			SiteID:              r.SiteID,
			RegisterMappingID:   r.RegisterMappingID,
			RegisterName:        regName,
			DeviceName:          devName,
			Unit:                unit,
			Precision:           prec,
			Name:                r.Name,
			Condition:           r.Condition,
			Threshold:           r.Threshold,
			ClearDeadband:       r.ClearDeadband,
			HoldSeconds:         r.HoldSeconds,
			PollIntervalSeconds: r.PollIntervalSeconds,
			Severity:            r.Severity,
			NotifyEmails:        emails,
			NotifyOnClear:       r.NotifyOnClear,
			IsActive:            r.IsActive,
			RuntimeStatus:       rtStatus,
			CreatedAt:           r.CreatedAt,
			UpdatedAt:           r.UpdatedAt,
		})
	}
	return out, nil
}

func (s *Service) ListAlarmRules(ctx context.Context, siteID uuid.UUID) ([]AlarmRuleView, error) {
	var rules []models.AlarmRule
	if err := s.db.NewSelect().Model(&rules).Where("site_id = ?", siteID).Order("created_at ASC").Scan(ctx); err != nil {
		return nil, err
	}
	return s.enrichAlarmRules(ctx, siteID, rules)
}

func (s *Service) CreateAlarmRule(ctx context.Context, siteID uuid.UUID, in CreateAlarmRuleInput) (*AlarmRuleView, error) {
	exists, err := s.db.NewSelect().Model((*models.RegisterMapping)(nil)).
		Where("id = ? AND site_id = ?", in.RegisterMappingID, siteID).Exists(ctx)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, ErrNotFound
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return nil, fmt.Errorf("alarm rule name is required")
	}
	cond, err := validateCondition(in.Condition)
	if err != nil {
		return nil, err
	}
	sev, err := validateSeverity(in.Severity)
	if err != nil {
		return nil, err
	}
	emails, err := normalizeEmails(in.NotifyEmails)
	if err != nil {
		return nil, err
	}
	deadband := in.ClearDeadband
	if deadband < 0 {
		return nil, fmt.Errorf("clear deadband cannot be negative")
	}
	if cond == models.AlarmCondEQ || cond == models.AlarmCondNEQ {
		deadband = 0
	}
	holdSec := in.HoldSeconds
	if holdSec < 0 || holdSec > 86400 {
		return nil, fmt.Errorf("hold seconds must be between 0 and 86400")
	}
	pollSec := in.PollIntervalSeconds
	if pollSec == 0 {
		pollSec = 10
	}
	if pollSec < 1 || pollSec > 3600 {
		return nil, fmt.Errorf("poll interval must be between 1 and 3600 seconds")
	}
	isActive := true
	if in.IsActive != nil {
		isActive = *in.IsActive
	}

	now := time.Now().UTC()
	rule := &models.AlarmRule{
		ID:                  uuid.New(),
		SiteID:              siteID,
		RegisterMappingID:   in.RegisterMappingID,
		Name:                name,
		Condition:           cond,
		Threshold:           in.Threshold,
		ClearDeadband:       deadband,
		HoldSeconds:         holdSec,
		PollIntervalSeconds: pollSec,
		Severity:            sev,
		NotifyEmails:        emails,
		NotifyOnClear:       in.NotifyOnClear,
		IsActive:            isActive,
		CreatedAt:           now,
		UpdatedAt:           now,
	}
	if _, err := s.db.NewInsert().Model(rule).Exec(ctx); err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			return nil, ErrConflict
		}
		return nil, err
	}
	s.reloadAsync()
	views, err := s.enrichAlarmRules(ctx, siteID, []models.AlarmRule{*rule})
	if err != nil || len(views) == 0 {
		return nil, err
	}
	return &views[0], nil
}

func (s *Service) UpdateAlarmRule(ctx context.Context, siteID, ruleID uuid.UUID, in UpdateAlarmRuleInput) (*AlarmRuleView, error) {
	rule := new(models.AlarmRule)
	if err := s.db.NewSelect().Model(rule).Where("id = ? AND site_id = ?", ruleID, siteID).Scan(ctx); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	wasActive := rule.IsActive
	condOrThreshChanged := false

	if in.RegisterMappingID != nil && *in.RegisterMappingID != rule.RegisterMappingID {
		exists, err := s.db.NewSelect().Model((*models.RegisterMapping)(nil)).
			Where("id = ? AND site_id = ?", *in.RegisterMappingID, siteID).Exists(ctx)
		if err != nil {
			return nil, err
		}
		if !exists {
			return nil, ErrNotFound
		}
		rule.RegisterMappingID = *in.RegisterMappingID
		condOrThreshChanged = true
	}
	if in.Name != nil {
		trimmed := strings.TrimSpace(*in.Name)
		if trimmed == "" {
			return nil, fmt.Errorf("alarm rule name cannot be empty")
		}
		rule.Name = trimmed
	}
	if in.Condition != nil {
		cond, err := validateCondition(*in.Condition)
		if err != nil {
			return nil, err
		}
		if cond != rule.Condition {
			condOrThreshChanged = true
		}
		rule.Condition = cond
	}
	if in.Threshold != nil {
		if *in.Threshold != rule.Threshold {
			condOrThreshChanged = true
		}
		rule.Threshold = *in.Threshold
	}
	if in.ClearDeadband != nil {
		if *in.ClearDeadband < 0 {
			return nil, fmt.Errorf("clear deadband cannot be negative")
		}
		rule.ClearDeadband = *in.ClearDeadband
	}
	if rule.Condition == models.AlarmCondEQ || rule.Condition == models.AlarmCondNEQ {
		rule.ClearDeadband = 0
	}
	if in.HoldSeconds != nil {
		if *in.HoldSeconds < 0 || *in.HoldSeconds > 86400 {
			return nil, fmt.Errorf("hold seconds must be between 0 and 86400")
		}
		rule.HoldSeconds = *in.HoldSeconds
	}
	if in.PollIntervalSeconds != nil {
		if *in.PollIntervalSeconds < 1 || *in.PollIntervalSeconds > 3600 {
			return nil, fmt.Errorf("poll interval must be between 1 and 3600 seconds")
		}
		rule.PollIntervalSeconds = *in.PollIntervalSeconds
	}
	if in.Severity != nil {
		sev, err := validateSeverity(*in.Severity)
		if err != nil {
			return nil, err
		}
		rule.Severity = sev
	}
	if in.NotifyEmails != nil {
		emails, err := normalizeEmails(*in.NotifyEmails)
		if err != nil {
			return nil, err
		}
		rule.NotifyEmails = emails
	}
	if in.NotifyOnClear != nil {
		rule.NotifyOnClear = *in.NotifyOnClear
	}
	if in.IsActive != nil {
		rule.IsActive = *in.IsActive
	}
	if rule.NotifyEmails == nil {
		rule.NotifyEmails = []string{}
	}
	rule.UpdatedAt = time.Now().UTC()

	if _, err := s.db.NewUpdate().Model(rule).WherePK().Exec(ctx); err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			return nil, ErrConflict
		}
		return nil, err
	}

	if s.alarmEng != nil {
		if wasActive && !rule.IsActive {
			s.alarmEng.HandleRuleDeactivated(ctx, rule.ID)
		} else if condOrThreshChanged {
			// Clear stale pending hold timer so the updated condition starts fresh
			_, _ = s.db.NewDelete().Model((*models.AlarmPendingState)(nil)).Where("rule_id = ?", rule.ID).Exec(ctx)
		}
	}
	s.reloadAsync()

	views, err := s.enrichAlarmRules(ctx, siteID, []models.AlarmRule{*rule})
	if err != nil || len(views) == 0 {
		return nil, err
	}
	return &views[0], nil
}

func (s *Service) DeleteAlarmRule(ctx context.Context, siteID, ruleID uuid.UUID) error {
	exists, err := s.db.NewSelect().Model((*models.AlarmRule)(nil)).
		Where("id = ? AND site_id = ?", ruleID, siteID).Exists(ctx)
	if err != nil {
		return err
	}
	if !exists {
		return ErrNotFound
	}
	if s.alarmEng != nil {
		s.alarmEng.HandleRuleDeactivated(ctx, ruleID)
	}
	if _, err := s.db.NewDelete().Model((*models.AlarmRule)(nil)).Where("id = ? AND site_id = ?", ruleID, siteID).Exec(ctx); err != nil {
		return err
	}
	s.reloadAsync()
	return nil
}

func (s *Service) queryAlarmEvents(ctx context.Context, siteIDs []uuid.UUID, filter AlarmEventFilter) ([]AlarmEventView, AlarmSummary, error) {
	if len(siteIDs) == 0 {
		return []AlarmEventView{}, AlarmSummary{}, nil
	}

	summary, err := s.computeAlarmSummary(ctx, siteIDs)
	if err != nil {
		return nil, AlarmSummary{}, err
	}

	limit := filter.Limit
	if limit <= 0 || limit > 1000 {
		limit = 500
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	q := s.db.NewSelect().Model((*models.AlarmEvent)(nil)).Where("site_id IN (?)", bun.In(siteIDs))

	switch strings.ToUpper(strings.TrimSpace(filter.State)) {
	case models.AlarmStateActive:
		q = q.Where("state = ?", models.AlarmStateActive)
	case models.AlarmStateCleared:
		q = q.Where("state = ?", models.AlarmStateCleared)
	}
	if sev := strings.ToUpper(strings.TrimSpace(filter.Severity)); sev != "" && sev != "ALL" {
		q = q.Where("severity = ?", sev)
	}
	switch strings.ToLower(strings.TrimSpace(filter.Acknowledged)) {
	case "true":
		q = q.Where("acknowledged_at IS NOT NULL")
	case "false":
		q = q.Where("acknowledged_at IS NULL")
	}

	var rows []models.AlarmEvent
	if err := q.OrderExpr("CASE WHEN state = 'ACTIVE' THEN 0 ELSE 1 END ASC, triggered_at DESC").
		Limit(limit).
		Offset(offset).
		Scan(ctx, &rows); err != nil {
		return nil, AlarmSummary{}, err
	}

	out := make([]AlarmEventView, len(rows))
	for i, ev := range rows {
		out[i] = s.ToAlarmEventView(ev)
	}
	return out, summary, nil
}

func (s *Service) computeAlarmSummary(ctx context.Context, siteIDs []uuid.UUID) (AlarmSummary, error) {
	if len(siteIDs) == 0 {
		return AlarmSummary{}, nil
	}
	var active []struct {
		Severity       string     `bun:"severity"`
		AcknowledgedAt *time.Time `bun:"acknowledged_at"`
	}
	err := s.db.NewSelect().
		Table("alarm_events").
		Column("severity", "acknowledged_at").
		Where("site_id IN (?) AND state = ?", bun.In(siteIDs), models.AlarmStateActive).
		Scan(ctx, &active)
	if err != nil {
		return AlarmSummary{}, err
	}
	var sum AlarmSummary
	sum.ActiveCount = len(active)
	for _, r := range active {
		switch r.Severity {
		case models.AlarmSeverityCritical:
			sum.ActiveCritical++
		case models.AlarmSeverityWarning:
			sum.ActiveWarning++
		case models.AlarmSeverityInfo:
			sum.ActiveInfo++
		}
		if r.AcknowledgedAt == nil {
			sum.UnacknowledgedCount++
		}
	}
	return sum, nil
}

func (s *Service) ListSiteAlarmEvents(ctx context.Context, siteID uuid.UUID, filter AlarmEventFilter) ([]AlarmEventView, AlarmSummary, error) {
	return s.queryAlarmEvents(ctx, []uuid.UUID{siteID}, filter)
}

func (s *Service) ListGlobalAlarmEvents(ctx context.Context, userID uuid.UUID, filter AlarmEventFilter) ([]AlarmEventView, AlarmSummary, error) {
	allowed, err := s.cache.GetUserSites(ctx, userID)
	if err != nil {
		return nil, AlarmSummary{}, err
	}
	siteIDs := allowed.SiteIDs
	if filter.SiteID != nil {
		found := false
		for _, id := range siteIDs {
			if id == *filter.SiteID {
				found = true
				break
			}
		}
		if !found {
			return []AlarmEventView{}, AlarmSummary{}, nil
		}
		siteIDs = []uuid.UUID{*filter.SiteID}
	}
	return s.queryAlarmEvents(ctx, siteIDs, filter)
}

func (s *Service) GetGlobalAlarmSummary(ctx context.Context, userID uuid.UUID) (AlarmSummary, map[string]int, error) {
	allowed, err := s.cache.GetUserSites(ctx, userID)
	if err != nil {
		return AlarmSummary{}, nil, err
	}
	if len(allowed.SiteIDs) == 0 {
		return AlarmSummary{}, map[string]int{}, nil
	}
	var active []struct {
		SiteID         uuid.UUID  `bun:"site_id"`
		Severity       string     `bun:"severity"`
		AcknowledgedAt *time.Time `bun:"acknowledged_at"`
	}
	err = s.db.NewSelect().
		Table("alarm_events").
		Column("site_id", "severity", "acknowledged_at").
		Where("site_id IN (?) AND state = ?", bun.In(allowed.SiteIDs), models.AlarmStateActive).
		Scan(ctx, &active)
	if err != nil {
		return AlarmSummary{}, nil, err
	}
	var sum AlarmSummary
	bySite := make(map[string]int)
	sum.ActiveCount = len(active)
	for _, r := range active {
		bySite[r.SiteID.String()]++
		switch r.Severity {
		case models.AlarmSeverityCritical:
			sum.ActiveCritical++
		case models.AlarmSeverityWarning:
			sum.ActiveWarning++
		case models.AlarmSeverityInfo:
			sum.ActiveInfo++
		}
		if r.AcknowledgedAt == nil {
			sum.UnacknowledgedCount++
		}
	}
	return sum, bySite, nil
}

func (s *Service) AcknowledgeAlarmEvent(ctx context.Context, siteID, eventID, userID uuid.UUID, userEmail, note string) (*AlarmEventView, error) {
	ev := new(models.AlarmEvent)
	if err := s.db.NewSelect().Model(ev).Where("id = ? AND site_id = ?", eventID, siteID).Scan(ctx); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	now := time.Now().UTC()
	trimmedNote := strings.TrimSpace(note)
	ev.AcknowledgedAt = &now
	ev.AcknowledgedByUserID = &userID
	ev.AcknowledgedByEmail = strings.TrimSpace(userEmail)
	ev.AckNote = trimmedNote

	if _, err := s.db.NewUpdate().Model(ev).
		Column("acknowledged_at", "acknowledged_by_user_id", "acknowledged_by_email", "ack_note").
		WherePK().
		Exec(ctx); err != nil {
		return nil, err
	}

	if s.alarmEng != nil {
		s.alarmEng.SyncAckEvent(*ev)
	}
	view := s.ToAlarmEventView(*ev)
	if s.hub != nil {
		s.hub.PublishAlarmEvent(siteID, view)
	}
	return &view, nil
}

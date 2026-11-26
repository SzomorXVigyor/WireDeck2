package models

import (
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type MemberPolicy struct {
	bun.BaseModel `bun:"table:member_policies,alias:mp"`

	ID        uuid.UUID `bun:",pk,type:uuid,default:gen_random_uuid()"`
	SiteID    uuid.UUID `bun:",notnull,type:uuid"`
	UserID    uuid.UUID `bun:",notnull,type:uuid"`
	CanWrite  bool      `bun:",notnull"`
	UpdatedAt time.Time `bun:",notnull,default:current_timestamp"`
}

type Device struct {
	bun.BaseModel `bun:"table:devices,alias:d"`

	ID        uuid.UUID `bun:",pk,type:uuid,default:gen_random_uuid()"`
	SiteID    uuid.UUID `bun:",notnull,type:uuid"`
	Name      string    `bun:",notnull"`
	Host      string    `bun:",notnull,type:inet"`
	Port      int       `bun:",notnull,default:502"`
	Protocol  string    `bun:",notnull,default:'MODBUS_TCP'"`
	IsActive  bool      `bun:",notnull,default:true"`
	CreatedAt time.Time `bun:",notnull,default:current_timestamp"`
	UpdatedAt time.Time `bun:",notnull,default:current_timestamp"`
}

// ModbusTCPAttributes are the protocol-defined attributes for MODBUS_TCP registers.
type ModbusTCPAttributes struct {
	SlaveAddress    int    `json:"slaveAddress"`
	FunctionCode    string `json:"functionCode"`        // READ_COIL | READ_DISCRETE | READ_HOLDING | READ_INPUT
	RegisterAddress int    `json:"registerAddress"`     // 0..65535
	DataType        string `json:"dataType"`            // BOOL | UINT16 | INT16 | UINT32 | INT32 | FLOAT32
	WordOrder       string `json:"wordOrder"`           // BIG | LITTLE
	AccessMode      string `json:"accessMode"`          // READ | READ_WRITE
	Unit            string `json:"unit,omitempty"`      // Optional display/history unit (e.g. "C", "bar", "%")
	Precision       *int   `json:"precision,omitempty"` // Optional decimal shift 10^-x (-5..5)
}

type RegisterMapping struct {
	bun.BaseModel `bun:"table:register_mappings,alias:rm"`

	ID                 uuid.UUID      `bun:",pk,type:uuid,default:gen_random_uuid()"`
	DeviceID           uuid.UUID      `bun:",notnull,type:uuid"`
	SiteID             uuid.UUID      `bun:",notnull,type:uuid"`
	Name               string         `bun:",notnull"`
	ProtocolAttributes map[string]any `bun:",notnull,type:jsonb"`
	// Mirrored columns kept in sync for SQL constraints / indexing compatibility
	FunctionCode    string    `bun:",notnull"`
	RegisterAddress int       `bun:",notnull"`
	RegisterCount   int       `bun:",notnull,default:1"`
	DataType        string    `bun:",notnull"`
	WordOrder       string    `bun:",notnull,default:'BIG'"`
	AccessMode      string    `bun:",notnull,default:'READ'"`
	IsActive        bool      `bun:",notnull,default:true"`
	CreatedAt       time.Time `bun:",notnull,default:current_timestamp"`
	UpdatedAt       time.Time `bun:",notnull,default:current_timestamp"`
}

type HistoryEntry struct {
	bun.BaseModel `bun:"table:history_entries,alias:he"`

	ID                  uuid.UUID `bun:",pk,type:uuid,default:gen_random_uuid()"`
	SiteID              uuid.UUID `bun:",notnull,type:uuid"`
	RegisterMappingID   uuid.UUID `bun:",notnull,type:uuid"`
	PollIntervalSeconds int       `bun:",notnull,default:60"`
	RetentionDays       int       `bun:",notnull,default:30"`
	IsActive            bool      `bun:",notnull,default:true"`
	CreatedAt           time.Time `bun:",notnull,default:current_timestamp"`
	UpdatedAt           time.Time `bun:",notnull,default:current_timestamp"`
}

type Dashboard struct {
	bun.BaseModel `bun:"table:dashboards,alias:db"`

	ID             uuid.UUID      `bun:",pk,type:uuid,default:gen_random_uuid()"`
	SiteID         uuid.UUID      `bun:",notnull,type:uuid"`
	Name           string         `bun:",notnull"`
	Layout         map[string]any `bun:",notnull,type:jsonb"`
	IsDefault      bool           `bun:",notnull,default:false"`
	SortOrder      int            `bun:",notnull,default:0"`
	PollIntervalMs int            `bun:",notnull,default:1000"`
	CreatedAt      time.Time      `bun:",notnull,default:current_timestamp"`
	UpdatedAt      time.Time      `bun:",notnull,default:current_timestamp"`

	Widgets []Widget `bun:"rel:has-many,join:id=dashboard_id"`
}

type Widget struct {
	bun.BaseModel `bun:"table:widgets,alias:w"`

	ID                uuid.UUID      `bun:",pk,type:uuid,default:gen_random_uuid()"`
	DashboardID       uuid.UUID      `bun:",notnull,type:uuid"`
	RegisterMappingID *uuid.UUID     `bun:",type:uuid"`
	Name              string         `bun:",notnull"`
	WidgetType        string         `bun:",notnull"`
	SortOrder         int            `bun:",notnull,default:0"`
	Style             map[string]any `bun:",notnull,type:jsonb"`
	Extra             map[string]any `bun:",notnull,type:jsonb"`
	CreatedAt         time.Time      `bun:",notnull,default:current_timestamp"`
	UpdatedAt         time.Time      `bun:",notnull,default:current_timestamp"`
}

const (
	AlarmCondGT  = "GT"
	AlarmCondGTE = "GTE"
	AlarmCondLT  = "LT"
	AlarmCondLTE = "LTE"
	AlarmCondEQ  = "EQ"
	AlarmCondNEQ = "NEQ"

	AlarmSeverityInfo     = "INFO"
	AlarmSeverityWarning  = "WARNING"
	AlarmSeverityCritical = "CRITICAL"

	AlarmStateActive  = "ACTIVE"
	AlarmStateCleared = "CLEARED"
)

type AlarmRule struct {
	bun.BaseModel `bun:"table:alarm_rules,alias:ar"`

	ID                  uuid.UUID `bun:",pk,type:uuid,default:gen_random_uuid()"`
	SiteID              uuid.UUID `bun:",notnull,type:uuid"`
	RegisterMappingID   uuid.UUID `bun:",notnull,type:uuid"`
	Name                string    `bun:",notnull"`
	Condition           string    `bun:",notnull"`
	Threshold           float64   `bun:",notnull"`
	ClearDeadband       float64   `bun:",notnull,default:0"`
	HoldSeconds         int       `bun:",notnull,default:0"`
	PollIntervalSeconds int       `bun:",notnull,default:10"`
	Severity            string    `bun:",notnull,default:'WARNING'"`
	NotifyEmails        []string  `bun:",notnull,array"`
	NotifyOnClear       bool      `bun:",notnull,default:false"`
	IsActive            bool      `bun:",notnull,default:true"`
	CreatedAt           time.Time `bun:",notnull,default:current_timestamp"`
	UpdatedAt           time.Time `bun:",notnull,default:current_timestamp"`
}

type AlarmEvent struct {
	bun.BaseModel `bun:"table:alarm_events,alias:ae"`

	ID                   uuid.UUID  `bun:",pk,type:uuid,default:gen_random_uuid()"`
	SiteID               uuid.UUID  `bun:",notnull,type:uuid"`
	RuleID               *uuid.UUID `bun:",type:uuid"`
	RegisterMappingID    *uuid.UUID `bun:",type:uuid"`
	RuleName             string     `bun:",notnull"`
	RegisterName         string     `bun:",notnull"`
	Condition            string     `bun:",notnull"`
	Threshold            float64    `bun:",notnull"`
	Unit                 string     `bun:",notnull,default:''"`
	Severity             string     `bun:",notnull"`
	State                string     `bun:",notnull"`
	IsStale              bool       `bun:",notnull,default:false"`
	TriggeredAt          time.Time  `bun:",notnull,default:current_timestamp"`
	TriggeredValue       float64    `bun:",notnull"`
	LastValue            *float64
	PeakDelta            *float64
	ClearedAt            *time.Time
	ClearedValue         *float64
	AcknowledgedAt       *time.Time
	AcknowledgedByUserID *uuid.UUID `bun:",type:uuid"`
	AcknowledgedByEmail  string     `bun:",notnull,default:''"`
	AckNote              string     `bun:",notnull,default:''"`
	NotifiedAt           *time.Time
	NotifyFailed         bool   `bun:",notnull,default:false"`
	NotifyError          string `bun:",notnull,default:''"`
}

type AlarmPendingState struct {
	bun.BaseModel `bun:"table:alarm_pending_states,alias:aps"`

	RuleID    uuid.UUID `bun:",pk,type:uuid"`
	SiteID    uuid.UUID `bun:",notnull,type:uuid"`
	SinceAt   time.Time `bun:",notnull,default:current_timestamp"`
	LastValue float64   `bun:",notnull"`
}

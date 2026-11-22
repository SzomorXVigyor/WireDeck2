package models

import (
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type SiteWebviewSettings struct {
	bun.BaseModel `bun:"table:site_webview_settings,alias:sws"`

	SiteID         uuid.UUID `bun:",pk,type:uuid"`
	HistoryEnabled bool      `bun:",notnull"`
	UpdatedAt      time.Time `bun:",notnull,default:current_timestamp"`
}

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

	ID                    uuid.UUID `bun:",pk,type:uuid,default:gen_random_uuid()"`
	SiteID                uuid.UUID `bun:",notnull,type:uuid"`
	Name                  string    `bun:",notnull"`
	Host                  string    `bun:",notnull,type:inet"`
	Port                  int       `bun:",notnull,default:502"`
	Protocol              string    `bun:",notnull,default:'MODBUS_TCP'"`
	UnitID                int       `bun:",notnull,default:1"`
	MaxConcurrentRequests int       `bun:",notnull,default:1"`
	RequestTimeoutMs      int       `bun:",notnull,default:2000"`
	IsActive              bool      `bun:",notnull,default:true"`
	CreatedAt             time.Time `bun:",notnull,default:current_timestamp"`
	UpdatedAt             time.Time `bun:",notnull,default:current_timestamp"`
}

type RegisterMapping struct {
	bun.BaseModel `bun:"table:register_mappings,alias:rm"`

	ID                        uuid.UUID `bun:",pk,type:uuid,default:gen_random_uuid()"`
	DeviceID                  uuid.UUID `bun:",notnull,type:uuid"`
	SiteID                    uuid.UUID `bun:",notnull,type:uuid"`
	Name                      string    `bun:",notnull"`
	FunctionCode              string    `bun:",notnull"`
	RegisterAddress           int       `bun:",notnull"`
	RegisterCount             int       `bun:",notnull,default:1"`
	DataType                  string    `bun:",notnull"`
	WordOrder                 string    `bun:",notnull,default:'BIG'"`
	ByteOrder                 string    `bun:",notnull,default:'BIG'"`
	Scale                     float64   `bun:",notnull,default:1"`
	OffsetVal                 float64   `bun:",notnull,default:0"`
	Unit                      string    `bun:",notnull,default:''"`
	AccessMode                string    `bun:",notnull,default:'READ'"`
	WriteFunctionCode         string    `bun:",notnull,default:''"`
	InteractivePollIntervalMs int       `bun:",notnull,default:1000"`
	ArchivalPollIntervalMs    int       `bun:",notnull,default:60000"`
	Deadband                  float64   `bun:",notnull,default:0"`
	MaxIntervalSeconds        int       `bun:",notnull,default:300"`
	IsActive                  bool      `bun:",notnull,default:true"`
	CreatedAt                 time.Time `bun:",notnull,default:current_timestamp"`
	UpdatedAt                 time.Time `bun:",notnull,default:current_timestamp"`
}

type Dashboard struct {
	bun.BaseModel `bun:"table:dashboards,alias:db"`

	ID        uuid.UUID      `bun:",pk,type:uuid,default:gen_random_uuid()"`
	SiteID    uuid.UUID      `bun:",notnull,type:uuid"`
	Name      string         `bun:",notnull"`
	Layout    map[string]any `bun:",notnull,type:jsonb"`
	IsDefault bool           `bun:",notnull,default:false"`
	CreatedAt time.Time      `bun:",notnull,default:current_timestamp"`
	UpdatedAt time.Time      `bun:",notnull,default:current_timestamp"`

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

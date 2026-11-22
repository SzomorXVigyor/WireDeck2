package models

import (
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type VncTarget struct {
	bun.BaseModel `bun:"table:vnc_targets,alias:vt"`

	ID                uuid.UUID `bun:"id,pk,type:uuid,default:gen_random_uuid()"`
	SiteID            uuid.UUID `bun:"site_id,notnull,type:uuid"`
	Name              string    `bun:"name,notnull"`
	Host              string    `bun:"host,notnull,type:inet"`
	Port              int       `bun:"port,notnull,default:5900"`
	PasswordEncrypted []byte    `bun:"password_encrypted"`
	IsActive          bool      `bun:"is_active,notnull,default:true"`
	CreatedAt         time.Time `bun:"created_at,notnull,default:now()"`
	UpdatedAt         time.Time `bun:"updated_at,notnull,default:now()"`
}

type VncSession struct {
	bun.BaseModel `bun:"table:vnc_sessions,alias:vs"`

	ID          uuid.UUID  `bun:"id,pk,type:uuid,default:gen_random_uuid()"`
	SiteID      uuid.UUID  `bun:"site_id,notnull,type:uuid"`
	TargetID    uuid.UUID  `bun:"target_id,notnull,type:uuid"`
	UserID      uuid.UUID  `bun:"user_id,notnull,type:uuid"`
	StartedAt   time.Time  `bun:"started_at,notnull,default:now()"`
	EndedAt     *time.Time `bun:"ended_at"`
	BytesIn     int64      `bun:"bytes_in,notnull,default:0"`
	BytesOut    int64      `bun:"bytes_out,notnull,default:0"`
	CloseReason *string    `bun:"close_reason"`

	Target *VncTarget `bun:"rel:belongs-to,join:target_id=id"`
}

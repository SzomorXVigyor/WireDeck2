package models

import (
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type Runner struct {
	bun.BaseModel `bun:"table:runners,alias:r"`

	ID                  uuid.UUID  `bun:"id,pk,type:uuid,default:gen_random_uuid()"`
	SiteID              uuid.UUID  `bun:"site_id,notnull,type:uuid,unique"`
	ContainerName       string     `bun:"container_name,notnull,unique"`
	ContainerID         *string    `bun:"container_id"`
	Image               string     `bun:"image,notnull"`
	ConfFingerprint     *string    `bun:"conf_fingerprint"`
	AuthTokenEncrypted  []byte     `bun:"auth_token_encrypted"`
	DesiredState        string     `bun:"desired_state,notnull"`
	ObservedState       *string    `bun:"observed_state"`
	Status              string     `bun:"status,notnull"`
	HandshakeState      string     `bun:"handshake_state,notnull"`
	HandshakeAgeSeconds *int64     `bun:"handshake_age_seconds"`
	LastHandshakeAt     *time.Time `bun:"last_handshake_at"`
	LastCheckedAt       *time.Time `bun:"last_checked_at"`
	LastError           *string    `bun:"last_error"`
	LastReconciledAt    *time.Time `bun:"last_reconciled_at"`
	CreatedAt           time.Time  `bun:"created_at,notnull,default:now()"`
	UpdatedAt           time.Time  `bun:"updated_at,notnull,default:now()"`
}

type RunnerHealthEvent struct {
	bun.BaseModel `bun:"table:runner_health_events,alias:rhe"`

	ID         int64     `bun:"id,pk,autoincrement"`
	SiteID     uuid.UUID `bun:"site_id,notnull,type:uuid"`
	FromStatus string    `bun:"from_status,notnull"`
	ToStatus   string    `bun:"to_status,notnull"`
	OccurredAt time.Time `bun:"occurred_at,notnull,default:now()"`
	Detail     *string   `bun:"detail"`
}

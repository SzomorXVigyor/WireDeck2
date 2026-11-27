package models

import (
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"

	"wiredeck/pkg/wireguard"
)

const (
	UserStatusInvited  = "INVITED"
	UserStatusActive   = "ACTIVE"
	UserStatusDisabled = "DISABLED"

	TokenTypeInvite        = "INVITE"
	TokenTypePasswordReset = "PASSWORD_RESET"

	RoleSiteOperator = "SITE_OPERATOR"
	RoleSiteMember   = "SITE_MEMBER"
)

type User struct {
	bun.BaseModel `bun:"table:users,alias:u"`

	ID              uuid.UUID `bun:"id,pk,type:uuid,default:gen_random_uuid()"`
	Email           string    `bun:"email,notnull,unique"`
	PasswordHash    *string   `bun:"password_hash"`
	Status          string    `bun:"status,notnull"`
	IsPlatformAdmin bool      `bun:"is_platform_admin,notnull"`
	CreatedAt       time.Time `bun:"created_at,notnull,default:now()"`
	UpdatedAt       time.Time `bun:"updated_at,notnull,default:now()"`
}

type UserToken struct {
	bun.BaseModel `bun:"table:user_tokens,alias:ut"`

	ID        uuid.UUID  `bun:"id,pk,type:uuid,default:gen_random_uuid()"`
	UserID    uuid.UUID  `bun:"user_id,notnull,type:uuid"`
	Type      string     `bun:"type,notnull"`
	TokenHash string     `bun:"token_hash,notnull,unique"`
	ExpiresAt time.Time  `bun:"expires_at,notnull"`
	UsedAt    *time.Time `bun:"used_at"`
	CreatedAt time.Time  `bun:"created_at,notnull,default:now()"`
}

type ClientInfo struct {
	UserAgent string `json:"userAgent,omitempty"`
	IP        string `json:"ip,omitempty"`
}

type RefreshToken struct {
	bun.BaseModel `bun:"table:refresh_tokens,alias:rt"`

	ID           uuid.UUID   `bun:"id,pk,type:uuid,default:gen_random_uuid()"`
	UserID       uuid.UUID   `bun:"user_id,notnull,type:uuid"`
	TokenHash    string      `bun:"token_hash,notnull,unique"`
	ExpiresAt    time.Time   `bun:"expires_at,notnull"`
	RevokedAt    *time.Time  `bun:"revoked_at"`
	ReplacedByID *uuid.UUID  `bun:"replaced_by_id,type:uuid"`
	ClientInfo   *ClientInfo `bun:"client_info,type:jsonb"`
	CreatedAt    time.Time   `bun:"created_at,notnull,default:now()"`
}

type Site struct {
	bun.BaseModel `bun:"table:sites,alias:s"`

	ID                     uuid.UUID      `bun:"id,pk,type:uuid,default:gen_random_uuid()"`
	Name                   string         `bun:"name,notnull"`
	Slug                   string         `bun:"slug,notnull,unique"`
	WireguardConfEncrypted []byte         `bun:"wireguard_conf_encrypted,notnull"`
	WireguardMeta          wireguard.Meta `bun:"wireguard_meta,type:jsonb,notnull"`
	VncEnabled             bool           `bun:"vnc_enabled,notnull"`
	WebviewEnabled         bool           `bun:"webview_enabled,notnull"`
	IsActive               bool           `bun:"is_active,notnull"`
	Status                 string         `bun:"-"`
	HandshakeState         string         `bun:"-"`
	LastHandshakeAt        *time.Time     `bun:"-"`
	CreatedAt              time.Time      `bun:"created_at,notnull,default:now()"`
	UpdatedAt              time.Time      `bun:"updated_at,notnull,default:now()"`
}

type SiteMembership struct {
	bun.BaseModel `bun:"table:site_memberships,alias:sm"`

	ID        uuid.UUID `bun:"id,pk,type:uuid,default:gen_random_uuid()"`
	UserID    uuid.UUID `bun:"user_id,notnull,type:uuid"`
	SiteID    uuid.UUID `bun:"site_id,notnull,type:uuid"`
	Role      string    `bun:"role,notnull"`
	CreatedAt time.Time `bun:"created_at,notnull,default:now()"`

	User *User `bun:"rel:belongs-to,join:user_id=id"`
	Site *Site `bun:"rel:belongs-to,join:site_id=id"`
}

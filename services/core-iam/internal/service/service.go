package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/netip"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"

	"github.com/SzomorXVigyor/WireDeck2/pkg/auth"
	wcrypto "github.com/SzomorXVigyor/WireDeck2/pkg/crypto"
	"github.com/SzomorXVigyor/WireDeck2/pkg/internalclient"
	"github.com/SzomorXVigyor/WireDeck2/pkg/mailgopher"
	"github.com/SzomorXVigyor/WireDeck2/pkg/wireguard"
	"github.com/SzomorXVigyor/WireDeck2/services/core-iam/internal/models"
	"github.com/SzomorXVigyor/WireDeck2/services/core-iam/internal/password"
)

var (
	ErrNotFound          = errors.New("not found")
	ErrConflict          = errors.New("conflict")
	ErrInvalidCredential = errors.New("invalid email or password")
	ErrInvalidToken      = errors.New("invalid or expired token")
	ErrForbidden         = errors.New("forbidden")
	ErrUserDisabled      = errors.New("account is disabled or not activated")
)

var slugRe = regexp.MustCompile(`[^a-z0-9]+`)

type Config struct {
	PublicBaseURL  string
	RefreshTTL     time.Duration
	InviteTTL      time.Duration
	ResetTTL       time.Duration
	ReservedCIDRs  []netip.Prefix
	SkipDNSCheck   bool
	InitAdminEmail string
	InitAdminPass  string
}

type IAMService struct {
	db           *bun.DB
	cfg          Config
	signer       *auth.Signer
	wgCipher     *wcrypto.Cipher
	mailer       *mailgopher.Client
	orchestrator *internalclient.HTTPClient
	webview      *internalclient.HTTPClient
	webvnc       *internalclient.HTTPClient
}

func NewIAMService(
	db *bun.DB,
	cfg Config,
	signer *auth.Signer,
	wgCipher *wcrypto.Cipher,
	mailer *mailgopher.Client,
	orchestrator *internalclient.HTTPClient,
	webview *internalclient.HTTPClient,
	webvnc *internalclient.HTTPClient,
) *IAMService {
	if cfg.RefreshTTL <= 0 {
		cfg.RefreshTTL = 30 * 24 * time.Hour
	}
	if cfg.InviteTTL <= 0 {
		cfg.InviteTTL = 48 * time.Hour
	}
	if cfg.ResetTTL <= 0 {
		cfg.ResetTTL = time.Hour
	}
	return &IAMService{
		db:           db,
		cfg:          cfg,
		signer:       signer,
		wgCipher:     wgCipher,
		mailer:       mailer,
		orchestrator: orchestrator,
		webview:      webview,
		webvnc:       webvnc,
	}
}

func (s *IAMService) Signer() *auth.Signer { return s.signer }

// BootstrapAdmin creates the initial platform admin idempotently if zero platform admins exist.
func (s *IAMService) BootstrapAdmin(ctx context.Context) error {
	if strings.TrimSpace(s.cfg.InitAdminEmail) == "" || strings.TrimSpace(s.cfg.InitAdminPass) == "" {
		return nil
	}
	count, err := s.db.NewSelect().Model((*models.User)(nil)).
		Where("is_platform_admin = TRUE").
		Count(ctx)
	if err != nil {
		return fmt.Errorf("check bootstrap admin: %w", err)
	}
	if count > 0 {
		return nil
	}
	hash, err := password.Hash(s.cfg.InitAdminPass)
	if err != nil {
		return fmt.Errorf("hash bootstrap admin password: %w", err)
	}
	u := &models.User{
		Email:           strings.ToLower(strings.TrimSpace(s.cfg.InitAdminEmail)),
		PasswordHash:    &hash,
		Status:          models.UserStatusActive,
		IsPlatformAdmin: true,
	}
	if _, err := s.db.NewInsert().Model(u).Exec(ctx); err != nil {
		return fmt.Errorf("insert bootstrap admin: %w", err)
	}
	slog.Info("bootstrap_platform_admin_created", "email", u.Email, "user_id", u.ID)
	return nil
}

// --- Auth Flows ---

type AuthResult struct {
	AccessToken      string
	AccessExpiresAt  time.Time
	RefreshToken     string
	RefreshExpiresAt time.Time
	User             models.User
}

func (s *IAMService) Login(ctx context.Context, email, plainPassword, userAgent, ip string) (*AuthResult, error) {
	u := new(models.User)
	err := s.db.NewSelect().Model(u).Where("email = ?", strings.ToLower(strings.TrimSpace(email))).Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrInvalidCredential
		}
		return nil, err
	}
	if u.Status != models.UserStatusActive || u.PasswordHash == nil {
		return nil, ErrUserDisabled
	}
	if !password.Verify(plainPassword, *u.PasswordHash) {
		return nil, ErrInvalidCredential
	}
	return s.issueSession(ctx, *u, userAgent, ip)
}

func (s *IAMService) Refresh(ctx context.Context, rawRefresh, userAgent, ip string) (*AuthResult, error) {
	if rawRefresh == "" {
		return nil, ErrInvalidToken
	}
	hash := wcrypto.HashToken(rawRefresh)
	rt := new(models.RefreshToken)
	err := s.db.NewSelect().Model(rt).Where("token_hash = ?", hash).Scan(ctx)
	if err != nil {
		return nil, ErrInvalidToken
	}

	now := time.Now().UTC()
	// Reuse of an already-rotated or revoked refresh token revokes the entire user chain.
	if rt.RevokedAt != nil || rt.ReplacedByID != nil {
		slog.Warn("refresh_token_reuse_detected", "user_id", rt.UserID, "token_id", rt.ID)
		_, _ = s.db.NewUpdate().Model((*models.RefreshToken)(nil)).
			Set("revoked_at = ?", now).
			Where("user_id = ? AND revoked_at IS NULL", rt.UserID).
			Exec(ctx)
		return nil, ErrInvalidToken
	}
	if now.After(rt.ExpiresAt) {
		return nil, ErrInvalidToken
	}

	u := new(models.User)
	if err := s.db.NewSelect().Model(u).Where("id = ?", rt.UserID).Scan(ctx); err != nil {
		return nil, ErrInvalidToken
	}
	if u.Status != models.UserStatusActive {
		return nil, ErrUserDisabled
	}

	rawNew, newHash, err := wcrypto.GenerateToken(32)
	if err != nil {
		return nil, err
	}
	newRT := &models.RefreshToken{
		ID:         uuid.New(),
		UserID:     u.ID,
		TokenHash:  newHash,
		ExpiresAt:  now.Add(s.cfg.RefreshTTL),
		ClientInfo: &models.ClientInfo{UserAgent: userAgent, IP: ip},
	}

	err = s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if _, err := tx.NewInsert().Model(newRT).Exec(ctx); err != nil {
			return err
		}
		_, err := tx.NewUpdate().Model((*models.RefreshToken)(nil)).
			Set("revoked_at = ?", now).
			Set("replaced_by_id = ?", newRT.ID).
			Where("id = ?", rt.ID).
			Exec(ctx)
		return err
	})
	if err != nil {
		return nil, err
	}

	accessTok, accessExp, err := s.signer.IssueAccessToken(u.ID, u.Email, u.IsPlatformAdmin)
	if err != nil {
		return nil, err
	}
	return &AuthResult{
		AccessToken:      accessTok,
		AccessExpiresAt:  accessExp,
		RefreshToken:     rawNew,
		RefreshExpiresAt: newRT.ExpiresAt,
		User:             *u,
	}, nil
}

func (s *IAMService) Logout(ctx context.Context, rawRefresh string) error {
	if rawRefresh == "" {
		return nil
	}
	now := time.Now().UTC()
	_, err := s.db.NewUpdate().Model((*models.RefreshToken)(nil)).
		Set("revoked_at = ?", now).
		Where("token_hash = ? AND revoked_at IS NULL", wcrypto.HashToken(rawRefresh)).
		Exec(ctx)
	return err
}

func (s *IAMService) issueSession(ctx context.Context, u models.User, userAgent, ip string) (*AuthResult, error) {
	accessTok, accessExp, err := s.signer.IssueAccessToken(u.ID, u.Email, u.IsPlatformAdmin)
	if err != nil {
		return nil, err
	}
	rawRefresh, refreshHash, err := wcrypto.GenerateToken(32)
	if err != nil {
		return nil, err
	}
	rt := &models.RefreshToken{
		ID:         uuid.New(),
		UserID:     u.ID,
		TokenHash:  refreshHash,
		ExpiresAt:  time.Now().UTC().Add(s.cfg.RefreshTTL),
		ClientInfo: &models.ClientInfo{UserAgent: userAgent, IP: ip},
	}
	if _, err := s.db.NewInsert().Model(rt).Exec(ctx); err != nil {
		return nil, err
	}
	return &AuthResult{
		AccessToken:      accessTok,
		AccessExpiresAt:  accessExp,
		RefreshToken:     rawRefresh,
		RefreshExpiresAt: rt.ExpiresAt,
		User:             u,
	}, nil
}

// --- Invite & Password Flows ---

type TokenCheckResult struct {
	Valid  bool
	Reason string // "", "INVALID", "EXPIRED", "USED"
}

func (s *IAMService) VerifyUserToken(ctx context.Context, rawToken, tokenType string) TokenCheckResult {
	trimmed := strings.TrimSpace(rawToken)
	if trimmed == "" || s.db == nil {
		return TokenCheckResult{Valid: false, Reason: "INVALID"}
	}
	ut := new(models.UserToken)
	err := s.db.NewSelect().Model(ut).
		Where("token_hash = ? AND type = ?", wcrypto.HashToken(trimmed), tokenType).
		Scan(ctx)
	if err != nil {
		return TokenCheckResult{Valid: false, Reason: "INVALID"}
	}
	if ut.UsedAt != nil {
		return TokenCheckResult{Valid: false, Reason: "USED"}
	}
	if time.Now().UTC().After(ut.ExpiresAt) {
		return TokenCheckResult{Valid: false, Reason: "EXPIRED"}
	}
	u := new(models.User)
	if err := s.db.NewSelect().Model(u).Where("id = ?", ut.UserID).Scan(ctx); err != nil {
		return TokenCheckResult{Valid: false, Reason: "INVALID"}
	}
	if u.Status == models.UserStatusDisabled {
		return TokenCheckResult{Valid: false, Reason: "INVALID"}
	}
	if tokenType == models.TokenTypeInvite && u.Status == models.UserStatusActive {
		return TokenCheckResult{Valid: false, Reason: "USED"}
	}
	return TokenCheckResult{Valid: true}
}

func (s *IAMService) AcceptInvite(ctx context.Context, rawToken, newPassword string) error {
	hash, err := password.Hash(newPassword)
	if err != nil {
		return err
	}
	ut := new(models.UserToken)
	err = s.db.NewSelect().Model(ut).
		Where("token_hash = ? AND type = ? AND used_at IS NULL", wcrypto.HashToken(strings.TrimSpace(rawToken)), models.TokenTypeInvite).
		Scan(ctx)
	if err != nil || time.Now().UTC().After(ut.ExpiresAt) {
		return ErrInvalidToken
	}

	now := time.Now().UTC()
	u := new(models.User)
	return s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if err := tx.NewSelect().Model(u).Where("id = ?", ut.UserID).Scan(ctx); err != nil {
			return err
		}
		if u.Status == models.UserStatusDisabled {
			return ErrUserDisabled
		}
		if u.Status == models.UserStatusActive {
			return ErrInvalidToken
		}
		u.PasswordHash = &hash
		u.Status = models.UserStatusActive
		u.UpdatedAt = now
		if _, err := tx.NewUpdate().Model(u).Column("password_hash", "status", "updated_at").WherePK().Exec(ctx); err != nil {
			return err
		}
		_, err := tx.NewUpdate().Model((*models.UserToken)(nil)).
			Set("used_at = ?", now).
			Where("user_id = ? AND type = ? AND used_at IS NULL", u.ID, models.TokenTypeInvite).
			Exec(ctx)
		return err
	})
}

func (s *IAMService) ChangePassword(ctx context.Context, userID uuid.UUID, currentPassword, newPassword string) error {
	u := new(models.User)
	if err := s.db.NewSelect().Model(u).Where("id = ?", userID).Scan(ctx); err != nil {
		return ErrNotFound
	}
	if u.Status != models.UserStatusActive || u.PasswordHash == nil {
		return ErrUserDisabled
	}
	if !password.Verify(currentPassword, *u.PasswordHash) {
		return ErrInvalidCredential
	}
	newHash, err := password.Hash(newPassword)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	return s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		u.PasswordHash = &newHash
		u.UpdatedAt = now
		if _, err := tx.NewUpdate().Model(u).Column("password_hash", "updated_at").WherePK().Exec(ctx); err != nil {
			return err
		}
		// Revoke all existing refresh sessions on password change
		_, err := tx.NewUpdate().Model((*models.RefreshToken)(nil)).
			Set("revoked_at = ?", now).
			Where("user_id = ? AND revoked_at IS NULL", u.ID).
			Exec(ctx)
		return err
	})
}

func (s *IAMService) RequestPasswordReset(ctx context.Context, email string) {
	u := new(models.User)
	err := s.db.NewSelect().Model(u).
		Where("email = ? AND status = ?", strings.ToLower(strings.TrimSpace(email)), models.UserStatusActive).
		Scan(ctx)
	if err != nil {
		return // anti-enumeration
	}
	raw, hash, err := wcrypto.GenerateToken(32)
	if err != nil {
		return
	}
	ut := &models.UserToken{
		UserID:    u.ID,
		Type:      models.TokenTypePasswordReset,
		TokenHash: hash,
		ExpiresAt: time.Now().UTC().Add(s.cfg.ResetTTL),
	}
	if _, err := s.db.NewInsert().Model(ut).Exec(ctx); err != nil {
		slog.Error("insert_password_reset_token_failed", "error", err)
		return
	}
	link := fmt.Sprintf("%s/reset-password?token=%s", strings.TrimRight(s.cfg.PublicBaseURL, "/"), raw)
	body := fmt.Sprintf("Hello,\n\nA password reset was requested for your WireDeck2 account.\nOpen the link below within 1 hour to set a new password:\n\n%s\n\nIf you did not request this, you can safely ignore this email.\n", link)
	if err := s.mailer.Send(ctx, []string{u.Email}, "WireDeck2 - Password Reset", body); err != nil {
		slog.Warn("password_reset_email_failed", "email", u.Email, "error", err)
	}
}

func (s *IAMService) ResetPassword(ctx context.Context, rawToken, newPassword string) error {
	newHash, err := password.Hash(newPassword)
	if err != nil {
		return err
	}
	ut := new(models.UserToken)
	err = s.db.NewSelect().Model(ut).
		Where("token_hash = ? AND type = ? AND used_at IS NULL", wcrypto.HashToken(rawToken), models.TokenTypePasswordReset).
		Scan(ctx)
	if err != nil || time.Now().UTC().After(ut.ExpiresAt) {
		return ErrInvalidToken
	}

	now := time.Now().UTC()
	return s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if _, err := tx.NewUpdate().Model((*models.User)(nil)).
			Set("password_hash = ?, updated_at = ?", newHash, now).
			Where("id = ?", ut.UserID).
			Exec(ctx); err != nil {
			return err
		}
		if _, err := tx.NewUpdate().Model((*models.RefreshToken)(nil)).
			Set("revoked_at = ?", now).
			Where("user_id = ? AND revoked_at IS NULL", ut.UserID).
			Exec(ctx); err != nil {
			return err
		}
		_, err := tx.NewUpdate().Model((*models.UserToken)(nil)).
			Set("used_at = ?", now).
			Where("user_id = ? AND type = ? AND used_at IS NULL", ut.UserID, models.TokenTypePasswordReset).
			Exec(ctx)
		return err
	})
}

// --- User Management (Platform Admin) ---

type InitialMembership struct {
	SiteID uuid.UUID
	Role   string
}

func (s *IAMService) GetUser(ctx context.Context, id uuid.UUID) (*models.User, error) {
	u := new(models.User)
	if err := s.db.NewSelect().Model(u).Where("id = ?", id).Scan(ctx); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return u, nil
}

func (s *IAMService) ListUsers(ctx context.Context) ([]models.User, error) {
	var users []models.User
	err := s.db.NewSelect().Model(&users).Order("created_at DESC").Scan(ctx)
	return users, err
}

func (s *IAMService) CreateInvitedUser(ctx context.Context, email string, isPlatformAdmin bool, memberships []InitialMembership) (*models.User, bool, error) {
	normEmail := strings.ToLower(strings.TrimSpace(email))
	raw, hash, err := wcrypto.GenerateToken(32)
	if err != nil {
		return nil, false, err
	}

	u := &models.User{
		ID:              uuid.New(),
		Email:           normEmail,
		Status:          models.UserStatusInvited,
		IsPlatformAdmin: isPlatformAdmin,
	}

	err = s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if _, err := tx.NewInsert().Model(u).Exec(ctx); err != nil {
			if strings.Contains(err.Error(), "duplicate key") {
				return ErrConflict
			}
			return err
		}
		for _, m := range memberships {
			sm := &models.SiteMembership{
				UserID: u.ID,
				SiteID: m.SiteID,
				Role:   m.Role,
			}
			if _, err := tx.NewInsert().Model(sm).Exec(ctx); err != nil {
				return err
			}
		}
		ut := &models.UserToken{
			UserID:    u.ID,
			Type:      models.TokenTypeInvite,
			TokenHash: hash,
			ExpiresAt: time.Now().UTC().Add(s.cfg.InviteTTL),
		}
		_, err := tx.NewInsert().Model(ut).Exec(ctx)
		return err
	})
	if err != nil {
		return nil, false, err
	}

	sent := s.sendInviteEmail(ctx, u.Email, raw)
	return u, sent, nil
}

func (s *IAMService) ResendInvite(ctx context.Context, userID uuid.UUID) (bool, error) {
	u, err := s.GetUser(ctx, userID)
	if err != nil {
		return false, err
	}
	if u.Status != models.UserStatusInvited {
		return false, fmt.Errorf("%w: user is already %s", ErrConflict, u.Status)
	}
	raw, hash, err := wcrypto.GenerateToken(32)
	if err != nil {
		return false, err
	}
	ut := &models.UserToken{
		UserID:    u.ID,
		Type:      models.TokenTypeInvite,
		TokenHash: hash,
		ExpiresAt: time.Now().UTC().Add(s.cfg.InviteTTL),
	}
	if _, err := s.db.NewInsert().Model(ut).Exec(ctx); err != nil {
		return false, err
	}
	return s.sendInviteEmail(ctx, u.Email, raw), nil
}

func (s *IAMService) sendInviteEmail(ctx context.Context, email, rawToken string) bool {
	link := fmt.Sprintf("%s/accept-invite?token=%s", strings.TrimRight(s.cfg.PublicBaseURL, "/"), rawToken)
	body := fmt.Sprintf("Welcome to WireDeck2!\n\nAn administrator has created an account for you.\nOpen the link below within 48 hours to set your password and activate your account:\n\n%s\n", link)
	if err := s.mailer.Send(ctx, []string{email}, "You are invited to WireDeck2", body); err != nil {
		slog.Warn("invite_email_failed", "email", email, "error", err)
		return false
	}
	return true
}

func (s *IAMService) UpdateUser(ctx context.Context, userID uuid.UUID, status *string, isPlatformAdmin *bool) (*models.User, error) {
	u, err := s.GetUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	if status != nil {
		u.Status = *status
	}
	if isPlatformAdmin != nil {
		u.IsPlatformAdmin = *isPlatformAdmin
	}
	u.UpdatedAt = time.Now().UTC()
	if _, err := s.db.NewUpdate().Model(u).Column("status", "is_platform_admin", "updated_at").WherePK().Exec(ctx); err != nil {
		return nil, err
	}
	if u.Status == models.UserStatusDisabled {
		_, _ = s.db.NewUpdate().Model((*models.RefreshToken)(nil)).
			Set("revoked_at = ?", time.Now().UTC()).
			Where("user_id = ? AND revoked_at IS NULL", u.ID).
			Exec(ctx)
	}
	return u, nil
}

func (s *IAMService) DeleteUser(ctx context.Context, userID uuid.UUID) error {
	res, err := s.db.NewDelete().Model((*models.User)(nil)).Where("id = ?", userID).Exec(ctx)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// --- Site & Membership Management ---

type AccessibleSite struct {
	Site models.Site
	Role string
}

func (s *IAMService) ListAccessibleSites(ctx context.Context, userID uuid.UUID, isPlatformAdmin bool) ([]AccessibleSite, error) {
	if isPlatformAdmin {
		var sites []models.Site
		if err := s.db.NewSelect().Model(&sites).Order("name ASC").Scan(ctx); err != nil {
			return nil, err
		}
		out := make([]AccessibleSite, len(sites))
		for i, site := range sites {
			out[i] = AccessibleSite{Site: site, Role: models.RoleSiteOperator}
		}
		return out, nil
	}

	var memberships []models.SiteMembership
	err := s.db.NewSelect().Model(&memberships).
		Relation("Site").
		Where("sm.user_id = ?", userID).
		Order("site.name ASC").
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]AccessibleSite, 0, len(memberships))
	for _, m := range memberships {
		if m.Site != nil {
			out = append(out, AccessibleSite{Site: *m.Site, Role: m.Role})
		}
	}
	return out, nil
}

func (s *IAMService) GetSiteForUser(ctx context.Context, siteID, userID uuid.UUID, isPlatformAdmin bool) (*AccessibleSite, error) {
	site := new(models.Site)
	if err := s.db.NewSelect().Model(site).Where("id = ?", siteID).Scan(ctx); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if isPlatformAdmin {
		return &AccessibleSite{Site: *site, Role: models.RoleSiteOperator}, nil
	}
	m := new(models.SiteMembership)
	if err := s.db.NewSelect().Model(m).Where("site_id = ? AND user_id = ?", siteID, userID).Scan(ctx); err != nil {
		return nil, ErrForbidden
	}
	return &AccessibleSite{Site: *site, Role: m.Role}, nil
}

func (s *IAMService) CheckSiteRole(ctx context.Context, siteID, userID uuid.UUID, isPlatformAdmin bool) (string, bool, error) {
	if isPlatformAdmin {
		return models.RoleSiteOperator, true, nil
	}
	m := new(models.SiteMembership)
	err := s.db.NewSelect().Model(m).Where("site_id = ? AND user_id = ?", siteID, userID).Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", false, nil
		}
		return "", false, err
	}
	return m.Role, true, nil
}

type CreateSiteInput struct {
	Name               string
	Slug               string
	WireguardConf      string
	OverrideAllowedIPs []string
	VncEnabled         bool
	WebviewEnabled     bool
	Activate           bool
}

func (s *IAMService) CreateSite(ctx context.Context, in CreateSiteInput) (*models.Site, []string, error) {
	parsed, err := wireguard.ParseAndNormalize(ctx, in.WireguardConf, wireguard.Options{
		ReservedCIDRs:      s.cfg.ReservedCIDRs,
		OverrideAllowedIPs: in.OverrideAllowedIPs,
		SkipDNSCheck:       s.cfg.SkipDNSCheck,
	})
	if err != nil {
		return nil, nil, err
	}
	enc, err := s.wgCipher.Encrypt([]byte(parsed.NormalizedConf))
	if err != nil {
		return nil, nil, fmt.Errorf("encrypt wireguard conf: %w", err)
	}

	slug := sanitizeSlug(in.Slug)
	if slug == "" {
		slug = sanitizeSlug(in.Name)
	}

	site := &models.Site{
		ID:                     uuid.New(),
		Name:                   strings.TrimSpace(in.Name),
		Slug:                   slug,
		WireguardConfEncrypted: enc,
		WireguardMeta:          parsed.Meta,
		VncEnabled:             in.VncEnabled,
		WebviewEnabled:         in.WebviewEnabled,
		IsActive:               in.Activate,
		Status:                 "UNKNOWN",
		HandshakeState:         "UNSPECIFIED",
	}

	if _, err := s.db.NewInsert().Model(site).Exec(ctx); err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			return nil, nil, ErrConflict
		}
		return nil, nil, err
	}

	if site.IsActive {
		if err := s.syncRunner(ctx, site, parsed.NormalizedConf); err != nil {
			slog.Warn("runner_sync_on_create_failed", "site_id", site.ID, "error", err)
		}
	}
	return site, parsed.Warnings, nil
}

func (s *IAMService) UpdateSiteSettings(ctx context.Context, siteID uuid.UUID, name *string, vnc, webview *bool) (*models.Site, error) {
	site := new(models.Site)
	if err := s.db.NewSelect().Model(site).Where("id = ?", siteID).Scan(ctx); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if name != nil {
		site.Name = strings.TrimSpace(*name)
	}
	if vnc != nil {
		site.VncEnabled = *vnc
	}
	if webview != nil {
		site.WebviewEnabled = *webview
	}
	site.UpdatedAt = time.Now().UTC()
	if _, err := s.db.NewUpdate().Model(site).
		Column("name", "vnc_enabled", "webview_enabled", "updated_at").
		WherePK().Exec(ctx); err != nil {
		return nil, err
	}
	return site, nil
}

func (s *IAMService) UpdateSiteConf(ctx context.Context, siteID uuid.UUID, rawConf string, overrideAllowedIPs []string) (*models.Site, []string, error) {
	site := new(models.Site)
	if err := s.db.NewSelect().Model(site).Where("id = ?", siteID).Scan(ctx); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil, ErrNotFound
		}
		return nil, nil, err
	}
	parsed, err := wireguard.ParseAndNormalize(ctx, rawConf, wireguard.Options{
		ReservedCIDRs:      s.cfg.ReservedCIDRs,
		OverrideAllowedIPs: overrideAllowedIPs,
		SkipDNSCheck:       s.cfg.SkipDNSCheck,
	})
	if err != nil {
		return nil, nil, err
	}
	enc, err := s.wgCipher.Encrypt([]byte(parsed.NormalizedConf))
	if err != nil {
		return nil, nil, err
	}
	site.WireguardConfEncrypted = enc
	site.WireguardMeta = parsed.Meta
	site.UpdatedAt = time.Now().UTC()
	if _, err := s.db.NewUpdate().Model(site).
		Column("wireguard_conf_encrypted", "wireguard_meta", "updated_at").
		WherePK().Exec(ctx); err != nil {
		return nil, nil, err
	}
	if site.IsActive {
		if err := s.syncRunner(ctx, site, parsed.NormalizedConf); err != nil {
			slog.Warn("runner_sync_on_conf_rotate_failed", "site_id", site.ID, "error", err)
		}
	}
	return site, parsed.Warnings, nil
}

func (s *IAMService) SetSiteActive(ctx context.Context, siteID uuid.UUID, active bool) (*models.Site, error) {
	site := new(models.Site)
	if err := s.db.NewSelect().Model(site).Where("id = ?", siteID).Scan(ctx); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	plainConf, err := s.wgCipher.Decrypt(site.WireguardConfEncrypted)
	if err != nil {
		return nil, fmt.Errorf("decrypt site conf: %w", err)
	}
	site.IsActive = active
	site.UpdatedAt = time.Now().UTC()
	if !active {
		site.Status = "OFFLINE"
	}
	if _, err := s.db.NewUpdate().Model(site).Column("is_active", "status", "updated_at").WherePK().Exec(ctx); err != nil {
		return nil, err
	}
	if err := s.syncRunner(ctx, site, string(plainConf)); err != nil {
		return nil, fmt.Errorf("sync runner state: %w", err)
	}
	return site, nil
}

func (s *IAMService) DeleteSite(ctx context.Context, siteID uuid.UUID) error {
	res, err := s.db.NewDelete().Model((*models.Site)(nil)).Where("id = ?", siteID).Exec(ctx)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	// Cascade runner removal & downstream site data cleanup (best-effort)
	if s.orchestrator != nil && s.orchestrator.BaseURL != "" {
		if _, err := s.orchestrator.DoJSON(ctx, http.MethodDelete, "/runners/"+siteID.String(), nil, nil); err != nil {
			slog.Warn("orchestrator_delete_runner_failed", "site_id", siteID, "error", err)
		}
	}
	for name, client := range map[string]*internalclient.HTTPClient{"webview-engine": s.webview, "webvnc-proxy": s.webvnc} {
		if client != nil && client.BaseURL != "" {
			if _, err := client.DoJSON(ctx, http.MethodDelete, "/internal/sites/"+siteID.String()+"/data", nil, nil); err != nil {
				slog.Warn("downstream_site_data_delete_failed", "target", name, "site_id", siteID, "error", err)
			}
		}
	}
	return nil
}

func (s *IAMService) ListSiteMembers(ctx context.Context, siteID uuid.UUID) ([]models.SiteMembership, error) {
	var members []models.SiteMembership
	err := s.db.NewSelect().Model(&members).Relation("User").Where("sm.site_id = ?", siteID).Order("sm.created_at ASC").Scan(ctx)
	return members, err
}

func (s *IAMService) UpsertSiteMember(ctx context.Context, siteID, userID uuid.UUID, role string) (*models.SiteMembership, error) {
	sm := &models.SiteMembership{
		ID:     uuid.New(),
		SiteID: siteID,
		UserID: userID,
		Role:   role,
	}
	_, err := s.db.NewInsert().Model(sm).
		On("CONFLICT (user_id, site_id) DO UPDATE").
		Set("role = EXCLUDED.role").
		Returning("*").
		Exec(ctx)
	return sm, err
}

func (s *IAMService) RemoveSiteMember(ctx context.Context, siteID, userID uuid.UUID) error {
	res, err := s.db.NewDelete().Model((*models.SiteMembership)(nil)).
		Where("site_id = ? AND user_id = ?", siteID, userID).
		Exec(ctx)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// OperatorInviteSiteMember lets a Site Operator (or Platform Admin) add an existing user
// OR invite a brand-new email directly to a site with the SITE_MEMBER role.
func (s *IAMService) OperatorInviteSiteMember(ctx context.Context, siteID uuid.UUID, email string) (*models.User, *models.SiteMembership, bool, bool, error) {
	normEmail := strings.ToLower(strings.TrimSpace(email))
	existing := new(models.User)
	err := s.db.NewSelect().Model(existing).Where("email = ?", normEmail).Scan(ctx)

	if err == nil {
		// User already exists: ensure membership on site (never downgrade an existing SITE_OPERATOR).
		sm := new(models.SiteMembership)
		mErr := s.db.NewSelect().Model(sm).Where("site_id = ? AND user_id = ?", siteID, existing.ID).Scan(ctx)
		if mErr == nil {
			return existing, sm, false, false, nil
		}
		sm = &models.SiteMembership{
			ID:     uuid.New(),
			SiteID: siteID,
			UserID: existing.ID,
			Role:   models.RoleSiteMember,
		}
		if _, err := s.db.NewInsert().Model(sm).Exec(ctx); err != nil {
			return nil, nil, false, false, err
		}
		return existing, sm, false, false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, nil, false, false, err
	}

	// Brand-new email: create INVITED user + SITE_MEMBER membership + send invite email.
	u, sent, err := s.CreateInvitedUser(ctx, normEmail, false, []InitialMembership{
		{SiteID: siteID, Role: models.RoleSiteMember},
	})
	if err != nil {
		return nil, nil, false, false, err
	}
	sm := &models.SiteMembership{SiteID: siteID, UserID: u.ID, Role: models.RoleSiteMember}
	return u, sm, true, sent, nil
}

// OperatorRemoveSiteMember removes a SITE_MEMBER from siteID.
// Site Operators cannot remove another SITE_OPERATOR (only Platform Admins can).
func (s *IAMService) OperatorRemoveSiteMember(ctx context.Context, siteID, targetUserID uuid.UUID, callerIsPlatformAdmin bool) error {
	sm := new(models.SiteMembership)
	if err := s.db.NewSelect().Model(sm).Where("site_id = ? AND user_id = ?", siteID, targetUserID).Scan(ctx); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if sm.Role == models.RoleSiteOperator && !callerIsPlatformAdmin {
		return ErrForbidden
	}
	_, err := s.db.NewDelete().Model(sm).WherePK().Exec(ctx)
	return err
}

// --- Runner Orchestrator Integration & Internal API Helpers ---

func (s *IAMService) syncRunner(ctx context.Context, site *models.Site, normalizedConf string) error {
	if s.orchestrator == nil || s.orchestrator.BaseURL == "" {
		return nil
	}
	desired := "STOPPED"
	if site.IsActive {
		desired = "RUNNING"
	}
	_, err := s.orchestrator.DoJSON(ctx, http.MethodPut, "/runners/"+site.ID.String(), internalclient.EnsureRunnerRequest{
		Conf:            normalizedConf,
		ConfFingerprint: site.WireguardMeta.ConfFingerprint,
		DesiredState:    desired,
	}, nil)
	return err
}

func (s *IAMService) ListRunners(ctx context.Context) ([]internalclient.RunnerRecord, error) {
	if s.orchestrator == nil || s.orchestrator.BaseURL == "" {
		return []internalclient.RunnerRecord{}, nil
	}
	var out []internalclient.RunnerRecord
	_, err := s.orchestrator.DoJSON(ctx, http.MethodGet, "/runners", nil, &out)
	return out, err
}

func (s *IAMService) RestartRunner(ctx context.Context, siteID uuid.UUID) error {
	if s.orchestrator == nil || s.orchestrator.BaseURL == "" {
		return errors.New("orchestrator URL not configured")
	}
	_, err := s.orchestrator.DoJSON(ctx, http.MethodPost, "/runners/"+siteID.String()+"/restart", nil, nil)
	return err
}

// ProjectRunnerStatuses fetches runner health from runner-orchestrator and projects it onto sites.
func (s *IAMService) ProjectRunnerStatuses(ctx context.Context) {
	if s.orchestrator == nil || s.orchestrator.BaseURL == "" {
		return
	}
	runners, err := s.ListRunners(ctx)
	if err != nil {
		slog.Warn("status_projection_fetch_failed", "error", err)
		return
	}
	now := time.Now().UTC()
	for _, r := range runners {
		_, err := s.db.NewUpdate().Model((*models.Site)(nil)).
			Set("status = ?", r.Status).
			Set("handshake_state = ?", r.HandshakeState).
			Set("last_handshake_at = ?", r.LastHandshakeAt).
			Set("status_updated_at = ?", now).
			Where("id = ?", r.SiteID).
			Exec(ctx)
		if err != nil {
			slog.Warn("status_projection_update_failed", "site_id", r.SiteID, "error", err)
		}
	}
}

func (s *IAMService) InternalAuthz(ctx context.Context, siteID, userID uuid.UUID) (*internalclient.SiteAuthzResponse, error) {
	u, err := s.GetUser(ctx, userID)
	if err != nil || u.Status != models.UserStatusActive {
		return &internalclient.SiteAuthzResponse{Allowed: false}, nil
	}
	site := new(models.Site)
	if err := s.db.NewSelect().Model(site).Where("id = ?", siteID).Scan(ctx); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	role := ""
	allowed := false
	if u.IsPlatformAdmin {
		role = models.RoleSiteOperator
		allowed = true
	} else {
		m := new(models.SiteMembership)
		if err := s.db.NewSelect().Model(m).Where("site_id = ? AND user_id = ?", siteID, userID).Scan(ctx); err == nil {
			role = m.Role
			allowed = true
		}
	}

	return &internalclient.SiteAuthzResponse{
		Allowed:         allowed,
		Role:            role,
		IsPlatformAdmin: u.IsPlatformAdmin,
		IsActive:        site.IsActive,
		Status:          site.Status,
		VncEnabled:      site.VncEnabled,
		WebviewEnabled:  site.WebviewEnabled,
		LanSubnets:      site.WireguardMeta.LanSubnets,
	}, nil
}

func (s *IAMService) InternalRuntimeStates(ctx context.Context) ([]internalclient.SiteRuntimeState, error) {
	var sites []models.Site
	if err := s.db.NewSelect().Model(&sites).Scan(ctx); err != nil {
		return nil, err
	}
	out := make([]internalclient.SiteRuntimeState, len(sites))
	for i, site := range sites {
		out[i] = internalclient.SiteRuntimeState{
			SiteID:         site.ID,
			Name:           site.Name,
			IsActive:       site.IsActive,
			Status:         site.Status,
			VncEnabled:     site.VncEnabled,
			WebviewEnabled: site.WebviewEnabled,
			LanSubnets:     site.WireguardMeta.LanSubnets,
		}
	}
	return out, nil
}

// InternalUserWebviewSites returns all site IDs the active user may view WebView/Alarms for.
func (s *IAMService) InternalUserWebviewSites(ctx context.Context, userID uuid.UUID) (*internalclient.UserSitesResponse, error) {
	u, err := s.GetUser(ctx, userID)
	if err != nil || u.Status != models.UserStatusActive {
		return &internalclient.UserSitesResponse{SiteIDs: []uuid.UUID{}}, nil
	}
	var ids []uuid.UUID
	if u.IsPlatformAdmin {
		if err := s.db.NewSelect().
			Table("sites").
			Column("id").
			Where("webview_enabled = TRUE").
			Scan(ctx, &ids); err != nil {
			return nil, err
		}
	} else {
		if err := s.db.NewSelect().
			TableExpr("site_memberships AS sm").
			Join("JOIN sites AS s ON s.id = sm.site_id").
			Column("s.id").
			Where("sm.user_id = ? AND s.webview_enabled = TRUE", userID).
			Scan(ctx, &ids); err != nil {
			return nil, err
		}
	}
	if ids == nil {
		ids = []uuid.UUID{}
	}
	return &internalclient.UserSitesResponse{
		IsPlatformAdmin: u.IsPlatformAdmin,
		SiteIDs:         ids,
	}, nil
}

func (s *IAMService) InternalDesiredRunners(ctx context.Context) ([]internalclient.DesiredRunnerSpec, error) {
	var sites []models.Site
	if err := s.db.NewSelect().Model(&sites).Scan(ctx); err != nil {
		return nil, err
	}
	out := make([]internalclient.DesiredRunnerSpec, len(sites))
	for i, site := range sites {
		desired := "STOPPED"
		if site.IsActive {
			desired = "RUNNING"
		}
		out[i] = internalclient.DesiredRunnerSpec{
			SiteID:          site.ID,
			DesiredState:    desired,
			ConfFingerprint: site.WireguardMeta.ConfFingerprint,
		}
	}
	return out, nil
}

func (s *IAMService) InternalRunnerConf(ctx context.Context, siteID uuid.UUID) (*internalclient.RunnerConfResponse, error) {
	site := new(models.Site)
	if err := s.db.NewSelect().Model(site).Where("id = ?", siteID).Scan(ctx); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	plain, err := s.wgCipher.Decrypt(site.WireguardConfEncrypted)
	if err != nil {
		return nil, err
	}
	return &internalclient.RunnerConfResponse{
		SiteID:          site.ID,
		Conf:            string(plain),
		ConfFingerprint: site.WireguardMeta.ConfFingerprint,
	}, nil
}

func sanitizeSlug(in string) string {
	s := strings.ToLower(strings.TrimSpace(in))
	s = slugRe.ReplaceAllString(s, "-")
	return strings.Trim(s, "-")
}

package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/SzomorXVigyor/WireDeck2/pkg/api"
	"github.com/SzomorXVigyor/WireDeck2/pkg/auth"
	"github.com/SzomorXVigyor/WireDeck2/pkg/internalclient"
	"github.com/SzomorXVigyor/WireDeck2/pkg/telemetry"
	"github.com/SzomorXVigyor/WireDeck2/pkg/wireguard"
	"github.com/SzomorXVigyor/WireDeck2/services/core-iam/internal/models"
	"github.com/SzomorXVigyor/WireDeck2/services/core-iam/internal/service"
)

const refreshCookieName = "wd2_refresh"

type PublicRouterOptions struct {
	EnableDocs   bool
	CookieSecure bool
}

// --- DTOs ---

type UserDTO struct {
	ID              uuid.UUID `json:"id"`
	Email           string    `json:"email"`
	Status          string    `json:"status"`
	IsPlatformAdmin bool      `json:"isPlatformAdmin"`
	CreatedAt       time.Time `json:"createdAt"`
}

func toUserDTO(u models.User) UserDTO {
	return UserDTO{
		ID:              u.ID,
		Email:           u.Email,
		Status:          u.Status,
		IsPlatformAdmin: u.IsPlatformAdmin,
		CreatedAt:       u.CreatedAt,
	}
}

type SiteDTO struct {
	ID              uuid.UUID       `json:"id"`
	Name            string          `json:"name"`
	Slug            string          `json:"slug"`
	Role            string          `json:"role,omitempty"`
	VncEnabled      bool            `json:"vncEnabled"`
	WebviewEnabled  bool            `json:"webviewEnabled"`
	IsActive        bool            `json:"isActive"`
	Status          string          `json:"status"`
	HandshakeState  string          `json:"handshakeState"`
	LastHandshakeAt *time.Time      `json:"lastHandshakeAt,omitempty"`
	Wireguard       *wireguard.Meta `json:"wireguard,omitempty"`
	CreatedAt       time.Time       `json:"createdAt"`
}

// toSiteDTO maps Site to DTO. WireGuard metadata is ONLY populated for Platform Admins.
func toSiteDTO(s models.Site, role string, includeWG bool) SiteDTO {
	dto := SiteDTO{
		ID:              s.ID,
		Name:            s.Name,
		Slug:            s.Slug,
		Role:            role,
		VncEnabled:      s.VncEnabled,
		WebviewEnabled:  s.WebviewEnabled,
		IsActive:        s.IsActive,
		Status:          s.Status,
		HandshakeState:  s.HandshakeState,
		LastHandshakeAt: s.LastHandshakeAt,
		CreatedAt:       s.CreatedAt,
	}
	if includeWG {
		m := s.WireguardMeta
		dto.Wireguard = &m
	}
	return dto
}

type SiteMemberDTO struct {
	UserID    uuid.UUID `json:"userId"`
	Email     string    `json:"email"`
	Status    string    `json:"status"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"createdAt"`
}

// NewPublicRouter builds the schema-validated Huma v2 public HTTP router for core-iam (:8080)
// with strictly separated Auth, User, Site Operator, and Platform Admin groups.
//
// Valid roles across the entire platform:
//   - Platform Admin (isPlatformAdmin = true): sole owner of sites, site features, WireGuard configs, runners, global users.
//   - Site Operator (SITE_OPERATOR): manages site members (invites as SITE_MEMBER) and service-level config (VNC targets, WebView).
//   - Site Member (SITE_MEMBER): uses enabled services (WebView, VNC); fine-grained policies live inside each service.
func NewPublicRouter(svc *service.IAMService, verifier *auth.Verifier, opts PublicRouterOptions) http.Handler {
	r := chi.NewRouter()
	r.Use(telemetry.CorrelationIDMiddleware)
	r.Use(telemetry.RequestLogger)

	// JWKS endpoint (standard RFC 7517 public discovery)
	r.Get("/api/v1/iam/.well-known/jwks.json", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "public, max-age=300")
		_ = json.NewEncoder(w).Encode(svc.Signer().JWKS())
	})

	humaAPI := api.NewHumaAPI(r, "WireDeck2 Core IAM API", "2.2.0", "/api/v1/iam", opts.EnableDocs)

	makeRefreshCookie := func(raw string, expires time.Time) http.Cookie {
		return http.Cookie{
			Name:     refreshCookieName,
			Value:    raw,
			Path:     "/api/v1/iam/auth",
			HttpOnly: true,
			Secure:   opts.CookieSecure,
			SameSite: http.SameSiteStrictMode,
			Expires:  expires,
		}
	}
	clearRefreshCookie := func() http.Cookie {
		return http.Cookie{
			Name:     refreshCookieName,
			Value:    "",
			Path:     "/api/v1/iam/auth",
			HttpOnly: true,
			Secure:   opts.CookieSecure,
			SameSite: http.SameSiteStrictMode,
			MaxAge:   -1,
		}
	}

	// =========================================================================
	// 1. Public Authentication Endpoints (/api/v1/iam/auth/*)
	// =========================================================================
	type LoginInput struct {
		UserAgent string `header:"User-Agent"`
		Body      struct {
			Email    string `json:"email" format:"email" required:"true"`
			Password string `json:"password" minLength:"1" required:"true"`
		}
	}
	type AuthResponse struct {
		SetCookie http.Cookie `header:"Set-Cookie"`
		Body      struct {
			AccessToken string    `json:"accessToken"`
			ExpiresAt   time.Time `json:"expiresAt"`
			User        UserDTO   `json:"user"`
		}
	}

	huma.Register(humaAPI, huma.Operation{
		OperationID: "authLogin",
		Method:      http.MethodPost,
		Path:        "/api/v1/iam/auth/login",
		Summary:     "Authenticate with email and password",
		Tags:        []string{"Auth"},
	}, func(ctx context.Context, in *LoginInput) (*AuthResponse, error) {
		res, err := svc.Login(ctx, in.Body.Email, in.Body.Password, in.UserAgent, "")
		if err != nil {
			return nil, mapError(err)
		}
		out := &AuthResponse{SetCookie: makeRefreshCookie(res.RefreshToken, res.RefreshExpiresAt)}
		out.Body.AccessToken = res.AccessToken
		out.Body.ExpiresAt = res.AccessExpiresAt
		out.Body.User = toUserDTO(res.User)
		return out, nil
	})

	type RefreshInput struct {
		UserAgent     string      `header:"User-Agent"`
		RefreshCookie http.Cookie `cookie:"wd2_refresh"`
	}
	huma.Register(humaAPI, huma.Operation{
		OperationID: "authRefresh",
		Method:      http.MethodPost,
		Path:        "/api/v1/iam/auth/refresh",
		Summary:     "Rotate HttpOnly refresh cookie and issue a new RS256 access token",
		Tags:        []string{"Auth"},
	}, func(ctx context.Context, in *RefreshInput) (*AuthResponse, error) {
		res, err := svc.Refresh(ctx, in.RefreshCookie.Value, in.UserAgent, "")
		if err != nil {
			return nil, mapError(err)
		}
		out := &AuthResponse{SetCookie: makeRefreshCookie(res.RefreshToken, res.RefreshExpiresAt)}
		out.Body.AccessToken = res.AccessToken
		out.Body.ExpiresAt = res.AccessExpiresAt
		out.Body.User = toUserDTO(res.User)
		return out, nil
	})

	type LogoutInput struct {
		RefreshCookie http.Cookie `cookie:"wd2_refresh"`
	}
	type LogoutOutput struct {
		SetCookie http.Cookie `header:"Set-Cookie"`
	}
	huma.Register(humaAPI, huma.Operation{
		OperationID:   "authLogout",
		Method:        http.MethodPost,
		Path:          "/api/v1/iam/auth/logout",
		Summary:       "Revoke current refresh token and clear cookie",
		Tags:          []string{"Auth"},
		DefaultStatus: http.StatusNoContent,
	}, func(ctx context.Context, in *LogoutInput) (*LogoutOutput, error) {
		_ = svc.Logout(ctx, in.RefreshCookie.Value)
		return &LogoutOutput{SetCookie: clearRefreshCookie()}, nil
	})

	type VerifyTokenInput struct {
		Body struct {
			Token string `json:"token" required:"true"`
			Type  string `json:"type" enum:"INVITE,PASSWORD_RESET" required:"true"`
		}
	}
	type VerifyTokenOutput struct {
		Body struct {
			Valid  bool   `json:"valid"`
			Reason string `json:"reason,omitempty"`
		}
	}
	huma.Register(humaAPI, huma.Operation{
		OperationID: "authVerifyToken",
		Method:      http.MethodPost,
		Path:        "/api/v1/iam/auth/verify-token",
		Summary:     "Check whether an account activation or password reset link token is valid before showing the password form",
		Tags:        []string{"Auth"},
	}, func(ctx context.Context, in *VerifyTokenInput) (*VerifyTokenOutput, error) {
		res := svc.VerifyUserToken(ctx, in.Body.Token, in.Body.Type)
		out := &VerifyTokenOutput{}
		out.Body.Valid = res.Valid
		out.Body.Reason = res.Reason
		return out, nil
	})

	type AcceptInviteInput struct {
		Body struct {
			Token    string `json:"token" minLength:"16" required:"true"`
			Password string `json:"password" minLength:"8" maxLength:"128" required:"true"`
		}
	}
	huma.Register(humaAPI, huma.Operation{
		OperationID:   "authAcceptInvite",
		Method:        http.MethodPost,
		Path:          "/api/v1/iam/auth/accept-invite",
		Summary:       "Activate an invited user account and set initial password",
		Tags:          []string{"Auth"},
		DefaultStatus: http.StatusNoContent,
	}, func(ctx context.Context, in *AcceptInviteInput) (*struct{}, error) {
		if err := svc.AcceptInvite(ctx, in.Body.Token, in.Body.Password); err != nil {
			return nil, mapError(err)
		}
		return nil, nil
	})

	type ForgotPasswordInput struct {
		Body struct {
			Email string `json:"email" format:"email" required:"true"`
		}
	}
	huma.Register(humaAPI, huma.Operation{
		OperationID:   "authForgotPassword",
		Method:        http.MethodPost,
		Path:          "/api/v1/iam/auth/forgot-password",
		Summary:       "Request a password reset email",
		Tags:          []string{"Auth"},
		DefaultStatus: http.StatusAccepted,
	}, func(ctx context.Context, in *ForgotPasswordInput) (*struct{}, error) {
		svc.RequestPasswordReset(ctx, in.Body.Email)
		return nil, nil
	})

	type ResetPasswordInput struct {
		Body struct {
			Token       string `json:"token" minLength:"16" required:"true"`
			NewPassword string `json:"newPassword" minLength:"8" maxLength:"128" required:"true"`
		}
	}
	huma.Register(humaAPI, huma.Operation{
		OperationID:   "authResetPassword",
		Method:        http.MethodPost,
		Path:          "/api/v1/iam/auth/reset-password",
		Summary:       "Complete a password reset using an emailed token",
		Tags:          []string{"Auth"},
		DefaultStatus: http.StatusNoContent,
	}, func(ctx context.Context, in *ResetPasswordInput) (*struct{}, error) {
		if err := svc.ResetPassword(ctx, in.Body.Token, in.Body.NewPassword); err != nil {
			return nil, mapError(err)
		}
		return nil, nil
	})

	// =========================================================================
	// 2. Authenticated User / Member Endpoints (/api/v1/iam/me/*, /sites*)
	// =========================================================================
	userGroup := huma.NewGroup(humaAPI)
	userGroup.UseMiddleware(humaAuthMiddleware(humaAPI, verifier))

	type MeOutput struct {
		Body UserDTO
	}
	huma.Register(userGroup, huma.Operation{
		OperationID: "getMe",
		Method:      http.MethodGet,
		Path:        "/api/v1/iam/me",
		Summary:     "Return current authenticated user profile",
		Tags:        []string{"User"},
		Security:    []map[string][]string{{"BearerAuth": {}}},
	}, func(ctx context.Context, _ *struct{}) (*MeOutput, error) {
		uid, ok := auth.UserIDFromContext(ctx)
		if !ok {
			return nil, huma.Error401Unauthorized("unauthenticated")
		}
		u, err := svc.GetUser(ctx, uid)
		if err != nil {
			return nil, mapError(err)
		}
		return &MeOutput{Body: toUserDTO(*u)}, nil
	})

	type ChangePasswordInput struct {
		Body struct {
			CurrentPassword string `json:"currentPassword" minLength:"1" required:"true"`
			NewPassword     string `json:"newPassword" minLength:"8" maxLength:"128" required:"true"`
		}
	}
	huma.Register(userGroup, huma.Operation{
		OperationID:   "changeMyPassword",
		Method:        http.MethodPost,
		Path:          "/api/v1/iam/me/change-password",
		Summary:       "Change current user's password and revoke all active sessions",
		Tags:          []string{"User"},
		Security:      []map[string][]string{{"BearerAuth": {}}},
		DefaultStatus: http.StatusNoContent,
	}, func(ctx context.Context, in *ChangePasswordInput) (*LogoutOutput, error) {
		uid, _ := auth.UserIDFromContext(ctx)
		if err := svc.ChangePassword(ctx, uid, in.Body.CurrentPassword, in.Body.NewPassword); err != nil {
			return nil, mapError(err)
		}
		return &LogoutOutput{SetCookie: clearRefreshCookie()}, nil
	})

	type ListSitesOutput struct {
		Body struct {
			Sites []SiteDTO `json:"sites"`
		}
	}
	huma.Register(userGroup, huma.Operation{
		OperationID: "listAccessibleSites",
		Method:      http.MethodGet,
		Path:        "/api/v1/iam/sites",
		Summary:     "List sites accessible to the authenticated caller",
		Tags:        []string{"User"},
		Security:    []map[string][]string{{"BearerAuth": {}}},
	}, func(ctx context.Context, _ *struct{}) (*ListSitesOutput, error) {
		claims, _ := auth.ClaimsFromContext(ctx)
		uid, _ := auth.UserIDFromContext(ctx)
		items, err := svc.ListAccessibleSites(ctx, uid, claims.IsPlatformAdmin)
		if err != nil {
			return nil, mapError(err)
		}
		out := &ListSitesOutput{}
		out.Body.Sites = make([]SiteDTO, len(items))
		for i, item := range items {
			// WireGuard metadata is strictly Platform Admin only
			out.Body.Sites[i] = toSiteDTO(item.Site, item.Role, claims.IsPlatformAdmin)
		}
		return out, nil
	})

	type SiteIDInput struct {
		SiteID uuid.UUID `path:"siteId" format:"uuid"`
	}
	type SiteOutput struct {
		Body SiteDTO
	}
	huma.Register(userGroup, huma.Operation{
		OperationID: "getSite",
		Method:      http.MethodGet,
		Path:        "/api/v1/iam/sites/{siteId}",
		Summary:     "Get details for an accessible site",
		Tags:        []string{"User"},
		Security:    []map[string][]string{{"BearerAuth": {}}},
	}, func(ctx context.Context, in *SiteIDInput) (*SiteOutput, error) {
		claims, _ := auth.ClaimsFromContext(ctx)
		uid, _ := auth.UserIDFromContext(ctx)
		item, err := svc.GetSiteForUser(ctx, in.SiteID, uid, claims.IsPlatformAdmin)
		if err != nil {
			return nil, mapError(err)
		}
		return &SiteOutput{Body: toSiteDTO(item.Site, item.Role, claims.IsPlatformAdmin)}, nil
	})

	// =========================================================================
	// 3. Site Operator Endpoints (/api/v1/iam/sites/{siteId}/operator/*)
	//    Site Operators can list/invite/remove SITE_MEMBERs on their site.
	//    They CANNOT access WireGuard configs or toggle site features.
	// =========================================================================
	siteOpGroup := huma.NewGroup(humaAPI)
	siteOpGroup.UseMiddleware(humaAuthMiddleware(humaAPI, verifier), humaSiteOperatorMiddleware(humaAPI, svc))

	type ListMembersOutput struct {
		Body struct {
			Members []SiteMemberDTO `json:"members"`
		}
	}
	huma.Register(siteOpGroup, huma.Operation{
		OperationID: "siteOperatorListMembers",
		Method:      http.MethodGet,
		Path:        "/api/v1/iam/sites/{siteId}/operator/members",
		Summary:     "List members assigned to a site (Site Operator)",
		Tags:        []string{"Site Operator"},
		Security:    []map[string][]string{{"BearerAuth": {}}},
	}, func(ctx context.Context, in *SiteIDInput) (*ListMembersOutput, error) {
		members, err := svc.ListSiteMembers(ctx, in.SiteID)
		if err != nil {
			return nil, mapError(err)
		}
		out := &ListMembersOutput{}
		out.Body.Members = make([]SiteMemberDTO, 0, len(members))
		for _, m := range members {
			email := ""
			st := ""
			if m.User != nil {
				email = m.User.Email
				st = m.User.Status
			}
			out.Body.Members = append(out.Body.Members, SiteMemberDTO{
				UserID:    m.UserID,
				Email:     email,
				Status:    st,
				Role:      m.Role,
				CreatedAt: m.CreatedAt,
			})
		}
		return out, nil
	})

	type OperatorInviteMemberInput struct {
		SiteID uuid.UUID `path:"siteId" format:"uuid"`
		Body   struct {
			Email string `json:"email" format:"email" required:"true"`
		}
	}
	type OperatorInviteMemberOutput struct {
		Body struct {
			Member     SiteMemberDTO `json:"member"`
			CreatedNew bool          `json:"createdNew"`
			InviteSent bool          `json:"inviteSent"`
		}
	}
	huma.Register(siteOpGroup, huma.Operation{
		OperationID:   "siteOperatorInviteMember",
		Method:        http.MethodPost,
		Path:          "/api/v1/iam/sites/{siteId}/operator/members/invite",
		Summary:       "Add an existing user or invite a new email to the site as SITE_MEMBER (Site Operator)",
		Tags:          []string{"Site Operator"},
		Security:      []map[string][]string{{"BearerAuth": {}}},
		DefaultStatus: http.StatusCreated,
	}, func(ctx context.Context, in *OperatorInviteMemberInput) (*OperatorInviteMemberOutput, error) {
		u, sm, createdNew, inviteSent, err := svc.OperatorInviteSiteMember(ctx, in.SiteID, in.Body.Email)
		if err != nil {
			return nil, mapError(err)
		}
		out := &OperatorInviteMemberOutput{}
		out.Body.Member = SiteMemberDTO{
			UserID:    u.ID,
			Email:     u.Email,
			Status:    u.Status,
			Role:      sm.Role,
			CreatedAt: sm.CreatedAt,
		}
		out.Body.CreatedNew = createdNew
		out.Body.InviteSent = inviteSent
		return out, nil
	})

	type SiteMemberUserInput struct {
		SiteID uuid.UUID `path:"siteId" format:"uuid"`
		UserID uuid.UUID `path:"userId" format:"uuid"`
	}
	huma.Register(siteOpGroup, huma.Operation{
		OperationID:   "siteOperatorRemoveMember",
		Method:        http.MethodDelete,
		Path:          "/api/v1/iam/sites/{siteId}/operator/members/{userId}",
		Summary:       "Remove a SITE_MEMBER from the site (Site Operator)",
		Tags:          []string{"Site Operator"},
		Security:      []map[string][]string{{"BearerAuth": {}}},
		DefaultStatus: http.StatusNoContent,
	}, func(ctx context.Context, in *SiteMemberUserInput) (*struct{}, error) {
		claims, _ := auth.ClaimsFromContext(ctx)
		if err := svc.OperatorRemoveSiteMember(ctx, in.SiteID, in.UserID, claims.IsPlatformAdmin); err != nil {
			return nil, mapError(err)
		}
		return nil, nil
	})

	// =========================================================================
	// 4. Platform Admin Endpoints (/api/v1/iam/admin/*)
	//    Sole authority for sites, site features (WebView/VNC), WireGuard configs,
	//    global users, SITE_OPERATOR assignments, and tunnel runners.
	// =========================================================================
	platformGroup := huma.NewGroup(humaAPI)
	platformGroup.UseMiddleware(humaAuthMiddleware(humaAPI, verifier), humaPlatformAdminMiddleware(humaAPI))

	type ListUsersOutput struct {
		Body struct {
			Users []UserDTO `json:"users"`
		}
	}
	huma.Register(platformGroup, huma.Operation{
		OperationID: "adminListUsers",
		Method:      http.MethodGet,
		Path:        "/api/v1/iam/admin/users",
		Summary:     "List all platform users (Platform Admin)",
		Tags:        []string{"Platform Admin"},
		Security:    []map[string][]string{{"BearerAuth": {}}},
	}, func(ctx context.Context, _ *struct{}) (*ListUsersOutput, error) {
		users, err := svc.ListUsers(ctx)
		if err != nil {
			return nil, mapError(err)
		}
		out := &ListUsersOutput{}
		out.Body.Users = make([]UserDTO, len(users))
		for i, u := range users {
			out.Body.Users[i] = toUserDTO(u)
		}
		return out, nil
	})

	type CreateUserInput struct {
		Body struct {
			Email           string `json:"email" format:"email" required:"true"`
			IsPlatformAdmin bool   `json:"isPlatformAdmin,omitempty"`
			Memberships     []struct {
				SiteID uuid.UUID `json:"siteId" format:"uuid" required:"true"`
				Role   string    `json:"role" enum:"SITE_OPERATOR,SITE_MEMBER" required:"true"`
			} `json:"memberships,omitempty"`
		}
	}
	type CreateUserOutput struct {
		Body struct {
			User       UserDTO `json:"user"`
			InviteSent bool    `json:"inviteSent"`
		}
	}
	huma.Register(platformGroup, huma.Operation{
		OperationID:   "adminCreateUser",
		Method:        http.MethodPost,
		Path:          "/api/v1/iam/admin/users",
		Summary:       "Create a user and dispatch an email invite (Platform Admin)",
		Tags:          []string{"Platform Admin"},
		Security:      []map[string][]string{{"BearerAuth": {}}},
		DefaultStatus: http.StatusCreated,
	}, func(ctx context.Context, in *CreateUserInput) (*CreateUserOutput, error) {
		mems := make([]service.InitialMembership, len(in.Body.Memberships))
		for i, m := range in.Body.Memberships {
			mems[i] = service.InitialMembership{SiteID: m.SiteID, Role: m.Role}
		}
		u, sent, err := svc.CreateInvitedUser(ctx, in.Body.Email, in.Body.IsPlatformAdmin, mems)
		if err != nil {
			return nil, mapError(err)
		}
		out := &CreateUserOutput{}
		out.Body.User = toUserDTO(*u)
		out.Body.InviteSent = sent
		return out, nil
	})

	type UserIDInput struct {
		UserID uuid.UUID `path:"userId" format:"uuid"`
	}
	type ResendInviteOutput struct {
		Body struct {
			InviteSent bool `json:"inviteSent"`
		}
	}
	huma.Register(platformGroup, huma.Operation{
		OperationID: "adminResendInvite",
		Method:      http.MethodPost,
		Path:        "/api/v1/iam/admin/users/{userId}/resend-invite",
		Summary:     "Resend invitation email for an invited user (Platform Admin)",
		Tags:        []string{"Platform Admin"},
		Security:    []map[string][]string{{"BearerAuth": {}}},
	}, func(ctx context.Context, in *UserIDInput) (*ResendInviteOutput, error) {
		sent, err := svc.ResendInvite(ctx, in.UserID)
		if err != nil {
			return nil, mapError(err)
		}
		out := &ResendInviteOutput{}
		out.Body.InviteSent = sent
		return out, nil
	})

	type UpdateUserInput struct {
		UserID uuid.UUID `path:"userId" format:"uuid"`
		Body   struct {
			Status          *string `json:"status,omitempty" enum:"ACTIVE,DISABLED"`
			IsPlatformAdmin *bool   `json:"isPlatformAdmin,omitempty"`
		}
	}
	huma.Register(platformGroup, huma.Operation{
		OperationID: "adminUpdateUser",
		Method:      http.MethodPatch,
		Path:        "/api/v1/iam/admin/users/{userId}",
		Summary:     "Update user active status or platform admin role (Platform Admin)",
		Tags:        []string{"Platform Admin"},
		Security:    []map[string][]string{{"BearerAuth": {}}},
	}, func(ctx context.Context, in *UpdateUserInput) (*MeOutput, error) {
		u, err := svc.UpdateUser(ctx, in.UserID, in.Body.Status, in.Body.IsPlatformAdmin)
		if err != nil {
			return nil, mapError(err)
		}
		return &MeOutput{Body: toUserDTO(*u)}, nil
	})

	huma.Register(platformGroup, huma.Operation{
		OperationID:   "adminDeleteUser",
		Method:        http.MethodDelete,
		Path:          "/api/v1/iam/admin/users/{userId}",
		Summary:       "Delete a user (Platform Admin)",
		Tags:          []string{"Platform Admin"},
		Security:      []map[string][]string{{"BearerAuth": {}}},
		DefaultStatus: http.StatusNoContent,
	}, func(ctx context.Context, in *UserIDInput) (*struct{}, error) {
		if err := svc.DeleteUser(ctx, in.UserID); err != nil {
			return nil, mapError(err)
		}
		return nil, nil
	})

	huma.Register(platformGroup, huma.Operation{
		OperationID: "adminListSites",
		Method:      http.MethodGet,
		Path:        "/api/v1/iam/admin/sites",
		Summary:     "List all sites with WireGuard metadata (Platform Admin)",
		Tags:        []string{"Platform Admin"},
		Security:    []map[string][]string{{"BearerAuth": {}}},
	}, func(ctx context.Context, _ *struct{}) (*ListSitesOutput, error) {
		uid, _ := auth.UserIDFromContext(ctx)
		items, err := svc.ListAccessibleSites(ctx, uid, true)
		if err != nil {
			return nil, mapError(err)
		}
		out := &ListSitesOutput{}
		out.Body.Sites = make([]SiteDTO, len(items))
		for i, item := range items {
			out.Body.Sites[i] = toSiteDTO(item.Site, models.RoleSiteOperator, true)
		}
		return out, nil
	})

	type SiteWithWarningsOutput struct {
		Body struct {
			Site     SiteDTO  `json:"site"`
			Warnings []string `json:"warnings,omitempty"`
		}
	}
	type CreateSiteReq struct {
		Body struct {
			Name               string   `json:"name" minLength:"2" maxLength:"100" required:"true"`
			Slug               string   `json:"slug,omitempty" maxLength:"80"`
			WireguardConf      string   `json:"wireguardConf" minLength:"20" required:"true"`
			OverrideAllowedIPs []string `json:"overrideAllowedIPs,omitempty"`
			VncEnabled         bool     `json:"vncEnabled,omitempty"`
			WebviewEnabled     bool     `json:"webviewEnabled,omitempty"`
			Activate           bool     `json:"activate,omitempty"`
		}
	}
	huma.Register(platformGroup, huma.Operation{
		OperationID:   "adminCreateSite",
		Method:        http.MethodPost,
		Path:          "/api/v1/iam/admin/sites",
		Summary:       "Onboard a new site with validated WireGuard client config (Platform Admin)",
		Tags:          []string{"Platform Admin"},
		Security:      []map[string][]string{{"BearerAuth": {}}},
		DefaultStatus: http.StatusCreated,
	}, func(ctx context.Context, in *CreateSiteReq) (*SiteWithWarningsOutput, error) {
		site, warnings, err := svc.CreateSite(ctx, service.CreateSiteInput{
			Name:               in.Body.Name,
			Slug:               in.Body.Slug,
			WireguardConf:      in.Body.WireguardConf,
			OverrideAllowedIPs: in.Body.OverrideAllowedIPs,
			VncEnabled:         in.Body.VncEnabled,
			WebviewEnabled:     in.Body.WebviewEnabled,
			Activate:           in.Body.Activate,
		})
		if err != nil {
			return nil, mapError(err)
		}
		out := &SiteWithWarningsOutput{}
		out.Body.Site = toSiteDTO(*site, models.RoleSiteOperator, true)
		out.Body.Warnings = warnings
		return out, nil
	})

	type AdminUpdateSiteInput struct {
		SiteID uuid.UUID `path:"siteId" format:"uuid"`
		Body   struct {
			Name           *string `json:"name,omitempty" minLength:"2" maxLength:"100"`
			VncEnabled     *bool   `json:"vncEnabled,omitempty"`
			WebviewEnabled *bool   `json:"webviewEnabled,omitempty"`
		}
	}
	huma.Register(platformGroup, huma.Operation{
		OperationID: "adminUpdateSite",
		Method:      http.MethodPatch,
		Path:        "/api/v1/iam/admin/sites/{siteId}",
		Summary:     "Update site name and enabled features (WebView / WebVNC) (Platform Admin)",
		Tags:        []string{"Platform Admin"},
		Security:    []map[string][]string{{"BearerAuth": {}}},
	}, func(ctx context.Context, in *AdminUpdateSiteInput) (*SiteOutput, error) {
		site, err := svc.UpdateSiteSettings(ctx, in.SiteID, in.Body.Name, in.Body.VncEnabled, in.Body.WebviewEnabled)
		if err != nil {
			return nil, mapError(err)
		}
		return &SiteOutput{Body: toSiteDTO(*site, models.RoleSiteOperator, true)}, nil
	})

	type AdminUploadConfInput struct {
		SiteID uuid.UUID `path:"siteId" format:"uuid"`
		Body   struct {
			WireguardConf      string   `json:"wireguardConf" minLength:"20" required:"true"`
			OverrideAllowedIPs []string `json:"overrideAllowedIPs,omitempty"`
		}
	}
	huma.Register(platformGroup, huma.Operation{
		OperationID: "adminUploadSiteConf",
		Method:      http.MethodPost,
		Path:        "/api/v1/iam/admin/sites/{siteId}/conf",
		Summary:     "Validate, narrow, encrypt, and rotate a site's WireGuard client config (Platform Admin)",
		Tags:        []string{"Platform Admin"},
		Security:    []map[string][]string{{"BearerAuth": {}}},
	}, func(ctx context.Context, in *AdminUploadConfInput) (*SiteWithWarningsOutput, error) {
		site, warnings, err := svc.UpdateSiteConf(ctx, in.SiteID, in.Body.WireguardConf, in.Body.OverrideAllowedIPs)
		if err != nil {
			return nil, mapError(err)
		}
		out := &SiteWithWarningsOutput{}
		out.Body.Site = toSiteDTO(*site, models.RoleSiteOperator, true)
		out.Body.Warnings = warnings
		return out, nil
	})

	huma.Register(platformGroup, huma.Operation{
		OperationID: "adminActivateSite",
		Method:      http.MethodPost,
		Path:        "/api/v1/iam/admin/sites/{siteId}/activate",
		Summary:     "Activate a site and ensure its WireRelay runner is running (Platform Admin)",
		Tags:        []string{"Platform Admin"},
		Security:    []map[string][]string{{"BearerAuth": {}}},
	}, func(ctx context.Context, in *SiteIDInput) (*SiteOutput, error) {
		site, err := svc.SetSiteActive(ctx, in.SiteID, true)
		if err != nil {
			return nil, mapError(err)
		}
		return &SiteOutput{Body: toSiteDTO(*site, models.RoleSiteOperator, true)}, nil
	})

	huma.Register(platformGroup, huma.Operation{
		OperationID: "adminDeactivateSite",
		Method:      http.MethodPost,
		Path:        "/api/v1/iam/admin/sites/{siteId}/deactivate",
		Summary:     "Deactivate a site and stop its runner container (Platform Admin)",
		Tags:        []string{"Platform Admin"},
		Security:    []map[string][]string{{"BearerAuth": {}}},
	}, func(ctx context.Context, in *SiteIDInput) (*SiteOutput, error) {
		site, err := svc.SetSiteActive(ctx, in.SiteID, false)
		if err != nil {
			return nil, mapError(err)
		}
		return &SiteOutput{Body: toSiteDTO(*site, models.RoleSiteOperator, true)}, nil
	})

	huma.Register(platformGroup, huma.Operation{
		OperationID:   "adminDeleteSite",
		Method:        http.MethodDelete,
		Path:          "/api/v1/iam/admin/sites/{siteId}",
		Summary:       "Delete a site, remove its runner, and purge downstream data (Platform Admin)",
		Tags:          []string{"Platform Admin"},
		Security:      []map[string][]string{{"BearerAuth": {}}},
		DefaultStatus: http.StatusNoContent,
	}, func(ctx context.Context, in *SiteIDInput) (*struct{}, error) {
		if err := svc.DeleteSite(ctx, in.SiteID); err != nil {
			return nil, mapError(err)
		}
		return nil, nil
	})

	huma.Register(platformGroup, huma.Operation{
		OperationID: "adminListSiteMembers",
		Method:      http.MethodGet,
		Path:        "/api/v1/iam/admin/sites/{siteId}/members",
		Summary:     "List all members assigned to a site (Platform Admin)",
		Tags:        []string{"Platform Admin"},
		Security:    []map[string][]string{{"BearerAuth": {}}},
	}, func(ctx context.Context, in *SiteIDInput) (*ListMembersOutput, error) {
		members, err := svc.ListSiteMembers(ctx, in.SiteID)
		if err != nil {
			return nil, mapError(err)
		}
		out := &ListMembersOutput{}
		out.Body.Members = make([]SiteMemberDTO, 0, len(members))
		for _, m := range members {
			email := ""
			st := ""
			if m.User != nil {
				email = m.User.Email
				st = m.User.Status
			}
			out.Body.Members = append(out.Body.Members, SiteMemberDTO{
				UserID:    m.UserID,
				Email:     email,
				Status:    st,
				Role:      m.Role,
				CreatedAt: m.CreatedAt,
			})
		}
		return out, nil
	})

	type UpsertMemberInput struct {
		SiteID uuid.UUID `path:"siteId" format:"uuid"`
		UserID uuid.UUID `path:"userId" format:"uuid"`
		Body   struct {
			Role string `json:"role" enum:"SITE_OPERATOR,SITE_MEMBER" required:"true"`
		}
	}
	type MemberOutput struct {
		Body struct {
			SiteID uuid.UUID `json:"siteId"`
			UserID uuid.UUID `json:"userId"`
			Role   string    `json:"role"`
		}
	}
	huma.Register(platformGroup, huma.Operation{
		OperationID: "adminUpsertSiteMember",
		Method:      http.MethodPut,
		Path:        "/api/v1/iam/admin/sites/{siteId}/members/{userId}",
		Summary:     "Assign or update a user's site role (SITE_OPERATOR | SITE_MEMBER) (Platform Admin)",
		Tags:        []string{"Platform Admin"},
		Security:    []map[string][]string{{"BearerAuth": {}}},
	}, func(ctx context.Context, in *UpsertMemberInput) (*MemberOutput, error) {
		sm, err := svc.UpsertSiteMember(ctx, in.SiteID, in.UserID, in.Body.Role)
		if err != nil {
			return nil, mapError(err)
		}
		out := &MemberOutput{}
		out.Body.SiteID = sm.SiteID
		out.Body.UserID = sm.UserID
		out.Body.Role = sm.Role
		return out, nil
	})

	huma.Register(platformGroup, huma.Operation{
		OperationID:   "adminRemoveSiteMember",
		Method:        http.MethodDelete,
		Path:          "/api/v1/iam/admin/sites/{siteId}/members/{userId}",
		Summary:       "Remove any user's site membership (Platform Admin)",
		Tags:          []string{"Platform Admin"},
		Security:      []map[string][]string{{"BearerAuth": {}}},
		DefaultStatus: http.StatusNoContent,
	}, func(ctx context.Context, in *SiteMemberUserInput) (*struct{}, error) {
		if err := svc.RemoveSiteMember(ctx, in.SiteID, in.UserID); err != nil {
			return nil, mapError(err)
		}
		return nil, nil
	})

	type ListRunnersOutput struct {
		Body struct {
			Runners []internalclient.RunnerRecord `json:"runners"`
		}
	}
	huma.Register(platformGroup, huma.Operation{
		OperationID: "adminListRunners",
		Method:      http.MethodGet,
		Path:        "/api/v1/iam/admin/runners",
		Summary:     "Inspect all runner containers and handshake metrics (Platform Admin)",
		Tags:        []string{"Platform Admin"},
		Security:    []map[string][]string{{"BearerAuth": {}}},
	}, func(ctx context.Context, _ *struct{}) (*ListRunnersOutput, error) {
		runners, err := svc.ListRunners(ctx)
		if err != nil {
			return nil, mapError(err)
		}
		out := &ListRunnersOutput{}
		out.Body.Runners = runners
		return out, nil
	})

	huma.Register(platformGroup, huma.Operation{
		OperationID:   "adminRestartRunner",
		Method:        http.MethodPost,
		Path:          "/api/v1/iam/admin/runners/{siteId}/restart",
		Summary:       "Drain and restart a site's runner container (Platform Admin)",
		Tags:          []string{"Platform Admin"},
		Security:      []map[string][]string{{"BearerAuth": {}}},
		DefaultStatus: http.StatusNoContent,
	}, func(ctx context.Context, in *SiteIDInput) (*struct{}, error) {
		if err := svc.RestartRunner(ctx, in.SiteID); err != nil {
			return nil, mapError(err)
		}
		return nil, nil
	})

	return r
}

// --- Huma RBAC Group Middlewares ---

func humaAuthMiddleware(humaAPI huma.API, verifier *auth.Verifier) func(huma.Context, func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		h := ctx.Header("Authorization")
		if len(h) <= 7 || (h[:7] != "Bearer " && h[:7] != "bearer ") {
			_ = huma.WriteErr(humaAPI, ctx, http.StatusUnauthorized, "missing bearer access token")
			return
		}
		claims, err := verifier.Verify(ctx.Context(), h[7:])
		if err != nil {
			_ = huma.WriteErr(humaAPI, ctx, http.StatusUnauthorized, "invalid or expired access token")
			return
		}
		next(huma.WithContext(ctx, auth.WithClaims(ctx.Context(), claims)))
	}
}

func humaPlatformAdminMiddleware(humaAPI huma.API) func(huma.Context, func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		claims, ok := auth.ClaimsFromContext(ctx.Context())
		if !ok || !claims.IsPlatformAdmin {
			_ = huma.WriteErr(humaAPI, ctx, http.StatusForbidden, "platform administrator privileges required")
			return
		}
		next(ctx)
	}
}

func humaSiteOperatorMiddleware(humaAPI huma.API, svc *service.IAMService) func(huma.Context, func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		claims, ok := auth.ClaimsFromContext(ctx.Context())
		uid, uidOK := auth.UserIDFromContext(ctx.Context())
		if !ok || !uidOK {
			_ = huma.WriteErr(humaAPI, ctx, http.StatusUnauthorized, "unauthenticated")
			return
		}
		siteID, err := uuid.Parse(ctx.Param("siteId"))
		if err != nil {
			_ = huma.WriteErr(humaAPI, ctx, http.StatusBadRequest, "invalid siteId")
			return
		}
		role, allowed, err := svc.CheckSiteRole(ctx.Context(), siteID, uid, claims.IsPlatformAdmin)
		if err != nil || !allowed || role != models.RoleSiteOperator {
			_ = huma.WriteErr(humaAPI, ctx, http.StatusForbidden, "site operator privileges required")
			return
		}
		next(ctx)
	}
}

type catchAllErrorDetail struct {
	Code     string   `json:"code"`
	Detected []string `json:"detected"`
}

func (e catchAllErrorDetail) Error() string { return e.Code }

func mapError(err error) error {
	var catchAll *wireguard.CatchAllAllowedIPsError
	if errors.As(err, &catchAll) {
		return huma.Error422UnprocessableEntity(catchAll.Error(), catchAllErrorDetail{
			Code:     "CATCHALL_ALLOWED_IPS",
			Detected: catchAll.Detected,
		})
	}
	switch {
	case errors.Is(err, service.ErrInvalidCredential), errors.Is(err, service.ErrInvalidToken):
		return huma.Error401Unauthorized(err.Error())
	case errors.Is(err, service.ErrUserDisabled), errors.Is(err, service.ErrForbidden):
		return huma.Error403Forbidden(err.Error())
	case errors.Is(err, service.ErrNotFound):
		return huma.Error404NotFound(err.Error())
	case errors.Is(err, service.ErrConflict):
		return huma.Error409Conflict(err.Error())
	default:
		return huma.Error422UnprocessableEntity(err.Error())
	}
}

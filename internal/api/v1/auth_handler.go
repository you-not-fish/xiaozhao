// Package v1 holds HTTP handlers that map the REST surface to application
// services. Handlers must stay thin: parse + validate, call one service
// method, translate back to DTO, and let the response package emit JSON.
package v1

import (
	"context"
	"net/http"

	"github.com/xiaozhao/xiaozhao/internal/api/dto"
	"github.com/xiaozhao/xiaozhao/internal/api/response"
	"github.com/xiaozhao/xiaozhao/internal/app/auth"
	"github.com/xiaozhao/xiaozhao/internal/gateway/httpctx"
	"github.com/xiaozhao/xiaozhao/internal/pkg/errcode"
)

type AuthHandler struct {
	svc    *auth.Service
	orgSvc orgMembershipChecker
}

// orgMembershipChecker is the subset of org.Service that AuthHandler uses for
// org switching — declared locally so the API layer does not depend on the
// concrete service type.
type orgMembershipChecker interface {
	IsMember(ctx context.Context, orgID, userID string) error
}

func NewAuthHandler(svc *auth.Service, orgSvc orgMembershipChecker) *AuthHandler {
	return &AuthHandler{svc: svc, orgSvc: orgSvc}
}

func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req dto.RegisterRequest
	if err := decode(r, &req); err != nil {
		response.WriteError(r.Context(), w, err)
		return
	}
	u, tok, err := h.svc.Register(r.Context(), auth.RegisterInput{
		Email: req.Email, Password: req.Password, Name: req.Name,
	})
	if err != nil {
		response.WriteError(r.Context(), w, err)
		return
	}
	response.Write(r.Context(), w, http.StatusCreated, dto.AuthResponse{
		User:  dto.FromUser(u),
		Token: dto.BuildTokenResponse(tok.AccessToken, tok.ExpiresAt),
	})
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req dto.LoginRequest
	if err := decode(r, &req); err != nil {
		response.WriteError(r.Context(), w, err)
		return
	}
	u, tok, err := h.svc.Login(r.Context(), auth.LoginInput{Email: req.Email, Password: req.Password})
	if err != nil {
		response.WriteError(r.Context(), w, err)
		return
	}
	response.WriteOK(r.Context(), w, dto.AuthResponse{
		User:  dto.FromUser(u),
		Token: dto.BuildTokenResponse(tok.AccessToken, tok.ExpiresAt),
	})
}

func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	uid, err := httpctx.UserID(r.Context())
	if err != nil {
		response.WriteError(r.Context(), w, errcode.New(errcode.CodeUnauthorized, "authentication required"))
		return
	}
	u, err := h.svc.Me(r.Context(), uid)
	if err != nil {
		response.WriteError(r.Context(), w, err)
		return
	}
	response.WriteOK(r.Context(), w, dto.FromUser(u))
}

// SwitchOrg re-issues a token with the new active org.
func (h *AuthHandler) SwitchOrg(w http.ResponseWriter, r *http.Request) {
	uid, err := httpctx.UserID(r.Context())
	if err != nil {
		response.WriteError(r.Context(), w, errcode.New(errcode.CodeUnauthorized, "authentication required"))
		return
	}
	var req dto.SwitchOrgRequest
	if err := decode(r, &req); err != nil {
		response.WriteError(r.Context(), w, err)
		return
	}
	tok, err := h.svc.SwitchOrg(r.Context(), uid, req.OrgID, h.orgSvc.IsMember)
	if err != nil {
		response.WriteError(r.Context(), w, err)
		return
	}
	response.WriteOK(r.Context(), w, dto.BuildTokenResponse(tok.AccessToken, tok.ExpiresAt))
}

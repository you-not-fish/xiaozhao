package v1

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/xiaozhao/xiaozhao/internal/api/dto"
	"github.com/xiaozhao/xiaozhao/internal/api/response"
	orgapp "github.com/xiaozhao/xiaozhao/internal/app/org"
	"github.com/xiaozhao/xiaozhao/internal/domain"
	"github.com/xiaozhao/xiaozhao/internal/gateway/httpctx"
	"github.com/xiaozhao/xiaozhao/internal/pkg/errcode"
)

type OrgHandler struct {
	svc *orgapp.Service
}

func NewOrgHandler(svc *orgapp.Service) *OrgHandler { return &OrgHandler{svc: svc} }

func (h *OrgHandler) Create(w http.ResponseWriter, r *http.Request) {
	uid, err := httpctx.UserID(r.Context())
	if err != nil {
		response.WriteError(r.Context(), w, errcode.New(errcode.CodeUnauthorized, "authentication required"))
		return
	}
	var req dto.CreateOrgRequest
	if err := decode(r, &req); err != nil {
		response.WriteError(r.Context(), w, err)
		return
	}
	o, err := h.svc.Create(r.Context(), uid, req.Name, req.Plan)
	if err != nil {
		response.WriteError(r.Context(), w, err)
		return
	}
	response.Write(r.Context(), w, http.StatusCreated, dto.FromOrg(o))
}

func (h *OrgHandler) List(w http.ResponseWriter, r *http.Request) {
	uid, err := httpctx.UserID(r.Context())
	if err != nil {
		response.WriteError(r.Context(), w, errcode.New(errcode.CodeUnauthorized, "authentication required"))
		return
	}
	orgs, err := h.svc.ListForUser(r.Context(), uid)
	if err != nil {
		response.WriteError(r.Context(), w, err)
		return
	}
	items := make([]dto.OrgResponse, 0, len(orgs))
	for i := range orgs {
		items = append(items, dto.FromOrg(&orgs[i]))
	}
	response.WriteOK(r.Context(), w, dto.OrgListResponse{Items: items})
}

func (h *OrgHandler) Get(w http.ResponseWriter, r *http.Request) {
	uid, err := httpctx.UserID(r.Context())
	if err != nil {
		response.WriteError(r.Context(), w, errcode.New(errcode.CodeUnauthorized, "authentication required"))
		return
	}
	orgID := chi.URLParam(r, "orgID")
	o, err := h.svc.Get(r.Context(), uid, orgID)
	if err != nil {
		response.WriteError(r.Context(), w, err)
		return
	}
	response.WriteOK(r.Context(), w, dto.FromOrg(o))
}

func (h *OrgHandler) Rename(w http.ResponseWriter, r *http.Request) {
	uid, err := httpctx.UserID(r.Context())
	if err != nil {
		response.WriteError(r.Context(), w, errcode.New(errcode.CodeUnauthorized, "authentication required"))
		return
	}
	orgID := chi.URLParam(r, "orgID")
	var req dto.RenameOrgRequest
	if err := decode(r, &req); err != nil {
		response.WriteError(r.Context(), w, err)
		return
	}
	o, err := h.svc.Rename(r.Context(), uid, orgID, req.Name)
	if err != nil {
		response.WriteError(r.Context(), w, err)
		return
	}
	response.WriteOK(r.Context(), w, dto.FromOrg(o))
}

func (h *OrgHandler) ListMembers(w http.ResponseWriter, r *http.Request) {
	uid, err := httpctx.UserID(r.Context())
	if err != nil {
		response.WriteError(r.Context(), w, errcode.New(errcode.CodeUnauthorized, "authentication required"))
		return
	}
	orgID := chi.URLParam(r, "orgID")
	members, err := h.svc.ListMembers(r.Context(), uid, orgID)
	if err != nil {
		response.WriteError(r.Context(), w, err)
		return
	}
	items := make([]dto.MemberResponse, 0, len(members))
	for i := range members {
		items = append(items, dto.FromMember(&members[i]))
	}
	response.WriteOK(r.Context(), w, dto.MemberListResponse{Items: items})
}

func (h *OrgHandler) UpsertMember(w http.ResponseWriter, r *http.Request) {
	uid, err := httpctx.UserID(r.Context())
	if err != nil {
		response.WriteError(r.Context(), w, errcode.New(errcode.CodeUnauthorized, "authentication required"))
		return
	}
	orgID := chi.URLParam(r, "orgID")
	var req dto.UpsertMemberRequest
	if err := decode(r, &req); err != nil {
		response.WriteError(r.Context(), w, err)
		return
	}
	m, err := h.svc.InviteOrUpdateMember(r.Context(), uid, orgID, req.UserID, domain.Role(req.Role))
	if err != nil {
		response.WriteError(r.Context(), w, err)
		return
	}
	response.WriteOK(r.Context(), w, dto.FromMember(m))
}

func (h *OrgHandler) RemoveMember(w http.ResponseWriter, r *http.Request) {
	uid, err := httpctx.UserID(r.Context())
	if err != nil {
		response.WriteError(r.Context(), w, errcode.New(errcode.CodeUnauthorized, "authentication required"))
		return
	}
	orgID := chi.URLParam(r, "orgID")
	targetID := chi.URLParam(r, "userID")
	if err := h.svc.RemoveMember(r.Context(), uid, orgID, targetID); err != nil {
		response.WriteError(r.Context(), w, err)
		return
	}
	response.WriteOK(r.Context(), w, map[string]any{"removed": true})
}

package v1

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/xiaozhao/xiaozhao/internal/api/dto"
	"github.com/xiaozhao/xiaozhao/internal/api/response"
	projectapp "github.com/xiaozhao/xiaozhao/internal/app/project"
	"github.com/xiaozhao/xiaozhao/internal/gateway/httpctx"
	"github.com/xiaozhao/xiaozhao/internal/pkg/errcode"
)

type ProjectHandler struct {
	svc *projectapp.Service
}

func NewProjectHandler(svc *projectapp.Service) *ProjectHandler {
	return &ProjectHandler{svc: svc}
}

// Create expects the tenant middleware to have resolved an active org.
func (h *ProjectHandler) Create(w http.ResponseWriter, r *http.Request) {
	uid, err := httpctx.UserID(r.Context())
	if err != nil {
		response.WriteError(r.Context(), w, errcode.New(errcode.CodeUnauthorized, "authentication required"))
		return
	}
	orgID, err := httpctx.OrgID(r.Context())
	if err != nil {
		response.WriteError(r.Context(), w, errcode.New(errcode.CodeInvalidArgument, "organization context is required"))
		return
	}
	var req dto.CreateProjectRequest
	if err := decode(r, &req); err != nil {
		response.WriteError(r.Context(), w, err)
		return
	}
	p, err := h.svc.Create(r.Context(), uid, projectapp.CreateInput{
		OrgID:    orgID,
		Name:     req.Name,
		Settings: req.Settings,
	})
	if err != nil {
		response.WriteError(r.Context(), w, err)
		return
	}
	response.Write(r.Context(), w, http.StatusCreated, dto.FromProject(p))
}

func (h *ProjectHandler) List(w http.ResponseWriter, r *http.Request) {
	uid, err := httpctx.UserID(r.Context())
	if err != nil {
		response.WriteError(r.Context(), w, errcode.New(errcode.CodeUnauthorized, "authentication required"))
		return
	}
	orgID, err := httpctx.OrgID(r.Context())
	if err != nil {
		response.WriteError(r.Context(), w, errcode.New(errcode.CodeInvalidArgument, "organization context is required"))
		return
	}
	ps, err := h.svc.List(r.Context(), uid, orgID)
	if err != nil {
		response.WriteError(r.Context(), w, err)
		return
	}
	items := make([]dto.ProjectResponse, 0, len(ps))
	for i := range ps {
		items = append(items, dto.FromProject(&ps[i]))
	}
	response.WriteOK(r.Context(), w, dto.ProjectListResponse{Items: items})
}

func (h *ProjectHandler) Get(w http.ResponseWriter, r *http.Request) {
	uid, err := httpctx.UserID(r.Context())
	if err != nil {
		response.WriteError(r.Context(), w, errcode.New(errcode.CodeUnauthorized, "authentication required"))
		return
	}
	projectID := chi.URLParam(r, "projectID")
	p, err := h.svc.Get(r.Context(), uid, projectID)
	if err != nil {
		response.WriteError(r.Context(), w, err)
		return
	}
	response.WriteOK(r.Context(), w, dto.FromProject(p))
}

func (h *ProjectHandler) Update(w http.ResponseWriter, r *http.Request) {
	uid, err := httpctx.UserID(r.Context())
	if err != nil {
		response.WriteError(r.Context(), w, errcode.New(errcode.CodeUnauthorized, "authentication required"))
		return
	}
	projectID := chi.URLParam(r, "projectID")
	var req dto.UpdateProjectRequest
	if err := decode(r, &req); err != nil {
		response.WriteError(r.Context(), w, err)
		return
	}
	p, err := h.svc.Update(r.Context(), uid, projectID, projectapp.UpdateInput{
		Name:     req.Name,
		Settings: req.Settings,
	})
	if err != nil {
		response.WriteError(r.Context(), w, err)
		return
	}
	response.WriteOK(r.Context(), w, dto.FromProject(p))
}

func (h *ProjectHandler) Delete(w http.ResponseWriter, r *http.Request) {
	uid, err := httpctx.UserID(r.Context())
	if err != nil {
		response.WriteError(r.Context(), w, errcode.New(errcode.CodeUnauthorized, "authentication required"))
		return
	}
	projectID := chi.URLParam(r, "projectID")
	if err := h.svc.Delete(r.Context(), uid, projectID); err != nil {
		response.WriteError(r.Context(), w, err)
		return
	}
	response.WriteOK(r.Context(), w, map[string]any{"deleted": true})
}

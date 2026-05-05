package v1

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/xiaozhao/xiaozhao/internal/api/dto"
	"github.com/xiaozhao/xiaozhao/internal/api/response"
	knowledgeapp "github.com/xiaozhao/xiaozhao/internal/app/knowledge"
	"github.com/xiaozhao/xiaozhao/internal/pkg/errcode"
)

type KnowledgeHandler struct {
	svc *knowledgeapp.Service
}

func NewKnowledgeHandler(svc *knowledgeapp.Service) *KnowledgeHandler {
	return &KnowledgeHandler{svc: svc}
}

func (h *KnowledgeHandler) Create(w http.ResponseWriter, r *http.Request) {
	uid, orgID, ok := requireUserOrg(w, r)
	if !ok {
		return
	}
	var req dto.CreateKnowledgeBaseRequest
	if err := decode(r, &req); err != nil {
		response.WriteError(r.Context(), w, err)
		return
	}
	kb, err := h.svc.CreateKnowledgeBase(r.Context(), knowledgeapp.CreateKnowledgeBaseInput{
		OrgID: orgID, ProjectID: req.ProjectID, UserID: uid, Name: req.Name, Description: req.Description,
	})
	if err != nil {
		response.WriteError(r.Context(), w, err)
		return
	}
	response.Write(r.Context(), w, http.StatusCreated, dto.FromKnowledgeBase(kb))
}

func (h *KnowledgeHandler) List(w http.ResponseWriter, r *http.Request) {
	uid, orgID, ok := requireUserOrg(w, r)
	if !ok {
		return
	}
	projectID := r.URL.Query().Get("project_id")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	page, err := h.svc.ListKnowledgeBases(r.Context(), orgID, projectID, uid, limit, r.URL.Query().Get("cursor"))
	if err != nil {
		response.WriteError(r.Context(), w, err)
		return
	}
	items := make([]dto.KnowledgeBaseResponse, 0, len(page.Items))
	for i := range page.Items {
		items = append(items, dto.FromKnowledgeBase(&page.Items[i]))
	}
	response.WriteOK(r.Context(), w, dto.KnowledgeBaseListResponse{Items: items, NextCursor: page.NextCursor})
}

func (h *KnowledgeHandler) Get(w http.ResponseWriter, r *http.Request) {
	uid, orgID, ok := requireUserOrg(w, r)
	if !ok {
		return
	}
	kb, err := h.svc.GetKnowledgeBase(r.Context(), orgID, uid, chi.URLParam(r, "kbID"))
	if err != nil {
		response.WriteError(r.Context(), w, err)
		return
	}
	response.WriteOK(r.Context(), w, dto.FromKnowledgeBase(kb))
}

func (h *KnowledgeHandler) Delete(w http.ResponseWriter, r *http.Request) {
	uid, orgID, ok := requireUserOrg(w, r)
	if !ok {
		return
	}
	if err := h.svc.DeleteKnowledgeBase(r.Context(), orgID, uid, chi.URLParam(r, "kbID")); err != nil {
		response.WriteError(r.Context(), w, err)
		return
	}
	response.WriteOK(r.Context(), w, map[string]any{"deleted": true})
}

func (h *KnowledgeHandler) AddDocument(w http.ResponseWriter, r *http.Request) {
	uid, orgID, ok := requireUserOrg(w, r)
	if !ok {
		return
	}
	var req dto.AddDocumentRequest
	if err := decode(r, &req); err != nil {
		response.WriteError(r.Context(), w, err)
		return
	}
	if req.FileID == "" {
		response.WriteError(r.Context(), w, errcode.New(errcode.CodeInvalidArgument, "file_id is required"))
		return
	}
	doc, err := h.svc.AddDocument(r.Context(), orgID, uid, chi.URLParam(r, "kbID"), req.FileID)
	if err != nil {
		response.WriteError(r.Context(), w, err)
		return
	}
	response.Write(r.Context(), w, http.StatusCreated, dto.FromDocument(doc))
}

func (h *KnowledgeHandler) ListDocuments(w http.ResponseWriter, r *http.Request) {
	uid, orgID, ok := requireUserOrg(w, r)
	if !ok {
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	page, err := h.svc.ListDocuments(r.Context(), orgID, uid, chi.URLParam(r, "kbID"), limit, r.URL.Query().Get("cursor"))
	if err != nil {
		response.WriteError(r.Context(), w, err)
		return
	}
	items := make([]dto.DocumentResponse, 0, len(page.Items))
	for i := range page.Items {
		items = append(items, dto.FromDocument(&page.Items[i]))
	}
	response.WriteOK(r.Context(), w, dto.DocumentListResponse{Items: items, NextCursor: page.NextCursor})
}

func (h *KnowledgeHandler) DeleteDocument(w http.ResponseWriter, r *http.Request) {
	uid, orgID, ok := requireUserOrg(w, r)
	if !ok {
		return
	}
	if err := h.svc.DeleteDocument(r.Context(), orgID, uid, chi.URLParam(r, "kbID"), chi.URLParam(r, "docID")); err != nil {
		response.WriteError(r.Context(), w, err)
		return
	}
	response.WriteOK(r.Context(), w, map[string]any{"deleted": true})
}

func (h *KnowledgeHandler) Search(w http.ResponseWriter, r *http.Request) {
	uid, orgID, ok := requireUserOrg(w, r)
	if !ok {
		return
	}
	var req dto.KnowledgeSearchRequest
	if err := decode(r, &req); err != nil {
		response.WriteError(r.Context(), w, err)
		return
	}
	kb, err := h.svc.GetKnowledgeBase(r.Context(), orgID, uid, chi.URLParam(r, "kbID"))
	if err != nil {
		response.WriteError(r.Context(), w, err)
		return
	}
	results, err := h.svc.Search(r.Context(), knowledgeapp.SearchInput{
		OrgID: orgID, ProjectID: kb.ProjectID, UserID: uid, KnowledgeBaseIDs: []string{kb.ID},
		DocumentIDs: req.DocumentIDs, Query: req.Query, TopK: req.TopK, ScoreThreshold: req.ScoreThreshold,
	})
	if err != nil {
		response.WriteError(r.Context(), w, err)
		return
	}
	items := make([]dto.KnowledgeSearchResultResponse, 0, len(results))
	for _, res := range results {
		items = append(items, dto.FromKnowledgeSearchResult(res))
	}
	response.WriteOK(r.Context(), w, dto.KnowledgeSearchResponse{Items: items})
}

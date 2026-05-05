package v1

import (
	"io"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/xiaozhao/xiaozhao/internal/api/dto"
	"github.com/xiaozhao/xiaozhao/internal/api/response"
	fileapp "github.com/xiaozhao/xiaozhao/internal/app/file"
	"github.com/xiaozhao/xiaozhao/internal/domain"
	"github.com/xiaozhao/xiaozhao/internal/gateway/httpctx"
	"github.com/xiaozhao/xiaozhao/internal/pkg/errcode"
)

type FileHandler struct {
	svc            *fileapp.Service
	maxUploadBytes int64
}

func NewFileHandler(svc *fileapp.Service, maxUploadBytes int64) *FileHandler {
	return &FileHandler{svc: svc, maxUploadBytes: maxUploadBytes}
}

func (h *FileHandler) Upload(w http.ResponseWriter, r *http.Request) {
	uid, orgID, ok := requireUserOrg(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, h.maxUploadBytes+1024*1024)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		response.WriteError(r.Context(), w, errcode.Wrap(err, errcode.CodeInvalidArgument, "invalid multipart form"))
		return
	}
	projectID := r.FormValue("project_id")
	purpose := domain.FilePurpose(r.FormValue("purpose"))
	file, header, err := r.FormFile("file")
	if err != nil {
		response.WriteError(r.Context(), w, errcode.New(errcode.CodeInvalidArgument, "file is required"))
		return
	}
	defer file.Close()

	f, err := h.svc.Upload(r.Context(), fileapp.UploadInput{
		OrgID:      orgID,
		ProjectID:  projectID,
		UserID:     uid,
		Filename:   header.Filename,
		HeaderMIME: header.Header.Get("Content-Type"),
		SizeBytes:  header.Size,
		Purpose:    purpose,
		Body:       file,
	})
	if err != nil {
		response.WriteError(r.Context(), w, err)
		return
	}
	response.Write(r.Context(), w, http.StatusCreated, dto.FromFile(f))
}

func (h *FileHandler) List(w http.ResponseWriter, r *http.Request) {
	uid, orgID, ok := requireUserOrg(w, r)
	if !ok {
		return
	}
	projectID := r.URL.Query().Get("project_id")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	files, err := h.svc.List(r.Context(), orgID, projectID, uid, limit)
	if err != nil {
		response.WriteError(r.Context(), w, err)
		return
	}
	items := make([]dto.FileResponse, 0, len(files))
	for i := range files {
		items = append(items, dto.FromFile(&files[i]))
	}
	response.WriteOK(r.Context(), w, dto.FileListResponse{Items: items})
}

func (h *FileHandler) Get(w http.ResponseWriter, r *http.Request) {
	uid, orgID, ok := requireUserOrg(w, r)
	if !ok {
		return
	}
	f, err := h.svc.Get(r.Context(), orgID, uid, chi.URLParam(r, "fileID"))
	if err != nil {
		response.WriteError(r.Context(), w, err)
		return
	}
	response.WriteOK(r.Context(), w, dto.FromFile(f))
}

func (h *FileHandler) Content(w http.ResponseWriter, r *http.Request) {
	uid, orgID, ok := requireUserOrg(w, r)
	if !ok {
		return
	}
	out, err := h.svc.Download(r.Context(), orgID, uid, chi.URLParam(r, "fileID"))
	if err != nil {
		response.WriteError(r.Context(), w, err)
		return
	}
	defer out.Body.Close()
	ct := out.ContentType
	if ct == "" {
		ct = out.File.MimeType
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Content-Disposition", `attachment; filename="`+out.File.Filename+`"`)
	if out.Size > 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(out.Size, 10))
	}
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, out.Body)
}

func (h *FileHandler) Delete(w http.ResponseWriter, r *http.Request) {
	uid, orgID, ok := requireUserOrg(w, r)
	if !ok {
		return
	}
	if err := h.svc.Delete(r.Context(), orgID, uid, chi.URLParam(r, "fileID")); err != nil {
		response.WriteError(r.Context(), w, err)
		return
	}
	response.WriteOK(r.Context(), w, map[string]any{"deleted": true})
}

func requireUserOrg(w http.ResponseWriter, r *http.Request) (string, string, bool) {
	uid, err := httpctx.UserID(r.Context())
	if err != nil {
		response.WriteError(r.Context(), w, errcode.New(errcode.CodeUnauthorized, "authentication required"))
		return "", "", false
	}
	orgID, err := httpctx.OrgID(r.Context())
	if err != nil {
		response.WriteError(r.Context(), w, errcode.New(errcode.CodeInvalidArgument, "organization context is required"))
		return "", "", false
	}
	return uid, orgID, true
}

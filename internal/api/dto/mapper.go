package dto

import (
	"time"

	"github.com/xiaozhao/xiaozhao/internal/domain"
)

// FromUser converts a domain user into the API DTO. We deliberately strip
// password-related fields at this boundary so an accidental leak via handler
// refactoring cannot happen later.
func FromUser(u *domain.User) UserResponse {
	return UserResponse{
		ID:        u.ID,
		Email:     u.Email,
		Name:      u.Name,
		Status:    string(u.Status),
		CreatedAt: u.CreatedAt,
	}
}

// FromOrg converts a domain org into the API DTO.
func FromOrg(o *domain.Organization) OrgResponse {
	return OrgResponse{
		ID:        o.ID,
		Name:      o.Name,
		Plan:      o.Plan,
		Status:    string(o.Status),
		CreatedBy: o.CreatedBy,
		CreatedAt: o.CreatedAt,
	}
}

// FromMember converts a domain membership into the API DTO.
func FromMember(m *domain.OrgMember) MemberResponse {
	return MemberResponse{
		OrgID:    m.OrgID,
		UserID:   m.UserID,
		Role:     string(m.Role),
		Status:   string(m.Status),
		JoinedAt: m.JoinedAt,
	}
}

// FromProject converts a domain project into the API DTO.
func FromProject(p *domain.Project) ProjectResponse {
	return ProjectResponse{
		ID:         p.ID,
		OrgID:      p.OrgID,
		Name:       p.Name,
		Settings:   p.Settings,
		Visibility: string(p.Visibility),
		CreatedBy:  p.CreatedBy,
		CreatedAt:  p.CreatedAt,
	}
}

// FromResponseEvent converts a persisted event into the public replay DTO.
func FromResponseEvent(e domain.ResponseEvent) ResponseEventResponse {
	data := e.Data
	if len(data) == 0 {
		data = []byte(`{}`)
	}
	return ResponseEventResponse{
		ID:             e.ID,
		ResponseID:     e.ResponseID,
		ConversationID: e.ConversationID,
		Seq:            e.Seq,
		Type:           string(e.Type),
		Data:           data,
		CreatedAt:      e.CreatedAt,
	}
}

func FromFile(f *domain.File) FileResponse {
	return FileResponse{
		ID:        f.ID,
		OrgID:     f.OrgID,
		ProjectID: f.ProjectID,
		Filename:  f.Filename,
		MimeType:  f.MimeType,
		SizeBytes: f.SizeBytes,
		SHA256:    f.SHA256,
		Purpose:   string(f.Purpose),
		Status:    string(f.Status),
		CreatedAt: f.CreatedAt,
	}
}

func FromKnowledgeBase(kb *domain.KnowledgeBase) KnowledgeBaseResponse {
	return KnowledgeBaseResponse{
		ID:              kb.ID,
		OrgID:           kb.OrgID,
		ProjectID:       kb.ProjectID,
		Name:            kb.Name,
		Description:     kb.Description,
		Visibility:      string(kb.Visibility),
		EmbeddingModel:  kb.EmbeddingModel,
		EmbeddingDim:    kb.EmbeddingDim,
		ChunkConfig:     kb.ChunkConfig,
		RetrievalConfig: kb.RetrievalConfig,
		Status:          string(kb.Status),
		CreatedAt:       kb.CreatedAt,
	}
}

func FromDocument(doc *domain.Document) DocumentResponse {
	return DocumentResponse{
		ID:              doc.ID,
		OrgID:           doc.OrgID,
		ProjectID:       doc.ProjectID,
		KnowledgeBaseID: doc.KnowledgeBaseID,
		FileID:          doc.FileID,
		Filename:        doc.Filename,
		MimeType:        doc.MimeType,
		Status:          string(doc.Status),
		ParseError:      doc.ParseError,
		ChunkCount:      doc.ChunkCount,
		CharCount:       doc.CharCount,
		TokenCount:      doc.TokenCount,
		CreatedAt:       doc.CreatedAt,
	}
}

func FromKnowledgeSearchResult(r domain.KnowledgeSearchResult) KnowledgeSearchResultResponse {
	return KnowledgeSearchResultResponse{
		ChunkID:         r.ChunkID,
		DocumentID:      r.DocumentID,
		FileID:          r.FileID,
		Filename:        r.Filename,
		KnowledgeBaseID: r.KnowledgeBaseID,
		Content:         r.Content,
		Score:           r.Score,
		Citation: map[string]any{
			"file_id":     r.FileID,
			"filename":    r.Filename,
			"document_id": r.DocumentID,
			"chunk_id":    r.ChunkID,
			"metadata":    r.Metadata,
		},
	}
}

// BuildTokenResponse wraps an access token pair.
func BuildTokenResponse(accessToken string, expiresAt time.Time) TokenResponse {
	return TokenResponse{
		AccessToken: accessToken,
		TokenType:   "Bearer",
		ExpiresAt:   expiresAt,
	}
}

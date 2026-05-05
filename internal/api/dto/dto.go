// Package dto holds the JSON-facing request and response types. Keeping them
// separate from domain entities means the API surface can evolve without
// dragging the domain along (e.g. adding a password_updated_at field to the
// persistence model does not leak into responses).
package dto

import (
	"encoding/json"
	"time"
)

// --- Auth ---

type RegisterRequest struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required,min=6,max=128"`
	Name     string `json:"name" validate:"max=128"`
}

type LoginRequest struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required"`
}

type TokenResponse struct {
	AccessToken string    `json:"access_token"`
	TokenType   string    `json:"token_type"`
	ExpiresAt   time.Time `json:"expires_at"`
}

type AuthResponse struct {
	User  UserResponse  `json:"user"`
	Token TokenResponse `json:"token"`
}

type UserResponse struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

type SwitchOrgRequest struct {
	OrgID string `json:"org_id" validate:"required"`
}

// --- Organizations ---

type CreateOrgRequest struct {
	Name string `json:"name" validate:"required,max=128"`
	Plan string `json:"plan" validate:"max=64"`
}

type RenameOrgRequest struct {
	Name string `json:"name" validate:"required,max=128"`
}

type OrgResponse struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Plan      string    `json:"plan"`
	Status    string    `json:"status"`
	CreatedBy string    `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
}

type OrgListResponse struct {
	Items []OrgResponse `json:"items"`
}

type MemberResponse struct {
	OrgID    string    `json:"org_id"`
	UserID   string    `json:"user_id"`
	Role     string    `json:"role"`
	Status   string    `json:"status"`
	JoinedAt time.Time `json:"joined_at"`
}

type MemberListResponse struct {
	Items []MemberResponse `json:"items"`
}

type UpsertMemberRequest struct {
	UserID string `json:"user_id" validate:"required"`
	Role   string `json:"role" validate:"required,oneof=admin member viewer"`
}

// --- Projects ---

type CreateProjectRequest struct {
	Name     string         `json:"name" validate:"required,max=128"`
	Settings map[string]any `json:"settings"`
}

type UpdateProjectRequest struct {
	Name     *string         `json:"name"`
	Settings *map[string]any `json:"settings"`
}

type ProjectResponse struct {
	ID         string         `json:"id"`
	OrgID      string         `json:"org_id"`
	Name       string         `json:"name"`
	Settings   map[string]any `json:"settings"`
	Visibility string         `json:"visibility"`
	CreatedBy  string         `json:"created_by"`
	CreatedAt  time.Time      `json:"created_at"`
}

type ProjectListResponse struct {
	Items []ProjectResponse `json:"items"`
}

// --- Agent Responses ---

type ResponseToolRequest struct {
	Type             string   `json:"type" validate:"required"`
	KnowledgeBaseIDs []string `json:"knowledge_base_ids,omitempty"`
}

type CreateResponseRequest struct {
	ProjectID      string                `json:"project_id" validate:"required"`
	ConversationID string                `json:"conversation_id,omitempty"`
	Model          string                `json:"model,omitempty"`
	Input          string                `json:"input" validate:"required"`
	Tools          []ResponseToolRequest `json:"tools,omitempty"`
	Stream         *bool                 `json:"stream,omitempty"`
}

type ResponseCreatedResponse struct {
	ResponseID     string `json:"response_id"`
	ConversationID string `json:"conversation_id"`
	MessageID      string `json:"message_id"`
}

type ResponseEventResponse struct {
	ID             string          `json:"id"`
	ResponseID     string          `json:"response_id"`
	ConversationID string          `json:"conversation_id"`
	Seq            int             `json:"seq"`
	Type           string          `json:"type"`
	Data           json.RawMessage `json:"data"`
	CreatedAt      time.Time       `json:"created_at"`
}

type ResponseEventListResponse struct {
	Items []ResponseEventResponse `json:"items"`
}

// --- Files ---

type FileResponse struct {
	ID        string    `json:"id"`
	OrgID     string    `json:"org_id"`
	ProjectID string    `json:"project_id"`
	Filename  string    `json:"filename"`
	MimeType  string    `json:"mime_type"`
	SizeBytes int64     `json:"size_bytes"`
	SHA256    string    `json:"sha256"`
	Purpose   string    `json:"purpose"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

type FileListResponse struct {
	Items []FileResponse `json:"items"`
}

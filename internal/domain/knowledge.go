package domain

import "time"

type KnowledgeBaseStatus string

const (
	KnowledgeBaseStatusActive  KnowledgeBaseStatus = "active"
	KnowledgeBaseStatusDeleted KnowledgeBaseStatus = "deleted"
)

type KnowledgeBaseVisibility string

const (
	KnowledgeBaseVisibilityProject KnowledgeBaseVisibility = "project"
)

type KnowledgeBase struct {
	ID              string
	OrgID           string
	ProjectID       string
	Name            string
	Description     string
	Visibility      KnowledgeBaseVisibility
	AccessPolicy    map[string]any
	EmbeddingModel  string
	EmbeddingDim    int
	ChunkConfig     map[string]any
	RetrievalConfig map[string]any
	Status          KnowledgeBaseStatus
	CreatedBy       string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type DocumentStatus string

const (
	DocumentStatusPending    DocumentStatus = "pending"
	DocumentStatusProcessing DocumentStatus = "processing"
	DocumentStatusReady      DocumentStatus = "ready"
	DocumentStatusFailed     DocumentStatus = "failed"
	DocumentStatusDeleted    DocumentStatus = "deleted"
)

type Document struct {
	ID                  string
	OrgID               string
	ProjectID           string
	KnowledgeBaseID     string
	FileID              string
	Filename            string
	MimeType            string
	Status              DocumentStatus
	ParseError          string
	Attempts            int
	ProcessingUpdatedAt time.Time
	ChunkCount          int
	CharCount           int
	TokenCount          int
	CreatedBy           string
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

type DocumentChunkStatus string

const (
	DocumentChunkStatusReady   DocumentChunkStatus = "ready"
	DocumentChunkStatusDeleted DocumentChunkStatus = "deleted"
)

type DocumentChunk struct {
	ID              string
	OrgID           string
	ProjectID       string
	KnowledgeBaseID string
	DocumentID      string
	FileID          string
	ChunkIndex      int
	Content         string
	Metadata        map[string]any
	Embedding       []float32
	Status          DocumentChunkStatus
	CreatedAt       time.Time
}

type KnowledgeSearchResult struct {
	ChunkID         string
	DocumentID      string
	FileID          string
	Filename        string
	KnowledgeBaseID string
	Content         string
	Score           float64
	Metadata        map[string]any
}

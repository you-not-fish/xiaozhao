package domain

import "context"

type KnowledgeBaseRepository interface {
	Create(ctx context.Context, kb *KnowledgeBase) error
	Update(ctx context.Context, kb *KnowledgeBase) error
	GetByID(ctx context.Context, id string) (*KnowledgeBase, error)
	ListByProject(ctx context.Context, orgID, projectID string, limit int, cursor string) ([]KnowledgeBase, error)
}

type DocumentRepository interface {
	Create(ctx context.Context, doc *Document) error
	Update(ctx context.Context, doc *Document) error
	GetByID(ctx context.Context, id string) (*Document, error)
	ListByKnowledgeBase(ctx context.Context, orgID, knowledgeBaseID string, limit int, cursor string) ([]Document, error)
	ListPending(ctx context.Context, limit int) ([]Document, error)
	DeleteChunks(ctx context.Context, documentID string) error
}

type DocumentChunkRepository interface {
	CreateBatch(ctx context.Context, chunks []DocumentChunk) error
	ReplaceByDocument(ctx context.Context, documentID string, chunks []DocumentChunk) error
	Search(ctx context.Context, in ChunkSearchInput) ([]KnowledgeSearchResult, error)
}

type ChunkSearchInput struct {
	OrgID            string
	ProjectID        string
	KnowledgeBaseIDs []string
	DocumentIDs      []string
	Query            string
	Embedding        []float32
	TopK             int
	ScoreThreshold   float64
}

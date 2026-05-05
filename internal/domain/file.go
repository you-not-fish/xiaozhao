package domain

import "time"

type FilePurpose string

const (
	FilePurposeKnowledgeBase FilePurpose = "knowledge_base"
	FilePurposeConversation  FilePurpose = "conversation"
	FilePurposeUserData      FilePurpose = "user_data"
)

func (p FilePurpose) Valid() bool {
	switch p {
	case FilePurposeKnowledgeBase, FilePurposeConversation, FilePurposeUserData:
		return true
	default:
		return false
	}
}

type FileStatus string

const (
	FileStatusUploading FileStatus = "uploading"
	FileStatusUploaded  FileStatus = "uploaded"
	FileStatusFailed    FileStatus = "failed"
	FileStatusDeleted   FileStatus = "deleted"
)

type File struct {
	ID        string
	OrgID     string
	ProjectID string
	UserID    string
	Filename  string
	MimeType  string
	SizeBytes int64
	SHA256    string
	Purpose   FilePurpose
	Status    FileStatus
	CreatedAt time.Time
	UpdatedAt time.Time
}

type FileObject struct {
	ID              string
	FileID          string
	StorageProvider string
	Bucket          string
	ObjectKey       string
	ETag            string
	VersionID       string
	CreatedAt       time.Time
}

package agent

import (
	"context"
	"errors"
	"strings"

	"github.com/xiaozhao/xiaozhao/internal/app/rbac"
	"github.com/xiaozhao/xiaozhao/internal/domain"
	"github.com/xiaozhao/xiaozhao/internal/pkg/errcode"
	"github.com/xiaozhao/xiaozhao/internal/pkg/id"
)

// ConversationService 负责会话生命周期：创建、获取、列表、改名、删除。
// /v1/responses 入口允许调用方"不传 conversation_id"，由本服务在 EnsureConversation
// 中按需创建一条新会话——既保留了上下文延续能力，也降低了客户端复杂度。
type ConversationService struct {
	convs    domain.ConversationRepository
	projects domain.ProjectRepository
	rbac     *rbac.Checker
}

// NewConversationService 构造。
func NewConversationService(
	convs domain.ConversationRepository,
	projects domain.ProjectRepository,
	rbacChecker *rbac.Checker,
) *ConversationService {
	return &ConversationService{convs: convs, projects: projects, rbac: rbacChecker}
}

// EnsureInput 是"按需取/建会话"的入参。
type EnsureInput struct {
	OrgID          string
	UserID         string
	ProjectID      string
	ConversationID string // 可空：为空时自动创建一条新会话
	Title          string // 仅在新建会话时用作初始标题，可空
}

// EnsureConversation 实现 P0 约定的"可传可不传 conversation_id"策略：
//   - 传了 ID：必须属于同 org/project/user，否则视为不存在；
//   - 没传 ID：自动新建一条会话，标题截取 Title 前 50 字（中文按 rune 截断）。
//
// 任何路径都会做一次 RBAC 校验，确保调用方至少是该组织 viewer。
func (s *ConversationService) EnsureConversation(ctx context.Context, in EnsureInput) (*domain.Conversation, error) {
	if in.OrgID == "" || in.UserID == "" || in.ProjectID == "" {
		return nil, errcode.New(errcode.CodeInvalidArgument, "org_id / user_id / project_id are required")
	}
	if _, err := s.rbac.Require(ctx, in.OrgID, in.UserID, domain.RoleViewer); err != nil {
		return nil, err
	}
	// 校验 project 归属当前 org，避免跨租户访问。
	p, err := s.projects.GetByID(ctx, in.ProjectID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, errcode.New(errcode.CodeProjectNotFound, "project not found")
		}
		return nil, errcode.Wrap(err, errcode.CodeInternal, "load project")
	}
	if p.OrgID != in.OrgID {
		// 跨租户访问统一返回 not_found，避免泄漏存在性。
		return nil, errcode.New(errcode.CodeProjectNotFound, "project not found")
	}

	// 如果调用方传了已有 conversation_id，先校验 ownership。
	if in.ConversationID != "" {
		c, err := s.convs.GetByID(ctx, in.ConversationID)
		if err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				return nil, errcode.New(errcode.CodeNotFound, "conversation not found")
			}
			return nil, errcode.Wrap(err, errcode.CodeInternal, "load conversation")
		}
		if c.OrgID != in.OrgID || c.ProjectID != in.ProjectID || c.UserID != in.UserID {
			// 任何不匹配都视为不存在。
			return nil, errcode.New(errcode.CodeNotFound, "conversation not found")
		}
		return c, nil
	}

	// 自动创建：标题用首条用户输入截断；空标题留给后续 TitleWorker 异步生成。
	c := &domain.Conversation{
		ID:        id.New(id.PrefixConversation),
		OrgID:     in.OrgID,
		ProjectID: in.ProjectID,
		UserID:    in.UserID,
		Title:     truncateTitle(in.Title, 50),
		Status:    domain.ConversationStatusActive,
		Metadata:  map[string]any{},
	}
	if err := s.convs.Create(ctx, c); err != nil {
		return nil, errcode.Wrap(err, errcode.CodeInternal, "create conversation")
	}
	return c, nil
}

// Get 仅校验当前用户对该会话的访问权限。
func (s *ConversationService) Get(ctx context.Context, userID, convID string) (*domain.Conversation, error) {
	c, err := s.convs.GetByID(ctx, convID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, errcode.New(errcode.CodeNotFound, "conversation not found")
		}
		return nil, errcode.Wrap(err, errcode.CodeInternal, "load conversation")
	}
	if _, err := s.rbac.Require(ctx, c.OrgID, userID, domain.RoleViewer); err != nil {
		return nil, errcode.New(errcode.CodeNotFound, "conversation not found")
	}
	if c.UserID != userID {
		return nil, errcode.New(errcode.CodeNotFound, "conversation not found")
	}
	return c, nil
}

// truncateTitle 按 rune 截断；保留至少 1 个有效字符。
func truncateTitle(s string, max int) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	rs := []rune(s)
	if len(rs) <= max {
		return string(rs)
	}
	return string(rs[:max])
}

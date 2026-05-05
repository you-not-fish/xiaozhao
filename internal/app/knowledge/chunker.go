package knowledge

import (
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/xiaozhao/xiaozhao/internal/domain"
	"github.com/xiaozhao/xiaozhao/internal/infra/parser"
	"github.com/xiaozhao/xiaozhao/internal/pkg/id"
)

type Chunker struct {
	Size    int
	Overlap int
}

func NewChunker(size, overlap int) *Chunker {
	if size <= 0 {
		size = 1000
	}
	if overlap < 0 {
		overlap = 0
	}
	if overlap >= size/2 {
		overlap = size / 5
	}
	return &Chunker{Size: size, Overlap: overlap}
}

type ChunkInput struct {
	OrgID           string
	ProjectID       string
	KnowledgeBaseID string
	DocumentID      string
	FileID          string
	Parsed          *parser.Document
}

func (c *Chunker) Split(in ChunkInput) []domain.DocumentChunk {
	var out []domain.DocumentChunk
	idx := 0
	for _, part := range in.Parsed.Parts {
		text := normalizeText(part.Text)
		if text == "" {
			continue
		}
		title := part.Title
		for _, piece := range c.splitText(text) {
			chunkText := strings.TrimSpace(piece.Text)
			if chunkText == "" {
				continue
			}
			out = append(out, domain.DocumentChunk{
				ID:              id.New(id.PrefixChunk),
				OrgID:           in.OrgID,
				ProjectID:       in.ProjectID,
				KnowledgeBaseID: in.KnowledgeBaseID,
				DocumentID:      in.DocumentID,
				FileID:          in.FileID,
				ChunkIndex:      idx,
				Content:         chunkText,
				Metadata: map[string]any{
					"page":       part.Page,
					"row":        part.Row,
					"title_path": title,
					"char_start": piece.Start,
					"char_end":   piece.End,
				},
				Status: domain.DocumentChunkStatusReady,
			})
			idx++
		}
	}
	return out
}

type textPiece struct {
	Text       string
	Start, End int
}

func (c *Chunker) splitText(text string) []textPiece {
	runes := []rune(text)
	if len(runes) <= c.Size {
		return []textPiece{{Text: text, Start: 0, End: len(runes)}}
	}
	var out []textPiece
	start := 0
	for start < len(runes) {
		end := start + c.Size
		if end >= len(runes) {
			end = len(runes)
		} else {
			end = start + bestSplitOffset(runes[start:end])
		}
		if end <= start {
			end = min(start+c.Size, len(runes))
		}
		out = append(out, textPiece{Text: string(runes[start:end]), Start: start, End: end})
		if end >= len(runes) {
			break
		}
		start = end - c.Overlap
		if start < 0 {
			start = 0
		}
	}
	return out
}

var markdownHeaderRE = regexp.MustCompile(`(?m)^#{1,6}\s+`)

func normalizeText(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	s = markdownHeaderRE.ReplaceAllStringFunc(s, func(h string) string {
		return "\n\n" + h
	})
	lines := strings.Split(s, "\n")
	for i := range lines {
		lines[i] = strings.TrimSpace(lines[i])
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func bestSplitOffset(runes []rune) int {
	separators := []string{"\n\n", "\n", "。", "？", "！", "；", ".", "?", "!", ";", "，", "、", ",", " "}
	text := string(runes)
	for _, sep := range separators {
		pos := strings.LastIndex(text, sep)
		if pos <= 0 {
			continue
		}
		offset := utf8.RuneCountInString(text[:pos+len(sep)])
		if offset > len(runes)/2 {
			return offset
		}
	}
	return len(runes)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

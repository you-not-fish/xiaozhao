package parser

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/csv"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"
)

type Document struct {
	Parts []Part
}

type Part struct {
	Text     string
	Page     int
	Row      int
	Title    string
	Metadata map[string]any
}

type Parser interface {
	Parse(ctx context.Context, in Input) (*Document, error)
}

type Input struct {
	Filename string
	MimeType string
	Reader   io.Reader
}

type DefaultParser struct{}

func NewDefaultParser() *DefaultParser { return &DefaultParser{} }

func (DefaultParser) Parse(ctx context.Context, in Input) (*Document, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	raw, err := io.ReadAll(io.LimitReader(in.Reader, 64*1024*1024))
	if err != nil {
		return nil, fmt.Errorf("parser: read file: %w", err)
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, errors.New("parser: empty document")
	}
	ext := strings.ToLower(filepath.Ext(in.Filename))
	switch ext {
	case ".txt", ".md", ".markdown":
		return parseText(raw)
	case ".csv":
		return parseCSV(raw)
	case ".docx":
		return parseDOCX(raw)
	case ".pdf":
		return parsePDF(raw)
	default:
		if strings.HasPrefix(in.MimeType, "text/") || utf8.Valid(raw) {
			return parseText(raw)
		}
		return nil, fmt.Errorf("parser: unsupported file type %q", ext)
	}
}

func parseText(raw []byte) (*Document, error) {
	if !utf8.Valid(raw) {
		return nil, errors.New("parser: text is not valid utf-8")
	}
	text := strings.TrimSpace(string(raw))
	if text == "" {
		return nil, errors.New("parser: empty document")
	}
	return &Document{Parts: []Part{{Text: text, Page: 1}}}, nil
}

func parseCSV(raw []byte) (*Document, error) {
	r := csv.NewReader(bytes.NewReader(raw))
	r.FieldsPerRecord = -1
	records, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("parser: csv: %w", err)
	}
	if len(records) == 0 {
		return nil, errors.New("parser: empty csv")
	}
	headers := records[0]
	parts := make([]Part, 0, len(records))
	for i, row := range records {
		if i == 0 {
			continue
		}
		cells := make([]string, 0, len(row))
		for j, v := range row {
			name := fmt.Sprintf("col_%d", j+1)
			if j < len(headers) && strings.TrimSpace(headers[j]) != "" {
				name = strings.TrimSpace(headers[j])
			}
			cells = append(cells, name+": "+strings.TrimSpace(v))
		}
		text := strings.TrimSpace(strings.Join(cells, "\n"))
		if text != "" {
			parts = append(parts, Part{Text: text, Row: i + 1})
		}
	}
	if len(parts) == 0 {
		return nil, errors.New("parser: empty csv content")
	}
	return &Document{Parts: parts}, nil
}

func parseDOCX(raw []byte) (*Document, error) {
	reader, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return nil, fmt.Errorf("parser: docx zip: %w", err)
	}
	var docXML io.ReadCloser
	for _, f := range reader.File {
		if f.Name == "word/document.xml" {
			docXML, err = f.Open()
			if err != nil {
				return nil, err
			}
			break
		}
	}
	if docXML == nil {
		return nil, errors.New("parser: docx document.xml not found")
	}
	defer docXML.Close()

	decoder := xml.NewDecoder(docXML)
	var parts []string
	var current strings.Builder
	for {
		tok, err := decoder.Token()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, fmt.Errorf("parser: docx xml: %w", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Local == "t" {
				var text string
				if err := decoder.DecodeElement(&text, &t); err != nil {
					return nil, err
				}
				current.WriteString(text)
			}
		case xml.EndElement:
			if t.Name.Local == "p" || t.Name.Local == "tr" {
				if s := strings.TrimSpace(current.String()); s != "" {
					parts = append(parts, s)
				}
				current.Reset()
			}
		}
	}
	if s := strings.TrimSpace(current.String()); s != "" {
		parts = append(parts, s)
	}
	if len(parts) == 0 {
		return nil, errors.New("parser: empty docx")
	}
	return &Document{Parts: []Part{{Text: strings.Join(parts, "\n\n"), Page: 1}}}, nil
}

var pdfLiteralRE = regexp.MustCompile(`\(([^()]*)\)`)

func parsePDF(raw []byte) (*Document, error) {
	// MVP 只做基础文本抽取：优先提取简单 PDF 文本操作里的 literal string。
	// 复杂压缩流、扫描件和版面还原留给后续 Python parser/OCR 服务。
	matches := pdfLiteralRE.FindAllSubmatch(raw, -1)
	var parts []string
	for _, m := range matches {
		s := strings.TrimSpace(unescapePDFLiteral(string(m[1])))
		if len(s) >= 2 && utf8.ValidString(s) {
			parts = append(parts, s)
		}
	}
	text := strings.TrimSpace(strings.Join(parts, " "))
	if text == "" {
		return nil, errors.New("parser: no extractable pdf text")
	}
	return &Document{Parts: []Part{{Text: text, Page: 1}}}, nil
}

func unescapePDFLiteral(s string) string {
	repl := strings.NewReplacer(`\(`, `(`, `\)`, `)`, `\\`, `\`, `\n`, "\n", `\r`, "\n", `\t`, "\t")
	return repl.Replace(s)
}

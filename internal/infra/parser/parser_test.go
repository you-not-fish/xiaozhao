package parser

import (
	"archive/zip"
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestDefaultParserParsesTextCSVDocXAndPDF(t *testing.T) {
	p := NewDefaultParser()
	cases := []struct {
		name     string
		filename string
		body     []byte
		want     string
	}{
		{name: "text", filename: "a.txt", body: []byte("hello 文档"), want: "hello 文档"},
		{name: "csv", filename: "a.csv", body: []byte("name,role\nalice,admin\n"), want: "name: alice"},
		{name: "docx", filename: "a.docx", body: minimalDocX(t, "docx paragraph"), want: "docx paragraph"},
		{name: "pdf", filename: "a.pdf", body: []byte("%PDF-1.4\nBT (pdf text) Tj ET"), want: "pdf text"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc, err := p.Parse(context.Background(), Input{Filename: tc.filename, Reader: bytes.NewReader(tc.body)})
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			var got strings.Builder
			for _, part := range doc.Parts {
				got.WriteString(part.Text)
			}
			if !strings.Contains(got.String(), tc.want) {
				t.Fatalf("parsed text = %q, want contains %q", got.String(), tc.want)
			}
		})
	}
}

func TestDefaultParserRejectsEmptyText(t *testing.T) {
	_, err := NewDefaultParser().Parse(context.Background(), Input{Filename: "a.txt", Reader: strings.NewReader("   ")})
	if err == nil {
		t.Fatal("Parse() error = nil, want error")
	}
}

func minimalDocX(t *testing.T, text string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("word/document.xml")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = w.Write([]byte(`<w:document xmlns:w="w"><w:body><w:p><w:r><w:t>` + text + `</w:t></w:r></w:p></w:body></w:document>`))
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

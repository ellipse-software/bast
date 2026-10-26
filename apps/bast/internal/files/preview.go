package files

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"unicode/utf8"

	pdf "github.com/ledongthuc/pdf"
)

const (
	// PreviewTextLimit is the maximum prefix read for text and JSON.
	PreviewTextLimit = 512 << 10
	// PreviewPDFLimit is the maximum whole-file size for PDF extraction.
	// PDFs store the xref at EOF, so a prefix is not enough.
	PreviewPDFLimit = 8 << 20
	previewSniff    = 8 << 10
)

// PreviewKind is the display class for a file peek.
type PreviewKind string

const (
	PreviewText      PreviewKind = "text"
	PreviewJSON      PreviewKind = "json"
	PreviewPDF       PreviewKind = "pdf"
	PreviewBinary    PreviewKind = "binary"
	PreviewImage     PreviewKind = "image"
	PreviewDirectory PreviewKind = "directory"
	PreviewOther     PreviewKind = "other"
)

// Preview is a bounded peek at a local or remote file.
type Preview struct {
	Name      string
	Path      string
	Kind      PreviewKind
	Size      int64
	Text      string
	Truncated bool
	Pages     int
	Reason    string
}

var previewTextExt = map[string]struct{}{
	".txt": {}, ".md": {}, ".markdown": {}, ".log": {}, ".csv": {}, ".tsv": {},
	".env": {}, ".ini": {}, ".conf": {}, ".cfg": {}, ".toml": {}, ".xml": {},
	".html": {}, ".css": {}, ".js": {}, ".ts": {}, ".jsx": {}, ".tsx": {},
	".go": {}, ".rs": {}, ".py": {}, ".rb": {}, ".sh": {}, ".bash": {},
	".zsh": {}, ".fish": {}, ".yaml": {}, ".yml": {}, ".sql": {}, ".graphql": {},
	".dockerfile": {}, ".svg": {},
}

var previewTextName = map[string]struct{}{
	"dockerfile": {}, "makefile": {}, "cmakelists.txt": {},
	".gitignore": {}, ".editorconfig": {},
}

var previewImageExt = map[string]struct{}{
	".png": {}, ".jpg": {}, ".jpeg": {}, ".gif": {}, ".webp": {},
	".ico": {}, ".bmp": {}, ".tif": {}, ".tiff": {}, ".heic": {}, ".avif": {},
}

// ReadPreview returns a bounded preview of path on ep.
// size is used when > 0 (listing size); otherwise Stat fills it in.
func ReadPreview(ctx context.Context, ep Endpoint, filePath string, size int64, name string) (Preview, error) {
	if err := ctx.Err(); err != nil {
		return Preview{}, err
	}
	cleaned, err := cleanPreviewPath(ep, filePath)
	if err != nil {
		return Preview{}, err
	}
	if name == "" {
		name = BaseName(cleaned)
	}
	out := Preview{Name: name, Path: cleaned}

	info, err := statPreview(ep, cleaned)
	if err != nil {
		return Preview{}, err
	}
	if info.IsDir() {
		out.Kind = PreviewDirectory
		out.Size = info.Size()
		return out, nil
	}
	if size <= 0 {
		size = info.Size()
	}
	out.Size = size

	named := kindFromName(name)
	switch named {
	case PreviewImage:
		out.Kind = PreviewImage
		return out, nil
	case PreviewPDF:
		if size > PreviewPDFLimit {
			out.Kind = PreviewPDF
			out.Reason = "too large to preview"
			return out, nil
		}
	}

	src, err := openPreview(ep, cleaned)
	if err != nil {
		return Preview{}, err
	}
	defer src.Close()

	head, _, err := readCapped(ctx, src, previewSniff)
	if err != nil {
		return Preview{}, err
	}
	kind := named
	if kind == PreviewOther {
		kind = kindFromSniff(head)
	}
	if kind != PreviewPDF && isBinary(head) {
		out.Kind = PreviewBinary
		return out, nil
	}
	if kind == PreviewImage {
		out.Kind = PreviewImage
		return out, nil
	}

	want := PreviewTextLimit
	if kind == PreviewPDF {
		if size > PreviewPDFLimit {
			out.Kind = PreviewPDF
			out.Reason = "too large to preview"
			return out, nil
		}
		want = PreviewPDFLimit
	}
	data, truncated, err := continueCapped(ctx, src, head, want)
	if err != nil {
		return Preview{}, err
	}

	switch kind {
	case PreviewPDF:
		if truncated {
			out.Kind = PreviewPDF
			out.Reason = "too large to preview"
			return out, nil
		}
		text, pages, err := extractPDF(ctx, data)
		out.Kind = PreviewPDF
		out.Pages = pages
		if err != nil || strings.TrimSpace(text) == "" {
			out.Reason = "no extractable text"
			return out, nil
		}
		out.Text = sanitizePreviewText(text)
		return out, nil
	default:
		if kind == PreviewJSON {
			if pretty, ok := indentJSON(data); ok {
				out.Kind = PreviewJSON
				out.Text = pretty
				out.Truncated = truncated
				return out, nil
			}
			out.Kind = PreviewText
			out.Text = sanitizePreviewText(string(data))
			out.Truncated = truncated
			return out, nil
		}
		out.Kind = PreviewText
		out.Text = sanitizePreviewText(string(data))
		out.Truncated = truncated
		return out, nil
	}
}

func cleanPreviewPath(ep Endpoint, filePath string) (string, error) {
	if ep.local() {
		return CleanLocal(filePath)
	}
	return CleanRemote(filePath)
}

func statPreview(ep Endpoint, cleaned string) (os.FileInfo, error) {
	if ep.local() {
		return os.Stat(cleaned)
	}
	return StatRemote(ep.Session, cleaned)
}

func openPreview(ep Endpoint, cleaned string) (io.ReadCloser, error) {
	if ep.local() {
		return os.Open(cleaned)
	}
	return ep.Session.Client().Open(cleaned)
}

func kindFromName(name string) PreviewKind {
	lower := strings.ToLower(name)
	if _, ok := previewTextName[lower]; ok {
		return PreviewText
	}
	ext := strings.ToLower(path.Ext(name))
	switch ext {
	case ".json", ".jsonc":
		return PreviewJSON
	case ".pdf":
		return PreviewPDF
	}
	if _, ok := previewImageExt[ext]; ok {
		return PreviewImage
	}
	if _, ok := previewTextExt[ext]; ok {
		return PreviewText
	}
	return PreviewOther
}

func kindFromSniff(data []byte) PreviewKind {
	if isPDFMagic(data) {
		return PreviewPDF
	}
	if isImageMagic(data) {
		return PreviewImage
	}
	if isBinary(data) {
		return PreviewBinary
	}
	if looksJSON(data) {
		return PreviewJSON
	}
	return PreviewText
}

func isPDFMagic(data []byte) bool {
	return bytes.HasPrefix(bytes.TrimLeft(data, "\x00"), []byte("%PDF"))
}

func isImageMagic(data []byte) bool {
	switch {
	case bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")):
		return true
	case len(data) >= 3 && data[0] == 0xff && data[1] == 0xd8 && data[2] == 0xff:
		return true
	case bytes.HasPrefix(data, []byte("GIF87a")), bytes.HasPrefix(data, []byte("GIF89a")):
		return true
	case len(data) >= 12 && bytes.HasPrefix(data, []byte("RIFF")) && bytes.Equal(data[8:12], []byte("WEBP")):
		return true
	case bytes.HasPrefix(data, []byte("BM")):
		return true
	case bytes.HasPrefix(data, []byte("II*\x00")), bytes.HasPrefix(data, []byte("MM\x00*")):
		return true
	}
	return false
}

func isBinary(data []byte) bool {
	if bytes.IndexByte(data, 0) >= 0 {
		return true
	}
	if len(data) == 0 {
		return false
	}
	non := 0
	for _, c := range data {
		if c < 0x20 && c != '\t' && c != '\n' && c != '\r' && c != '\f' {
			non++
		}
	}
	return non*100/len(data) > 15
}

func looksJSON(data []byte) bool {
	trim := bytes.TrimSpace(data)
	return len(trim) > 0 && (trim[0] == '{' || trim[0] == '[')
}

func indentJSON(data []byte) (string, bool) {
	trim := bytes.TrimSpace(data)
	if !json.Valid(trim) {
		return "", false
	}
	var buf bytes.Buffer
	if err := json.Indent(&buf, trim, "", "  "); err != nil {
		return "", false
	}
	return buf.String(), true
}

func sanitizePreviewText(s string) string {
	if utf8.ValidString(s) {
		return s
	}
	return strings.ToValidUTF8(s, "\uFFFD")
}

func readCapped(ctx context.Context, r io.Reader, capBytes int) ([]byte, bool, error) {
	if capBytes < 0 {
		capBytes = 0
	}
	buf := make([]byte, capBytes+1)
	n := 0
	for n < len(buf) {
		if err := ctx.Err(); err != nil {
			return nil, false, err
		}
		nn, err := r.Read(buf[n:])
		n += nn
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, false, err
		}
		if nn == 0 {
			break
		}
	}
	if n > capBytes {
		return buf[:capBytes], true, nil
	}
	return buf[:n], false, nil
}

func continueCapped(ctx context.Context, r io.Reader, head []byte, capBytes int) ([]byte, bool, error) {
	if len(head) > capBytes {
		return head[:capBytes], true, nil
	}
	if len(head) == capBytes {
		// One extra byte tells us whether more remains.
		extra := make([]byte, 1)
		if err := ctx.Err(); err != nil {
			return nil, false, err
		}
		n, err := r.Read(extra)
		if n > 0 {
			return head, true, nil
		}
		if err != nil && err != io.EOF {
			return nil, false, err
		}
		return head, false, nil
	}
	rest, truncated, err := readCapped(ctx, r, capBytes-len(head))
	if err != nil {
		return nil, false, err
	}
	out := make([]byte, 0, len(head)+len(rest))
	out = append(out, head...)
	out = append(out, rest...)
	return out, truncated, nil
}

func extractPDF(ctx context.Context, data []byte) (text string, pages int, err error) {
	if err := ctx.Err(); err != nil {
		return "", 0, err
	}
	defer func() {
		if rec := recover(); rec != nil {
			text, pages = "", 0
			err = fmt.Errorf("pdf: %v", rec)
		}
	}()
	reader, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", 0, err
	}
	pages = reader.NumPage()
	var b strings.Builder
	for i := 1; i <= pages; i++ {
		if err := ctx.Err(); err != nil {
			return "", pages, err
		}
		page := reader.Page(i)
		if page.V.IsNull() {
			continue
		}
		plain, err := page.GetPlainText(nil)
		if err != nil || strings.TrimSpace(plain) == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(plain)
	}
	return strings.TrimSpace(b.String()), pages, nil
}

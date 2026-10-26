package files

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadPreviewTextTruncates(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "big.log")
	payload := bytes.Repeat([]byte("a"), PreviewTextLimit+64)
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := ReadPreview(context.Background(), Endpoint{}, path, int64(len(payload)), "big.log")
	if err != nil {
		t.Fatal(err)
	}
	if got.Kind != PreviewText {
		t.Fatalf("kind = %q", got.Kind)
	}
	if !got.Truncated {
		t.Fatal("expected truncated")
	}
	if len(got.Text) != PreviewTextLimit {
		t.Fatalf("len = %d, want %d", len(got.Text), PreviewTextLimit)
	}
}

func TestReadPreviewBinaryNUL(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "blob.bin")
	if err := os.WriteFile(path, []byte("hello\x00world"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := ReadPreview(context.Background(), Endpoint{}, path, 0, "blob.bin")
	if err != nil {
		t.Fatal(err)
	}
	if got.Kind != PreviewBinary {
		t.Fatalf("kind = %q", got.Kind)
	}
	if got.Text != "" {
		t.Fatalf("binary text = %q", got.Text)
	}
}

func TestReadPreviewJSONIndentKeepsKeyOrder(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "notes.json")
	raw := []byte(`{"z":1,"a":2}`)
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := ReadPreview(context.Background(), Endpoint{}, path, 0, "notes.json")
	if err != nil {
		t.Fatal(err)
	}
	if got.Kind != PreviewJSON {
		t.Fatalf("kind = %q", got.Kind)
	}
	if !strings.Contains(got.Text, `"z": 1`) || !strings.Contains(got.Text, `"a": 2`) {
		t.Fatalf("pretty = %q", got.Text)
	}
	if strings.Index(got.Text, `"z"`) > strings.Index(got.Text, `"a"`) {
		t.Fatalf("key order lost: %q", got.Text)
	}
}

func TestReadPreviewTextFileWithJSONStaysText(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "notes.txt")
	raw := []byte(`{"z":1,"a":2}`)
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := ReadPreview(context.Background(), Endpoint{}, path, 0, "notes.txt")
	if err != nil {
		t.Fatal(err)
	}
	if got.Kind != PreviewText {
		t.Fatalf("kind = %q", got.Kind)
	}
	if got.Text != string(raw) {
		t.Fatalf("text = %q", got.Text)
	}
}

func TestReadPreviewInvalidJSONIsText(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "broken.json")
	raw := []byte(`{"z":1,`)
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := ReadPreview(context.Background(), Endpoint{}, path, 0, "broken.json")
	if err != nil {
		t.Fatal(err)
	}
	if got.Kind != PreviewText {
		t.Fatalf("kind = %q", got.Kind)
	}
	if got.Text != string(raw) {
		t.Fatalf("text = %q", got.Text)
	}
}

func TestReadPreviewPDFExtractsText(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hello.pdf")
	payload := helloPDF("Hello preview")
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := ReadPreview(context.Background(), Endpoint{}, path, int64(len(payload)), "hello.pdf")
	if err != nil {
		t.Fatal(err)
	}
	if got.Kind != PreviewPDF {
		t.Fatalf("kind = %q", got.Kind)
	}
	if !strings.Contains(got.Text, "Hello preview") {
		t.Fatalf("pdf text = %q reason = %q", got.Text, got.Reason)
	}
	if got.Pages < 1 {
		t.Fatalf("pages = %d", got.Pages)
	}
}

func TestReadPreviewPDFEmptyHasReason(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.pdf")
	payload := helloPDF("")
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := ReadPreview(context.Background(), Endpoint{}, path, int64(len(payload)), "empty.pdf")
	if err != nil {
		t.Fatal(err)
	}
	if got.Kind != PreviewPDF {
		t.Fatalf("kind = %q", got.Kind)
	}
	if got.Text != "" {
		t.Fatalf("empty pdf text = %q", got.Text)
	}
	if got.Reason != "no extractable text" {
		t.Fatalf("reason = %q", got.Reason)
	}
}

func TestReadPreviewPDFOversizeDoesNotNeedBody(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "huge.pdf")
	if err := os.WriteFile(path, helloPDF("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := ReadPreview(context.Background(), Endpoint{}, path, PreviewPDFLimit+1, "huge.pdf")
	if err != nil {
		t.Fatal(err)
	}
	if got.Kind != PreviewPDF {
		t.Fatalf("kind = %q", got.Kind)
	}
	if got.Text != "" {
		t.Fatalf("oversize should not extract, text = %q", got.Text)
	}
	if got.Reason != "too large to preview" {
		t.Fatalf("reason = %q", got.Reason)
	}
}

func TestReadPreviewMalformedPDFDoesNotPanic(t *testing.T) {
	defer func() {
		if rec := recover(); rec != nil {
			t.Fatalf("ReadPreview panicked: %v", rec)
		}
	}()
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.pdf")
	payload := []byte("%PDF-1.4\n1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n%%EOF\n")
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		t.Fatal(err)
	}
	_, _ = ReadPreview(context.Background(), Endpoint{}, path, int64(len(payload)), "bad.pdf")
}

func TestExtractPDFRecoversPanic(t *testing.T) {
	defer func() {
		if rec := recover(); rec != nil {
			t.Fatalf("extractPDF panicked: %v", rec)
		}
	}()
	_, _, _ = extractPDF(context.Background(), []byte("%PDF-1.4\ntrailer\n<< /Root 99 0 R >>\n%%EOF\n"))
}

func TestReadPreviewDirectory(t *testing.T) {
	dir := t.TempDir()
	got, err := ReadPreview(context.Background(), Endpoint{}, dir, 0, "nested")
	if err != nil {
		t.Fatal(err)
	}
	if got.Kind != PreviewDirectory {
		t.Fatalf("kind = %q", got.Kind)
	}
}

func TestReadPreviewRejectsRemoteTraversal(t *testing.T) {
	_, err := CleanRemote("/tmp/../etc/passwd")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := JoinRemote("/tmp", "../x"); err == nil {
		t.Fatal("expected invalid join")
	}
}

func TestReadPreviewImageByExtensionSkipsBody(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "shot.png")
	if err := os.WriteFile(path, []byte("not-a-png-but-named-so"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := ReadPreview(context.Background(), Endpoint{}, path, 0, "shot.png")
	if err != nil {
		t.Fatal(err)
	}
	if got.Kind != PreviewImage {
		t.Fatalf("kind = %q", got.Kind)
	}
	if got.Text != "" {
		t.Fatalf("image text = %q", got.Text)
	}
}

func helloPDF(text string) []byte {
	escaped := strings.NewReplacer(`\`, `\\`, `(`, `\(`, `)`, `\)`).Replace(text)
	var stream string
	if text == "" {
		stream = "BT ET\n"
	} else {
		stream = fmt.Sprintf("BT /F1 12 Tf 72 720 Td (%s) Tj ET\n", escaped)
	}
	objs := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R /Resources << /Font << /F1 5 0 R >> >> >>",
		fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(stream), strings.TrimRight(stream, "\n")),
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
	}
	var buf bytes.Buffer
	buf.WriteString("%PDF-1.4\n")
	offsets := make([]int, len(objs)+1)
	for i, obj := range objs {
		offsets[i+1] = buf.Len()
		fmt.Fprintf(&buf, "%d 0 obj\n%s\nendobj\n", i+1, obj)
	}
	startxref := buf.Len()
	fmt.Fprintf(&buf, "xref\n0 %d\n", len(objs)+1)
	buf.WriteString("0000000000 65535 f \n")
	for i := 1; i <= len(objs); i++ {
		fmt.Fprintf(&buf, "%010d 00000 n \n", offsets[i])
	}
	fmt.Fprintf(&buf, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objs)+1, startxref)
	return buf.Bytes()
}

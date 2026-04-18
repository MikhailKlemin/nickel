//go:build go1.25

package statement

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractText_FromPDFFixture(t *testing.T) {
	t.Parallel()

	if _, err := exec.LookPath("pdftotext"); err != nil {
		t.Skip("pdftotext not installed")
	}

	ctx := t.Context()
	pdfPath := filepath.Join("testdata", "sample_statement.pdf")

	text, err := ExtractText(ctx, pdfPath)
	if err != nil {
		t.Fatalf("extract text from pdf: %v", err)
	}

	if strings.TrimSpace(text) == "" {
		t.Fatal("extracted text is empty")
	}

	if !strings.Contains(text, "IBAN") {
		t.Fatal("expected extracted text to contain IBAN")
	}
}

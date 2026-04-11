package statement

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func Read(path string, ctx context.Context) (string, error) {
	if strings.EqualFold(filepath.Ext(path), ".txt") {
		return ReadTextFile(path)
	}
	return ExtractText(ctx, path)
}

func ReadTextFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read text statement %q: %w", path, err)
	}
	return string(data), nil
}

func ExtractText(ctx context.Context, pdfPath string) (string, error) {
	var out bytes.Buffer
	var errBuf bytes.Buffer

	cmd := exec.CommandContext(ctx, "pdftotext", "-layout", pdfPath, "-")
	cmd.Stdout = &out
	cmd.Stderr = &errBuf

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("pdftotext %q: %w: %s", pdfPath, err, strings.TrimSpace(errBuf.String()))
	}

	return out.String(), nil
}

func ParseFile(ctx context.Context, path string, logger anyLogger) (ParsedStatement, error) {
	text, err := Read(path, ctx)
	if err != nil {
		return ParsedStatement{}, fmt.Errorf("read statement: %w", err)
	}
	return Parse(text, logger)
}

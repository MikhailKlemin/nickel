//go:build go1.25

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

func Read(ctx context.Context, path string) (string, error) {
	if strings.EqualFold(filepath.Ext(path), ".txt") {
		return ReadTextFile(path)
	}
	return ExtractText(ctx, path)
}

func ReadTextFile(path string) (string, error) {
	// Validate path first
	if err := validatePath(path); err != nil {
		return "", fmt.Errorf("path validation failed for %q: %w", path, err)
	}
	
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read text statement %q: %w", path, err)
	}
	return string(data), nil
}

func ExtractText(ctx context.Context, pdfPath string) (string, error) {
	// Validate path first
	if err := validatePath(pdfPath); err != nil {
		return "", fmt.Errorf("path validation failed for %q: %w", pdfPath, err)
	}
	
	var out bytes.Buffer
	var errBuf bytes.Buffer

	// Use absolute path for security
	absPath, err := filepath.Abs(pdfPath)
	if err != nil {
		return "", fmt.Errorf("failed to get absolute path for %q: %w", pdfPath, err)
	}
	
	cmd := exec.CommandContext(ctx, "pdftotext", "-layout", absPath, "-")
	cmd.Stdout = &out
	cmd.Stderr = &errBuf

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("pdftotext %q: %w: %s", pdfPath, err, strings.TrimSpace(errBuf.String()))
	}

	return out.String(), nil
}

func ParseFile(ctx context.Context, path string, logger anyLogger) (ParsedStatement, error) {
	text, err := Read(ctx, path)
	if err != nil {
		return ParsedStatement{}, fmt.Errorf("read statement: %w", err)
	}
	return Parse(text, logger)
}

// validatePath ensures the given path is within an allowed directory
// to prevent command injection attacks.
func validatePath(path string) error {
	// Get base directory from environment or use current directory
	baseDir := os.Getenv("SAFE_BASE_DIR")
	if baseDir == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("failed to get current directory: %w", err)
		}
		baseDir = cwd
	}

	// Get absolute paths
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("invalid path %q: %w", path, err)
	}
	
	absBaseDir, err := filepath.Abs(baseDir)
	if err != nil {
		return fmt.Errorf("invalid base directory %q: %w", baseDir, err)
	}

	// Check if path is within base directory
	relPath, err := filepath.Rel(absBaseDir, absPath)
	if err != nil {
		return fmt.Errorf("path %q is outside base directory %q", absPath, absBaseDir)
	}

	// Prevent directory traversal
	if strings.HasPrefix(relPath, "..") {
		return fmt.Errorf("path %q attempts directory traversal", path)
	}

	return nil
}

//go:build go1.25

package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"nickel/statement"
)

func run(ctx context.Context, args []string, logger *slog.Logger) error {
	fs := flag.NewFlagSet("nickel-parse", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	filePath := fs.String("file", "", "path to statement file (PDF or TXT)")
	outputPath := fs.String("out", "", "output JSON file path (default: <input>.json)")

	if err := fs.Parse(args[1:]); err != nil {
		return err
	}

	// Validate required flags
	if *filePath == "" {
		return fmt.Errorf("missing required flag -file")
	}

	// Parse statement file
	logger.Info("parsing statement file", "path", *filePath)
	parsedStmt, err := statement.ParseFile(ctx, *filePath, logger)
	if err != nil {
		return fmt.Errorf("parse file: %w", err)
	}

	// Convert to JSON
	jsonData, err := parsedStmt.ToJSON()
	if err != nil {
		return fmt.Errorf("convert to JSON: %w", err)
	}

	// Determine output path if not provided
	if *outputPath == "" {
		inputPath := *filePath // Dereference the pointer
		ext := filepath.Ext(inputPath)
		base := inputPath[:len(inputPath)-len(ext)]
		*outputPath = base + ".json"
	}

	// Ensure the directory exists
	if dir := filepath.Dir(*outputPath); dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("create output directory: %w", err)
		}
	}
	
	// Write to file
	if err := os.WriteFile(*outputPath, jsonData, 0644); err != nil {
		return fmt.Errorf("write output file: %w", err)
	}

	logger.Info("successfully wrote parsed statement",
		"output", *outputPath,
		"transactions", len(parsedStmt.Transactions),
		"skipped_blocks", parsedStmt.SkippedTxBlocks,
	)

	return nil
}

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	if err := run(context.Background(), os.Args, logger); err != nil {
		logger.Error("fatal", "err", err)
		os.Exit(1)
	}
}

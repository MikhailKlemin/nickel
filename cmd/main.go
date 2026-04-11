package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"

	"nickel/statement"
)

func run(ctx context.Context, args []string, logger *slog.Logger) error {
	fs := flag.NewFlagSet("parse-nickel-statement", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	outputPath := fs.String("output", "", "write JSON output to file instead of stdout")

	if err := fs.Parse(args[1:]); err != nil {
		return err
	}

	rest := fs.Args()
	if len(rest) < 1 || len(rest) > 2 {
		return fmt.Errorf("usage: parse-nickel-statement [-output result.json] <statement.pdf|statement.txt> [result.json]")
	}

	inputPath := rest[0]

	if *outputPath == "" && len(rest) == 2 {
		*outputPath = rest[1]
	}

	stmt, err := statement.ParseFile(ctx, inputPath, logger)
	if err != nil {
		return err
	}

	var w io.Writer = os.Stdout
	var file *os.File

	if *outputPath != "" {
		file, err = os.Create(*outputPath)
		if err != nil {
			return fmt.Errorf("create output file %q: %w", *outputPath, err)
		}
		defer file.Close()
		w = file
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(stmt); err != nil {
		return fmt.Errorf("encode JSON: %w", err)
	}

	return nil
}

func main() {
	logger := slog.Default()

	if err := run(context.Background(), os.Args, logger); err != nil {
		logger.Error("fatal", "err", err)
		os.Exit(1)
	}
}

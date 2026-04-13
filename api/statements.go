//go:build go1.25

package api

import (
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"nickel/statement"
)

// maxUploadBytes is the hard cap on the raw request body for PDF/TXT uploads.
// 32 MiB is generous for a bank statement; anything larger is rejected before
// the multipart parser even runs.
const maxUploadBytes = 32 << 20 // 32 MiB

// allowedExtensions is the set of file extensions the upload endpoint accepts.
var allowedExtensions = map[string]bool{
	".pdf": true,
	".txt": true,
}

func (s *Server) handleUpload(w http.ResponseWriter, r *http.Request) {
	// Enforce a hard body limit before the multipart parser buffers anything.
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes)

	if err := r.ParseMultipartForm(maxUploadBytes); err != nil {
		respondError(w, http.StatusRequestEntityTooLarge, "PAYLOAD_TOO_LARGE", "file exceeds the 32 MiB limit")
		return
	}

	f, header, err := r.FormFile("file")
	if err != nil {
		respondError(w, http.StatusBadRequest, "BAD_REQUEST", `missing "file" field`)
		return
	}
	defer f.Close()

	// Validate the file extension against the allowlist before touching disk.
	ext := strings.ToLower(filepath.Ext(header.Filename))
	if !allowedExtensions[ext] {
		respondError(w, http.StatusUnsupportedMediaType, "UNSUPPORTED_FILE_TYPE", "only .pdf and .txt files are accepted")
		return
	}

	// statement.ParseFile needs a file path, so we buffer to a temp file.
	tmp, err := os.CreateTemp("", "nickel-*"+ext)
	if err != nil {
		s.logger.Error("create temp file", "err", err)
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}
	defer os.Remove(tmp.Name())

	if _, err := io.Copy(tmp, f); err != nil {
		tmp.Close()
		s.logger.Error("write temp file", "err", err)
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}
	tmp.Close() // must close before ParseFile opens it

	parsed, err := statement.ParseFile(r.Context(), tmp.Name(), s.logger)
	if err != nil {
		respondError(w, http.StatusUnprocessableEntity, "PARSE_ERROR", err.Error())
		return
	}

	rec, err := statement.MapToStatementRecord(&parsed, time.Now())
	if err != nil {
		respondError(w, http.StatusUnprocessableEntity, "PARSE_ERROR", err.Error())
		return
	}

	txRecords := statement.MapToTransactionRecords(0, parsed.Transactions) // StatementID stamped inside ImportStatement
	result, err := statement.ImportStatement(r.Context(), s.pool, rec, txRecords)
	if err != nil {
		if errors.Is(err, statement.ErrStatementExists) {
			respondError(w, http.StatusConflict, "ALREADY_EXISTS", "statement for this period is already imported")
			return
		}
		s.logger.Error("import statement", "err", err)
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to save statement")
		return
	}

	row, err := statement.GetStatementByID(r.Context(), s.pool, result.StatementID)
	if err != nil {
		s.logger.Error("fetch statement after insert", "id", result.StatementID, "err", err)
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "statement saved but could not be fetched")
		return
	}

	resp := statement.StatementRowToResponse(row)
	if parsed.SkippedTxBlocks > 0 {
		s.logger.Warn("partial import: some transaction blocks could not be parsed",
			"statement_id", result.StatementID,
			"skipped", parsed.SkippedTxBlocks,
		)
	}
	respondJSON(w, http.StatusCreated, resp)
}

func (s *Server) handleListStatements(w http.ResponseWriter, r *http.Request) {
	rows, err := statement.ListStatements(r.Context(), s.pool)
	if err != nil {
		s.logger.Error("list statements", "err", err)
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list statements")
		return
	}

	resp := make([]statement.StatementResponse, len(rows))
	for i, row := range rows {
		resp[i] = statement.StatementRowToResponse(&row)
	}
	respondJSON(w, http.StatusOK, resp)
}

func (s *Server) handleGetStatement(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid statement id")
		return
	}

	row, err := statement.GetStatementByID(r.Context(), s.pool, id)
	if err != nil {
		if errors.Is(err, statement.ErrNotFound) {
			respondError(w, http.StatusNotFound, "NOT_FOUND", "statement not found")
			return
		}
		s.logger.Error("get statement", "id", id, "err", err)
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to get statement")
		return
	}

	respondJSON(w, http.StatusOK, statement.StatementRowToResponse(row))
}

func (s *Server) handleListStatementTransactions(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid statement id")
		return
	}

	f, err := parseTransactionFilter(r)
	if err != nil {
		respondError(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}
	f.StatementID = &id // narrow the query to this statement

	s.serveTransactionList(w, r, f)
}

//go:build go1.25

package api

import (
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"nickel/statement"
)

func (s *Server) handleUpload(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		respondError(w, http.StatusBadRequest, "BAD_REQUEST", "failed to parse multipart form")
		return
	}

	f, header, err := r.FormFile("file")
	if err != nil {
		respondError(w, http.StatusBadRequest, "BAD_REQUEST", `missing "file" field`)
		return
	}
	defer f.Close()

	// statement.ParseFile needs a file path, so we buffer to a temp file.
	tmp, err := os.CreateTemp("", "nickel-*"+filepath.Ext(header.Filename))
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

	stmtID, err := statement.InsertStatement(r.Context(), s.pool, rec)
	if err != nil {
		if errors.Is(err, statement.ErrStatementExists) {
			respondError(w, http.StatusConflict, "ALREADY_EXISTS", "statement for this period is already imported")
			return
		}
		s.logger.Error("insert statement", "err", err)
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to save statement")
		return
	}

	txRecords := statement.MapToTransactionRecords(stmtID, parsed.Transactions)
	if err := statement.InsertTransactions(r.Context(), s.pool, txRecords); err != nil {
		s.logger.Error("insert transactions", "err", err)
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to save transactions")
		return
	}

	row, err := statement.GetStatementByID(r.Context(), s.pool, stmtID)
	if err != nil {
		s.logger.Error("fetch statement after insert", "id", stmtID, "err", err)
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "statement saved but could not be fetched")
		return
	}

	respondJSON(w, http.StatusCreated, statement.StatementRowToResponse(row))
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

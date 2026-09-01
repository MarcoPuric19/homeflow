package httpapi

import (
	"errors"
	"net/http"

	"homeflow/backend/internal/auth"
	"homeflow/backend/internal/links"
)

func (s *Server) getDocumentLinks(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.PrincipalFromContext(r.Context())
	set, err := s.links.GetDocument(r.Context(), principal.UserID, r.PathValue("petID"), r.PathValue("documentID"))
	s.writeLinks(w, set, err)
}

func (s *Server) replaceDocumentLinks(w http.ResponseWriter, r *http.Request) {
	var input links.Set
	if err := decodeJSON(w, r, &input); err != nil {
		writeProblem(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	principal, _ := auth.PrincipalFromContext(r.Context())
	set, err := s.links.ReplaceDocument(r.Context(), principal.UserID, r.PathValue("petID"), r.PathValue("documentID"), input)
	s.writeLinks(w, set, err)
}

func (s *Server) getExpenseLinks(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.PrincipalFromContext(r.Context())
	set, err := s.links.GetExpense(r.Context(), principal.UserID, r.PathValue("expenseID"))
	s.writeLinks(w, set, err)
}

func (s *Server) replaceExpenseLinks(w http.ResponseWriter, r *http.Request) {
	var input links.Set
	if err := decodeJSON(w, r, &input); err != nil {
		writeProblem(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	principal, _ := auth.PrincipalFromContext(r.Context())
	set, err := s.links.ReplaceExpense(r.Context(), principal.UserID, r.PathValue("expenseID"), input)
	s.writeLinks(w, set, err)
}

func (s *Server) writeLinks(w http.ResponseWriter, set links.Set, err error) {
	switch {
	case errors.Is(err, links.ErrInvalid):
		writeProblem(w, http.StatusBadRequest, "Invalid relationship request")
	case errors.Is(err, links.ErrNotFound):
		writeProblem(w, http.StatusNotFound, "Relationship source or target not found")
	case err != nil:
		s.internalError(w, err)
	default:
		writeJSON(w, http.StatusOK, set)
	}
}

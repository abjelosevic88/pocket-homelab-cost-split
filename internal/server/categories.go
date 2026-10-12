package server

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/abjelosevic88/pocket-homelab-cost-split/internal/store"
)

func (s *Server) handleCategories(w http.ResponseWriter, r *http.Request) {
	list, err := s.store.ListCategories(r.Context())
	if err != nil {
		s.fail(w, err, "categories")
		return
	}
	writeJSON(w, http.StatusOK, list)
}

type categoryInput struct {
	Label     string `json:"label"`
	Icon      string `json:"icon"`
	SortOrder *int   `json:"sort_order"`
}

// handleCreateCategory is open to every signed-in user: the expense form
// offers "Create …" when a typed category does not exist yet.
func (s *Server) handleCreateCategory(w http.ResponseWriter, r *http.Request) {
	var in categoryInput
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	in.Label = strings.TrimSpace(in.Label)
	if in.Label == "" || len(in.Label) > 60 {
		writeErr(w, http.StatusBadRequest, "a name of up to 60 characters is required")
		return
	}
	if in.Icon == "" {
		in.Icon = "🏷️"
	}
	c, err := s.store.CreateCategory(r.Context(), in.Label, in.Icon)
	if err != nil {
		s.fail(w, err, "create category")
		return
	}
	writeJSON(w, http.StatusCreated, c)
}

func (s *Server) handleUpdateCategory(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	c, err := s.store.GetCategory(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		s.fail(w, err, "category")
		return
	}
	var in categoryInput
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if l := strings.TrimSpace(in.Label); l != "" && len(l) <= 60 {
		c.Label = l
	}
	if in.Icon != "" {
		c.Icon = strings.TrimSpace(in.Icon)
	}
	if in.SortOrder != nil {
		c.SortOrder = *in.SortOrder
	}
	if err := s.store.UpdateCategory(r.Context(), c); err != nil {
		s.fail(w, err, "update category")
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (s *Server) handleDeleteCategory(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	if err := s.store.DeleteCategory(r.Context(), chi.URLParam(r, "id")); err != nil {
		if err == store.ErrNotFound {
			s.fail(w, err, "category")
			return
		}
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

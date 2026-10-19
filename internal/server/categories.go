package server

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/abjelosevic88/pocket-homelab-cost-split/internal/store"
)

func (s *Server) handleCategories(w http.ResponseWriter, r *http.Request) {
	list, err := s.store.ListCategories(r.Context(), userFrom(r.Context()).ID)
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
// offers "Create …" when a typed category does not exist yet. The new
// category is the creator's own.
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
	c, err := s.store.CreateCategory(r.Context(), userFrom(r.Context()).ID, in.Label, in.Icon)
	if err != nil {
		s.fail(w, err, "create category")
		return
	}
	writeJSON(w, http.StatusCreated, c)
}

// editableCategory loads a category the viewer may change: their own, or a
// built-in one if they are an admin (the built-in list is box-wide). Someone
// else's category is 404; a built-in one for a member is 403.
func (s *Server) editableCategory(w http.ResponseWriter, r *http.Request) *store.Category {
	u := userFrom(r.Context())
	c, err := s.store.GetCategory(r.Context(), chi.URLParam(r, "id"), u.ID)
	if err != nil {
		s.fail(w, err, "category")
		return nil
	}
	if c.Builtin && u.Role != "admin" {
		writeErr(w, http.StatusForbidden, "only an admin can change the built-in categories")
		return nil
	}
	return c
}

func (s *Server) handleUpdateCategory(w http.ResponseWriter, r *http.Request) {
	c := s.editableCategory(w, r)
	if c == nil {
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
	c := s.editableCategory(w, r)
	if c == nil {
		return
	}
	if err := s.store.DeleteCategory(r.Context(), c.ID); err != nil {
		if err == store.ErrNotFound {
			s.fail(w, err, "category")
			return
		}
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

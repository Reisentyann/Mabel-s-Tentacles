package api

import (
	"net/http"
	"strconv"
)

func (s *Server) listOperations(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	size, _ := strconv.Atoi(r.URL.Query().Get("size"))
	if size < 1 || size > 200 {
		size = 20
	}

	if s.repo == nil {
		writeJSON(w, http.StatusOK, map[string]any{"items": []any{}, "total": 0, "page": page, "size": size})
		return
	}

	items, total, err := s.repo.GetOperations(r.Context(), page, size)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": total, "page": page, "size": size})
}

package handlers

import (
	"net/http"

	"github.com/lefteq/lovinka-deployik/internal/auth"
	"github.com/lefteq/lovinka-deployik/internal/db"
)

type OrganizationHandler struct {
	DB *db.DB
}

func (h *OrganizationHandler) List(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	organizations, err := h.DB.ListOrganizationsForUser(claims.UserID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list organizations"})
		return
	}
	if organizations == nil {
		organizations = []db.Organization{}
	}
	writeJSON(w, http.StatusOK, organizations)
}

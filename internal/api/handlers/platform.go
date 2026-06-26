package handlers

import (
	"net/http"

	"github.com/lefteq/lovinka-deployik/internal/db"
)

type PlatformHandler struct {
	DNSTargetIP string
}

func (h *PlatformHandler) Get(w http.ResponseWriter, r *http.Request) {
	// PreviewDomainSuffix is set once at startup from BASE_DOMAIN /
	// PREVIEW_DOMAIN_SUFFIX; the frontend uses it to show the real preview
	// hostname (e.g. my-app.preview.example.com) instead of a hardcoded domain.
	writeJSON(w, http.StatusOK, map[string]string{
		"dns_target_ip":         h.DNSTargetIP,
		"preview_domain_suffix": db.PreviewDomainSuffix,
	})
}

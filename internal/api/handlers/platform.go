package handlers

import "net/http"

type PlatformHandler struct {
	DNSTargetIP string
}

func (h *PlatformHandler) Get(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"dns_target_ip": h.DNSTargetIP,
	})
}

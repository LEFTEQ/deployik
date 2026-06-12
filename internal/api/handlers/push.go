package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/LEFTEQ/lovinka-deployik/internal/audit"
	"github.com/LEFTEQ/lovinka-deployik/internal/auth"
	"github.com/LEFTEQ/lovinka-deployik/internal/db"
	"github.com/LEFTEQ/lovinka-deployik/internal/push"
)

// PushHandler manages Web Push subscriptions for the calling user. Keys is
// nil when VAPID material couldn't be loaded — endpoints then answer 503 so
// the rest of the app keeps working with push disabled.
type PushHandler struct {
	DB    *db.DB
	Keys  *push.VAPIDKeys
	Audit *audit.Recorder
}

type pushSubscribeRequest struct {
	Endpoint string `json:"endpoint"`
	Keys     struct {
		P256dh string `json:"p256dh"`
		Auth   string `json:"auth"`
	} `json:"keys"`
	DeviceLabel string `json:"device_label"`
	// Optional initial preferences; nil = default true.
	NotifyDeployOutcomes *bool `json:"notify_deploy_outcomes"`
	NotifyBuildStarts    *bool `json:"notify_build_starts"`
	NotifySSLIssues      *bool `json:"notify_ssl_issues"`
}

type pushPreferencesRequest struct {
	NotifyDeployOutcomes *bool `json:"notify_deploy_outcomes"`
	NotifyBuildStarts    *bool `json:"notify_build_starts"`
	NotifySSLIssues      *bool `json:"notify_ssl_issues"`
}

func (h *PushHandler) pushDisabled(w http.ResponseWriter) bool {
	if h.Keys == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "push notifications are not available"})
		return true
	}
	return false
}

// VAPIDKey returns the public application server key used to subscribe.
func (h *PushHandler) VAPIDKey(w http.ResponseWriter, r *http.Request) {
	if h.pushDisabled(w) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"public_key": h.Keys.PublicKey})
}

// List returns the caller's registered devices.
func (h *PushHandler) List(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthenticated"})
		return
	}
	subs, err := h.DB.ListPushSubscriptionsForUser(claims.UserID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list subscriptions"})
		return
	}
	writeJSON(w, http.StatusOK, subs)
}

// Subscribe registers (or refreshes) the calling device.
func (h *PushHandler) Subscribe(w http.ResponseWriter, r *http.Request) {
	if h.pushDisabled(w) {
		return
	}
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthenticated"})
		return
	}

	var req pushSubscribeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	endpoint := strings.TrimSpace(req.Endpoint)
	if endpoint == "" || !strings.HasPrefix(endpoint, "https://") {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "a valid https endpoint is required"})
		return
	}
	if req.Keys.P256dh == "" || req.Keys.Auth == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "subscription keys are required"})
		return
	}

	boolOr := func(v *bool, fallback bool) bool {
		if v == nil {
			return fallback
		}
		return *v
	}
	label := strings.TrimSpace(req.DeviceLabel)
	if len(label) > 120 {
		label = label[:120]
	}

	sub := &db.PushSubscription{
		UserID:               claims.UserID,
		Endpoint:             endpoint,
		P256dh:               req.Keys.P256dh,
		Auth:                 req.Keys.Auth,
		DeviceLabel:          label,
		NotifyDeployOutcomes: boolOr(req.NotifyDeployOutcomes, true),
		NotifyBuildStarts:    boolOr(req.NotifyBuildStarts, true),
		NotifySSLIssues:      boolOr(req.NotifySSLIssues, true),
	}
	if err := h.DB.UpsertPushSubscription(sub); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save subscription"})
		return
	}

	h.Audit.Record(audit.Entry{
		UserID:       claims.UserID,
		Action:       "push.subscribe",
		ResourceType: "push_subscription",
		ResourceID:   sub.ID,
	})
	writeJSON(w, http.StatusCreated, sub)
}

// UpdatePreferences PATCHes per-device event toggles (presence-aware: omitted
// fields keep their stored value).
func (h *PushHandler) UpdatePreferences(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthenticated"})
		return
	}
	id := chi.URLParam(r, "sid")

	existing, err := h.DB.GetPushSubscriptionForUser(id, claims.UserID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load subscription"})
		return
	}
	if existing == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "subscription not found"})
		return
	}

	var req pushPreferencesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if err := h.DB.UpdatePushSubscriptionPreferences(id, claims.UserID,
		req.NotifyDeployOutcomes, req.NotifyBuildStarts, req.NotifySSLIssues); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update preferences"})
		return
	}

	updated, err := h.DB.GetPushSubscriptionForUser(id, claims.UserID)
	if err != nil || updated == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load subscription"})
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

// Unsubscribe removes a device. 404 for foreign or unknown subscriptions.
func (h *PushHandler) Unsubscribe(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthenticated"})
		return
	}
	id := chi.URLParam(r, "sid")

	deleted, err := h.DB.DeletePushSubscription(id, claims.UserID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete subscription"})
		return
	}
	if !deleted {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "subscription not found"})
		return
	}

	h.Audit.Record(audit.Entry{
		UserID:       claims.UserID,
		Action:       "push.unsubscribe",
		ResourceType: "push_subscription",
		ResourceID:   id,
	})
	w.WriteHeader(http.StatusNoContent)
}

package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/LEFTEQ/lovinka-deployik/internal/audit"
	"github.com/LEFTEQ/lovinka-deployik/internal/db"
	"github.com/LEFTEQ/lovinka-deployik/internal/push"
)

func newPushTestHandler(t *testing.T) (*PushHandler, *db.User, *db.User) {
	t.Helper()
	database, err := db.OpenMemory()
	if err != nil {
		t.Fatalf("open memory db: %v", err)
	}
	if err := database.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	owner := &db.User{ID: db.NewID(), GithubID: 1, Username: "owner", Role: "user"}
	other := &db.User{ID: db.NewID(), GithubID: 2, Username: "other", Role: "user"}
	for _, u := range []*db.User{owner, other} {
		if err := database.UpsertUser(u); err != nil {
			t.Fatalf("upsert user: %v", err)
		}
	}

	h := &PushHandler{
		DB:    database,
		Keys:  &push.VAPIDKeys{PublicKey: "pub", PrivateKey: "priv"},
		Audit: &audit.Recorder{DB: database},
	}
	return h, owner, other
}

func subscribeRequest(t *testing.T, h *PushHandler, userID, endpoint string) db.PushSubscription {
	t.Helper()
	body, _ := json.Marshal(map[string]any{
		"endpoint":     endpoint,
		"keys":         map[string]string{"p256dh": "p", "auth": "a"},
		"device_label": "iPhone",
	})
	req := withClaims(httptest.NewRequest(http.MethodPost, "/api/push/subscriptions", bytes.NewReader(body)), userID, "user")
	w := httptest.NewRecorder()
	h.Subscribe(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("subscribe status = %d, body = %s", w.Code, w.Body.String())
	}
	var sub db.PushSubscription
	if err := json.Unmarshal(w.Body.Bytes(), &sub); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return sub
}

func TestPushVAPIDKeyDisabledWithoutKeys(t *testing.T) {
	h, owner, _ := newPushTestHandler(t)
	h.Keys = nil

	req := withClaims(httptest.NewRequest(http.MethodGet, "/api/push/vapid-key", nil), owner.ID, "user")
	w := httptest.NewRecorder()
	h.VAPIDKey(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", w.Code)
	}
}

func TestPushSubscribeAndList(t *testing.T) {
	h, owner, _ := newPushTestHandler(t)
	sub := subscribeRequest(t, h, owner.ID, "https://push.example/device-1")
	if sub.DeviceLabel != "iPhone" || !sub.NotifyDeployOutcomes {
		t.Fatalf("unexpected subscription: %+v", sub)
	}

	req := withClaims(httptest.NewRequest(http.MethodGet, "/api/push/subscriptions", nil), owner.ID, "user")
	w := httptest.NewRecorder()
	h.List(w, req)
	var subs []db.PushSubscription
	if err := json.Unmarshal(w.Body.Bytes(), &subs); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(subs) != 1 || subs[0].Endpoint != "https://push.example/device-1" {
		t.Fatalf("unexpected list: %+v", subs)
	}
}

func TestPushSubscribeRejectsNonHTTPSEndpoint(t *testing.T) {
	h, owner, _ := newPushTestHandler(t)
	body, _ := json.Marshal(map[string]any{
		"endpoint": "http://insecure.example/x",
		"keys":     map[string]string{"p256dh": "p", "auth": "a"},
	})
	req := withClaims(httptest.NewRequest(http.MethodPost, "/api/push/subscriptions", bytes.NewReader(body)), owner.ID, "user")
	w := httptest.NewRecorder()
	h.Subscribe(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestPushPreferencesPatchIsPresenceAware(t *testing.T) {
	h, owner, _ := newPushTestHandler(t)
	sub := subscribeRequest(t, h, owner.ID, "https://push.example/device-1")

	// Only mute deploy outcomes; the other toggles must keep their values.
	body, _ := json.Marshal(map[string]any{"notify_deploy_outcomes": false})
	req := withClaims(httptest.NewRequest(http.MethodPatch, "/api/push/subscriptions/"+sub.ID, bytes.NewReader(body)), owner.ID, "user")
	req = req.WithContext(withChiID(req.Context(), "sid", sub.ID))
	w := httptest.NewRecorder()
	h.UpdatePreferences(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var updated db.PushSubscription
	if err := json.Unmarshal(w.Body.Bytes(), &updated); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if updated.NotifyDeployOutcomes {
		t.Fatal("notify_deploy_outcomes should be false")
	}
	if !updated.NotifyBuildStarts || !updated.NotifySSLIssues {
		t.Fatal("omitted toggles must preserve their stored values")
	}
}

func TestPushForeignSubscriptionRejected(t *testing.T) {
	h, owner, other := newPushTestHandler(t)
	sub := subscribeRequest(t, h, owner.ID, "https://push.example/device-1")

	// A different user can neither patch nor delete the subscription.
	body, _ := json.Marshal(map[string]any{"notify_ssl_issues": false})
	patch := withClaims(httptest.NewRequest(http.MethodPatch, "/api/push/subscriptions/"+sub.ID, bytes.NewReader(body)), other.ID, "user")
	patch = patch.WithContext(withChiID(patch.Context(), "sid", sub.ID))
	w := httptest.NewRecorder()
	h.UpdatePreferences(w, patch)
	if w.Code != http.StatusNotFound {
		t.Fatalf("patch status = %d, want 404", w.Code)
	}

	del := withClaims(httptest.NewRequest(http.MethodDelete, "/api/push/subscriptions/"+sub.ID, nil), other.ID, "user")
	del = del.WithContext(withChiID(del.Context(), "sid", sub.ID))
	w = httptest.NewRecorder()
	h.Unsubscribe(w, del)
	if w.Code != http.StatusNotFound {
		t.Fatalf("delete status = %d, want 404", w.Code)
	}
}

func TestPushUnsubscribe(t *testing.T) {
	h, owner, _ := newPushTestHandler(t)
	sub := subscribeRequest(t, h, owner.ID, "https://push.example/device-1")

	req := withClaims(httptest.NewRequest(http.MethodDelete, "/api/push/subscriptions/"+sub.ID, nil), owner.ID, "user")
	req = req.WithContext(withChiID(req.Context(), "sid", sub.ID))
	w := httptest.NewRecorder()
	h.Unsubscribe(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", w.Code)
	}

	list := withClaims(httptest.NewRequest(http.MethodGet, "/api/push/subscriptions", nil), owner.ID, "user")
	w = httptest.NewRecorder()
	h.List(w, list)
	var subs []db.PushSubscription
	json.Unmarshal(w.Body.Bytes(), &subs)
	if len(subs) != 0 {
		t.Fatalf("expected empty list, got %+v", subs)
	}
}

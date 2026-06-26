package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/lefteq/lovinka-deployik/internal/auth"
	"github.com/lefteq/lovinka-deployik/internal/db"
)

func TestAppVariableHandlerUpsertListMasksValue(t *testing.T) {
	database, enc, user := setupProjectTestDB(t)
	org, err := database.EnsurePersonalOrganization(user)
	if err != nil {
		t.Fatalf("EnsurePersonalOrganization: %v", err)
	}
	app, err := database.CreateApp(&db.AppCreate{OrganizationID: org.ID, Name: "Bundle"})
	if err != nil {
		t.Fatalf("CreateApp: %v", err)
	}

	h := &AppVariableHandler{DB: database, Encryptor: enc, Kind: db.VariableKindEnv}
	claims := &auth.Claims{UserID: user.ID, Role: "user"}

	withApp := func(req *http.Request) *http.Request {
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", app.ID)
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
		return req.WithContext(auth.WithClaims(req.Context(), claims))
	}

	// Upsert a shared app env var.
	body, _ := json.Marshal(map[string]string{"key": "DATABASE_URL", "value": "postgres://secret", "environment": "shared"})
	req := withApp(httptest.NewRequest(http.MethodPost, "/apps/x/env", bytes.NewReader(body)))
	rec := httptest.NewRecorder()
	h.Upsert(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("Upsert status = %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}

	// List masks the value.
	listReq := withApp(httptest.NewRequest(http.MethodGet, "/apps/x/env?environment=shared", nil))
	listRec := httptest.NewRecorder()
	h.List(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("List status = %d, want 200", listRec.Code)
	}
	var out []map[string]any
	if err := json.Unmarshal(listRec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(out) != 1 || out[0]["key"] != "DATABASE_URL" {
		t.Fatalf("list = %#v, want one DATABASE_URL", out)
	}
	if out[0]["value"] == "postgres://secret" {
		t.Fatal("value must be masked in the response")
	}

	// Stored value is encrypted and decrypts back to the original.
	stored, err := database.ListAppVariables(app.ID, "shared", db.VariableKindEnv)
	if err != nil {
		t.Fatalf("ListAppVariables: %v", err)
	}
	dec, err := enc.Decrypt(stored[0].Value)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if dec != "postgres://secret" {
		t.Fatalf("decrypted = %q, want postgres://secret", dec)
	}
}

func TestAppVariableHandlerNonMemberRejected(t *testing.T) {
	database, enc, user := setupProjectTestDB(t)
	org, err := database.EnsurePersonalOrganization(user)
	if err != nil {
		t.Fatalf("EnsurePersonalOrganization: %v", err)
	}
	app, err := database.CreateApp(&db.AppCreate{OrganizationID: org.ID, Name: "Bundle"})
	if err != nil {
		t.Fatalf("CreateApp: %v", err)
	}
	stranger := &db.User{ID: db.NewID(), GithubID: 999, Username: "stranger", Role: "user"}
	if err := database.UpsertUser(stranger); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}

	h := &AppVariableHandler{DB: database, Encryptor: enc, Kind: db.VariableKindEnv}
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", app.ID)
	req := httptest.NewRequest(http.MethodGet, "/apps/x/env", nil)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	req = req.WithContext(auth.WithClaims(req.Context(), &auth.Claims{UserID: stranger.ID, Role: "user"}))
	rec := httptest.NewRecorder()
	h.List(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("non-member List status = %d, want 404", rec.Code)
	}
}

package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/LEFTEQ/lovinka-deployik/internal/audit"
	"github.com/LEFTEQ/lovinka-deployik/internal/auth"
	"github.com/LEFTEQ/lovinka-deployik/internal/db"
	"github.com/LEFTEQ/lovinka-deployik/internal/ws"
)

func newDomainHandlerTestSetup(t *testing.T) (*DomainHandler, *db.DB, *db.Project) {
	t.Helper()

	database := newVariableHandlerTestDB(t)
	project := newTestProject(t, database)
	handler := &DomainHandler{
		DB:    database,
		Hub:   ws.NewHub(),
		Audit: &audit.Recorder{DB: database},
	}

	return handler, database, project
}

func patchDomainWithRoute(
	t *testing.T,
	handler *DomainHandler,
	userID, role, projectID, domainID string,
	body map[string]any,
) *httptest.ResponseRecorder {
	t.Helper()

	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	req := httptest.NewRequest(
		http.MethodPatch,
		"/api/projects/"+projectID+"/domains/"+domainID,
		bytes.NewReader(raw),
	)
	req.Header.Set("Content-Type", "application/json")

	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add("id", projectID)
	routeCtx.URLParams.Add("did", domainID)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx))
	req = req.WithContext(auth.WithClaims(req.Context(), &auth.Claims{UserID: userID, Role: role}))

	rec := httptest.NewRecorder()
	handler.Update(rec, req)
	return rec
}

func TestDomainUpdateMovesEnvironment(t *testing.T) {
	handler, database, project := newDomainHandlerTestSetup(t)

	domainRecord := &db.Domain{
		ProjectID:   project.ID,
		DomainName:  "move.example.com",
		Environment: "preview",
		IsPrimary:   true,
		DNSVerified: true,
		SSLStatus:   "active",
	}
	if err := database.CreateDomain(domainRecord); err != nil {
		t.Fatalf("CreateDomain: %v", err)
	}

	rec := patchDomainWithRoute(t, handler, project.UserID, "admin", project.ID, domainRecord.ID, map[string]any{
		"environment": "production",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	got, err := database.GetDomainByID(domainRecord.ID)
	if err != nil {
		t.Fatalf("GetDomainByID: %v", err)
	}
	if got == nil {
		t.Fatal("domain not found after update")
	}
	if got.Environment != "production" {
		t.Fatalf("environment = %q, want production", got.Environment)
	}
	if got.IsPrimary {
		t.Fatalf("moved domain should not remain primary after changing environment")
	}
}

func TestDomainUpdateRejectsAutoDomain(t *testing.T) {
	handler, database, project := newDomainHandlerTestSetup(t)

	autoDomain := &db.Domain{
		ProjectID:   project.ID,
		DomainName:  "domain-test.preview.example.com",
		Environment: "preview",
		IsAuto:      true,
		SSLStatus:   "active",
	}
	if err := database.CreateDomain(autoDomain); err != nil {
		t.Fatalf("CreateDomain: %v", err)
	}

	rec := patchDomainWithRoute(t, handler, project.UserID, "admin", project.ID, autoDomain.ID, map[string]any{
		"environment": "production",
	})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusForbidden, rec.Body.String())
	}
}

func TestDomainUpdateSetsPrimary(t *testing.T) {
	handler, database, project := newDomainHandlerTestSetup(t)

	first := &db.Domain{
		ProjectID:   project.ID,
		DomainName:  "a.example.com",
		Environment: "production",
		SSLStatus:   "active",
		IsPrimary:   true,
	}
	second := &db.Domain{
		ProjectID:   project.ID,
		DomainName:  "b.example.com",
		Environment: "production",
		SSLStatus:   "active",
	}
	if err := database.CreateDomain(first); err != nil {
		t.Fatalf("CreateDomain(first): %v", err)
	}
	if err := database.CreateDomain(second); err != nil {
		t.Fatalf("CreateDomain(second): %v", err)
	}

	rec := patchDomainWithRoute(t, handler, project.UserID, "admin", project.ID, second.ID, map[string]any{
		"is_primary": true,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	list, err := database.ListDomains(project.ID)
	if err != nil {
		t.Fatalf("ListDomains: %v", err)
	}

	var primary string
	primaries := 0
	for _, item := range list {
		if item.Environment == "production" && item.IsPrimary {
			primaries++
			primary = item.DomainName
		}
	}
	if primaries != 1 {
		t.Fatalf("expected exactly 1 primary, got %d", primaries)
	}
	if primary != "b.example.com" {
		t.Fatalf("primary = %q, want b.example.com", primary)
	}
}

func TestDomainUpdateRejectsForeignProject(t *testing.T) {
	handler, database, project := newDomainHandlerTestSetup(t)

	domainRecord := &db.Domain{
		ProjectID:   project.ID,
		DomainName:  "foreign.example.com",
		Environment: "preview",
		SSLStatus:   "active",
	}
	if err := database.CreateDomain(domainRecord); err != nil {
		t.Fatalf("CreateDomain: %v", err)
	}

	stranger := &db.User{ID: db.NewID(), GithubID: 999, Username: "stranger", Role: "user"}
	if err := database.UpsertUser(stranger); err != nil {
		t.Fatalf("UpsertUser(stranger): %v", err)
	}

	rec := patchDomainWithRoute(t, handler, stranger.ID, stranger.Role, project.ID, domainRecord.ID, map[string]any{
		"environment": "production",
	})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

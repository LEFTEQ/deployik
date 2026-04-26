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
	"github.com/LEFTEQ/lovinka-deployik/internal/crypto"
	"github.com/LEFTEQ/lovinka-deployik/internal/db"
	projectemail "github.com/LEFTEQ/lovinka-deployik/internal/email"
)

type handlerFakeEmailSender struct {
	calls int
}

func (h *handlerFakeEmailSender) Send(_ context.Context, _ projectemail.SMTPConfig, _ projectemail.Message) error {
	h.calls++
	return nil
}

func newEmailHandlerRequest(t *testing.T, method, path, projectID, userID string, body any) *http.Request {
	t.Helper()
	var payload []byte
	if body != nil {
		var err error
		payload, err = json.Marshal(body)
		if err != nil {
			t.Fatalf("Marshal: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(payload))
	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add("id", projectID)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx))
	return req.WithContext(auth.WithClaims(req.Context(), &auth.Claims{UserID: userID, Role: "admin"}))
}

func TestProjectEmailHandlerUpdateAndTestSMTP(t *testing.T) {
	database := newVariableHandlerTestDB(t)
	project := newTestProject(t, database)
	encryptor, err := crypto.NewEncryptor("test-key")
	if err != nil {
		t.Fatalf("NewEncryptor: %v", err)
	}
	sender := &handlerFakeEmailSender{}
	handler := &ProjectEmailHandler{
		DB:    database,
		Email: projectemail.NewService(database, encryptor, sender),
		Audit: &audit.Recorder{DB: database},
	}

	updateReq := newEmailHandlerRequest(t, http.MethodPut, "/projects/"+project.ID+"/email", project.ID, project.UserID, map[string]any{
		"smtp_host":                 "mail.webglobe.cz",
		"smtp_port":                 587,
		"smtp_security":             "starttls",
		"smtp_user":                 "noreply@example.com",
		"smtp_password":             "smtp-password",
		"email_from":                "noreply@example.com",
		"email_from_name":           "Example",
		"contact_email_to":          "owner@example.com",
		"recaptcha_site_key":        "site-key",
		"recaptcha_secret_key":      "secret-key",
		"recaptcha_score_threshold": 0.5,
	})
	updateRec := httptest.NewRecorder()
	handler.Update(updateRec, updateReq)
	if updateRec.Code != http.StatusOK {
		t.Fatalf("update status = %d, body: %s", updateRec.Code, updateRec.Body.String())
	}

	testReq := newEmailHandlerRequest(t, http.MethodPost, "/projects/"+project.ID+"/email/test-smtp", project.ID, project.UserID, nil)
	testRec := httptest.NewRecorder()
	handler.TestSMTP(testRec, testReq)
	if testRec.Code != http.StatusOK {
		t.Fatalf("test status = %d, body: %s", testRec.Code, testRec.Body.String())
	}
	if sender.calls != 1 {
		t.Fatalf("sender calls = %d, want 1", sender.calls)
	}

	var payload projectemail.ProjectPayload
	if err := json.NewDecoder(testRec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Settings.Status != string(db.EmailStatusSMTPTested) {
		t.Fatalf("status = %q, want smtp_tested", payload.Settings.Status)
	}

	var count int
	if err := database.QueryRow(
		`SELECT COUNT(*) FROM audit_logs WHERE project_id = ? AND action IN ('email.update', 'email.test_smtp')`,
		project.ID,
	).Scan(&count); err != nil {
		t.Fatalf("count audit logs: %v", err)
	}
	if count != 2 {
		t.Fatalf("audit log count = %d, want 2", count)
	}
}

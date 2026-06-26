package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/lefteq/lovinka-deployik/internal/auth"
	"github.com/lefteq/lovinka-deployik/internal/db"
)

func listPreviewInstances(t *testing.T, handler *PreviewInstanceHandler, userID, role, projectID string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/projects/"+projectID+"/preview-instances", nil)
	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add("id", projectID)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx))
	req = req.WithContext(auth.WithClaims(req.Context(), &auth.Claims{UserID: userID, Role: role}))
	rec := httptest.NewRecorder()
	handler.List(rec, req)
	return rec
}

// Without a Docker client (the test default), List must still return the
// instances and report no attached volume — never panic dereferencing Docker
// during the /system/df enrichment. A panic here would break the whole project
// overview page.
func TestPreviewInstanceListNilDockerReportsNoVolume(t *testing.T) {
	database := newVariableHandlerTestDB(t)
	project := newTestProject(t, database)
	if _, err := database.Exec(`UPDATE projects SET data_volume_enabled = 1 WHERE id = ?`, project.ID); err != nil {
		t.Fatalf("enable data volume: %v", err)
	}
	if _, err := database.GetOrCreatePreviewInstance(project, "feature/data"); err != nil {
		t.Fatalf("GetOrCreatePreviewInstance: %v", err)
	}

	handler := &PreviewInstanceHandler{DB: database} // Docker + Manager nil

	rec := listPreviewInstances(t, handler, project.UserID, "admin", project.ID)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body: %s", rec.Code, rec.Body.String())
	}

	var summaries []db.PreviewInstanceSummary
	if err := json.Unmarshal(rec.Body.Bytes(), &summaries); err != nil {
		t.Fatalf("decode: %v (body: %s)", err, rec.Body.String())
	}
	if len(summaries) != 1 {
		t.Fatalf("summaries = %d, want 1", len(summaries))
	}
	if summaries[0].Branch != "feature/data" {
		t.Fatalf("branch = %q, want feature/data", summaries[0].Branch)
	}
	if summaries[0].VolumeExists {
		t.Fatal("volume_exists should be false when Docker is unavailable")
	}
	if summaries[0].VolumeSizeBytes != 0 {
		t.Fatalf("volume_size_bytes = %d, want 0", summaries[0].VolumeSizeBytes)
	}
}

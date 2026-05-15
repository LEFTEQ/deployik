package db

import (
	"testing"
)

func TestProjectServiceRoundtrip(t *testing.T) {
	t.Parallel()

	database := newTestDB(t)

	user := &User{ID: NewID(), GithubID: 1, Username: "tester", Role: "user"}
	if err := database.UpsertUser(user); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}
	project := &Project{
		Name:        "pg-roundtrip",
		GithubRepo:  "sample",
		GithubOwner: "tester",
		Branch:      "main",
		UserID:      user.ID,
		Framework:   "node-api",
		Port:        3000,
		Status:      "active",
	}
	if err := database.CreateProject(project); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	svc := &ProjectService{
		ProjectID:           project.ID,
		Environment:         "preview",
		ServiceType:         ServiceTypePostgres,
		Image:               "postgres:16",
		DBName:              "app",
		DBUser:              "app",
		DBPasswordEncrypted: "ciphertext-placeholder",
		HostPort:            0,
		ConfigJSON:          "{}",
		Status:              ServiceStatusPending,
	}
	if err := database.CreateService(svc); err != nil {
		t.Fatalf("CreateService: %v", err)
	}
	if svc.ID == "" {
		t.Fatal("CreateService did not assign ID")
	}

	got, err := database.GetService(svc.ID)
	if err != nil {
		t.Fatalf("GetService: %v", err)
	}
	if got == nil {
		t.Fatal("GetService returned nil")
	}
	if got.Environment != "preview" || got.ServiceType != ServiceTypePostgres {
		t.Errorf("roundtrip mismatch: %+v", got)
	}

	// UNIQUE(project_id, environment, service_type) must reject a duplicate.
	dup := *svc
	dup.ID = ""
	if err := database.CreateService(&dup); err == nil {
		t.Fatal("expected UNIQUE constraint to reject duplicate (project, env, type)")
	}

	// GetServiceByProjectEnv looks up via the natural key.
	byKey, err := database.GetServiceByProjectEnv(project.ID, "preview", ServiceTypePostgres)
	if err != nil {
		t.Fatalf("GetServiceByProjectEnv: %v", err)
	}
	if byKey == nil || byKey.ID != svc.ID {
		t.Errorf("GetServiceByProjectEnv missed the row: %+v", byKey)
	}

	// List should return exactly one row.
	list, err := database.ListServicesByProject(project.ID)
	if err != nil {
		t.Fatalf("ListServicesByProject: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 service, got %d", len(list))
	}

	// Update host port + status.
	if err := database.UpdateServiceHostPort(svc.ID, 54321); err != nil {
		t.Fatalf("UpdateServiceHostPort: %v", err)
	}
	if err := database.UpdateServiceStatus(svc.ID, ServiceStatusRunning); err != nil {
		t.Fatalf("UpdateServiceStatus: %v", err)
	}
	got, _ = database.GetService(svc.ID)
	if got.HostPort != 54321 {
		t.Errorf("HostPort after update = %d", got.HostPort)
	}
	if got.Status != ServiceStatusRunning {
		t.Errorf("Status after update = %s", got.Status)
	}
	if got.LastStartedAt.Valid == false {
		t.Error("UpdateServiceHostPort should bump last_started_at")
	}

	// Update password.
	if err := database.UpdateServicePassword(svc.ID, "new-ciphertext"); err != nil {
		t.Fatalf("UpdateServicePassword: %v", err)
	}
	got, _ = database.GetService(svc.ID)
	if got.DBPasswordEncrypted != "new-ciphertext" {
		t.Errorf("password not updated")
	}

	// ServicesExist must report true.
	exists, err := database.ServicesExist(project.ID)
	if err != nil {
		t.Fatalf("ServicesExist: %v", err)
	}
	if !exists {
		t.Fatal("ServicesExist should be true after CreateService")
	}

	// Delete + verify gone.
	if err := database.DeleteService(svc.ID); err != nil {
		t.Fatalf("DeleteService: %v", err)
	}
	got, _ = database.GetService(svc.ID)
	if got != nil {
		t.Fatal("GetService should return nil after DeleteService")
	}
	exists, _ = database.ServicesExist(project.ID)
	if exists {
		t.Fatal("ServicesExist should be false after DeleteService")
	}
}

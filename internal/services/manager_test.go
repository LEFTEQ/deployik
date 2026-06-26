package services

import (
	"context"
	"crypto/rand"
	"testing"
	"time"

	"github.com/lefteq/lovinka-deployik/internal/crypto"
	"github.com/lefteq/lovinka-deployik/internal/db"
)

func newTestDB(t *testing.T) *db.DB {
	t.Helper()
	database, err := db.OpenMemory()
	if err != nil {
		t.Fatalf("OpenMemory: %v", err)
	}
	if err := database.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return database
}

func TestManagerProvisionPersistsRowWithEncryptedPassword(t *testing.T) {
	t.Parallel()

	database := newTestDB(t)
	encryptor, err := crypto.NewEncryptor("dev-encryption-key-32chars-yesssss")
	if err != nil {
		t.Fatalf("NewEncryptor: %v", err)
	}

	user := &db.User{ID: db.NewID(), GithubID: 1, Username: "tester", Role: "user"}
	if err := database.UpsertUser(user); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}
	project := &db.Project{
		Name: "mgr-test", GithubRepo: "x", GithubOwner: "y", Branch: "main",
		UserID: user.ID, Framework: "node-api", Port: 3000, Status: "active",
	}
	if err := database.CreateProject(project); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	mgr := &Manager{DB: database, Encryptor: encryptor, RandReader: rand.Reader}

	spec, err := mgr.Provision(context.Background(), project, "preview", db.ServiceTypePostgres)
	if err != nil {
		t.Fatalf("Provision: %v", err)
	}
	if spec.DBPasswordPlain == "" {
		t.Fatal("Provision returned empty plaintext password")
	}

	// Row persisted with encrypted password (not plaintext).
	row, err := database.GetServiceByProjectEnv(project.ID, "preview", db.ServiceTypePostgres)
	if err != nil {
		t.Fatalf("GetServiceByProjectEnv: %v", err)
	}
	if row == nil {
		t.Fatal("Provision did not persist a row")
	}
	if row.DBPasswordEncrypted == spec.DBPasswordPlain {
		t.Error("password stored as plaintext")
	}
	decrypted, err := encryptor.Decrypt(row.DBPasswordEncrypted)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if decrypted != spec.DBPasswordPlain {
		t.Errorf("decrypted password mismatch")
	}

	// Provision is NOT idempotent on conflict — second call returns ErrAlreadyProvisioned.
	_, err = mgr.Provision(context.Background(), project, "preview", db.ServiceTypePostgres)
	if err != ErrAlreadyProvisioned {
		t.Errorf("expected ErrAlreadyProvisioned, got %v", err)
	}
}

func TestManagerGetSpecLoadsAndDecrypts(t *testing.T) {
	t.Parallel()

	database := newTestDB(t)
	encryptor, err := crypto.NewEncryptor("dev-encryption-key-32chars-yesssss")
	if err != nil {
		t.Fatalf("NewEncryptor: %v", err)
	}

	user := &db.User{ID: db.NewID(), GithubID: 2, Username: "u2", Role: "user"}
	if err := database.UpsertUser(user); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}
	project := &db.Project{Name: "getspec", GithubRepo: "x", GithubOwner: "y", Branch: "main", UserID: user.ID, Framework: "node-api", Port: 3000, Status: "active"}
	if err := database.CreateProject(project); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	mgr := &Manager{DB: database, Encryptor: encryptor, RandReader: rand.Reader}
	provisioned, err := mgr.Provision(context.Background(), project, "production", db.ServiceTypePostgres)
	if err != nil {
		t.Fatalf("Provision: %v", err)
	}

	loaded, err := mgr.GetSpec(project, "production", db.ServiceTypePostgres)
	if err != nil {
		t.Fatalf("GetSpec: %v", err)
	}
	if loaded == nil {
		t.Fatal("GetSpec returned nil")
	}
	if loaded.DBPasswordPlain != provisioned.DBPasswordPlain {
		t.Errorf("GetSpec password mismatch")
	}
	if loaded.ContainerName != PostgresContainerName(project.Name, "production") {
		t.Errorf("ContainerName wrong: %s", loaded.ContainerName)
	}

	// GetSpec for an attached-but-different env returns nil.
	missing, err := mgr.GetSpec(project, "preview", db.ServiceTypePostgres)
	if err != nil {
		t.Fatalf("GetSpec (missing): %v", err)
	}
	if missing != nil {
		t.Error("GetSpec for missing env should return nil")
	}

	_ = time.Now()
}

func TestManagerEnsureForDeploymentNoServiceReturnsNilNil(t *testing.T) {
	t.Parallel()

	database := newTestDB(t)
	encryptor, err := crypto.NewEncryptor("dev-encryption-key-32chars-yesssss")
	if err != nil {
		t.Fatalf("NewEncryptor: %v", err)
	}

	user := &db.User{ID: db.NewID(), GithubID: 3, Username: "u3", Role: "user"}
	if err := database.UpsertUser(user); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}
	project := &db.Project{
		Name: "no-svc", GithubRepo: "x", GithubOwner: "y", Branch: "main",
		UserID: user.ID, Framework: "node-api", Port: 3000, Status: "active",
	}
	if err := database.CreateProject(project); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	// No Provision call — no service attached. Manager has nil Docker
	// (handler-test style) since this path returns before touching docker.
	mgr := &Manager{DB: database, Encryptor: encryptor}

	inj, err := mgr.EnsureForDeployment(context.Background(), project, "preview")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inj != nil {
		t.Errorf("expected nil EnvInjection when no service attached, got %+v", inj)
	}
}

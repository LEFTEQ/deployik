package services

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log"

	"github.com/lefteq/lovinka-deployik/internal/build"
	"github.com/lefteq/lovinka-deployik/internal/crypto"
	"github.com/lefteq/lovinka-deployik/internal/db"
	"github.com/docker/docker/errdefs"
)

// ErrAlreadyProvisioned is returned by Provision when the (project, env,
// service_type) row already exists. Handlers surface as 409 Conflict.
var ErrAlreadyProvisioned = errors.New("service already provisioned for this environment")

// Manager is the DB-aware facade for sidecar lifecycle. Handlers and the
// deploy pipeline construct one of these in cmd/server/main.go and inject it.
type Manager struct {
	DB           *db.DB
	Encryptor    *crypto.Encryptor
	Docker       *build.DockerClient
	ProxyNetwork string

	// RandReader is the source of entropy for password generation. Defaults
	// to crypto/rand.Reader; tests can override for determinism.
	RandReader io.Reader
}

// passwordBytes is the entropy length for generated Postgres passwords.
// 32 bytes → 43-character base64url password (no padding). Sufficient against
// online attacks; the DB is never internet-exposed.
const passwordBytes = 32

// generatePassword returns a base64url-encoded random password of fixed length.
// Using base64url means the raw string is safe in DATABASE_URL without
// percent-encoding (no /, +, or = chars). DSN composition still uses
// url.UserPassword for defense-in-depth (Task 6).
func (m *Manager) generatePassword() (string, error) {
	reader := m.RandReader
	if reader == nil {
		reader = rand.Reader
	}
	buf := make([]byte, passwordBytes)
	if _, err := io.ReadFull(reader, buf); err != nil {
		return "", fmt.Errorf("read random: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// GeneratePasswordForReset is the exported version of generatePassword.
// Used by handlers that need a fresh password without going through the
// full Provision flow (regenerate-password endpoint).
func (m *Manager) GeneratePasswordForReset() (string, error) {
	return m.generatePassword()
}

// Provision inserts a new project_services row for the given (project, env, type)
// with a freshly-generated encrypted password. Does NOT start the container —
// EnsureForDeployment / the API restart endpoint do that. Returns
// ErrAlreadyProvisioned if a row already exists.
func (m *Manager) Provision(ctx context.Context, project *db.Project, environment string, svcType db.ServiceType) (*ServiceSpec, error) {
	existing, err := m.DB.GetServiceByProjectEnv(project.ID, environment, svcType)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, ErrAlreadyProvisioned
	}

	password, err := m.generatePassword()
	if err != nil {
		return nil, err
	}
	encrypted, err := m.Encryptor.Encrypt(password)
	if err != nil {
		return nil, fmt.Errorf("encrypt pg password: %w", err)
	}

	row := &db.ProjectService{
		ProjectID:           project.ID,
		Environment:         environment,
		ServiceType:         svcType,
		Image:               PostgresImage,
		DBName:              "app",
		DBUser:              "app",
		DBPasswordEncrypted: encrypted,
		HostPort:            0,
		ConfigJSON:          "{}",
		Status:              db.ServiceStatusPending,
	}
	if err := m.DB.CreateService(row); err != nil {
		return nil, fmt.Errorf("persist pg service: %w", err)
	}

	return m.specFromRow(project, row, password), nil
}

// GetSpec loads the persisted row, decrypts the password, and assembles a
// ready-to-use ServiceSpec. Returns (nil, nil) when no row exists for the
// (project, env, type) tuple.
func (m *Manager) GetSpec(project *db.Project, environment string, svcType db.ServiceType) (*ServiceSpec, error) {
	row, err := m.DB.GetServiceByProjectEnv(project.ID, environment, svcType)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, nil
	}
	password, err := m.Encryptor.Decrypt(row.DBPasswordEncrypted)
	if err != nil {
		return nil, fmt.Errorf("decrypt pg password: %w", err)
	}
	return m.specFromRow(project, row, password), nil
}

// EnsureForDeployment is called from pipeline.go Step 4b. If a service is
// attached for (project, env), it ensures the container is running and
// returns the EnvInjection. When no service is attached it returns (nil, nil)
// — the deploy proceeds without DB env injection.
//
// On failure the deployment should abort BEFORE building the image so we
// don't waste compute on a broken DB dependency.
func (m *Manager) EnsureForDeployment(ctx context.Context, project *db.Project, environment string) (*EnvInjection, error) {
	spec, err := m.GetSpec(project, environment, db.ServiceTypePostgres)
	if err != nil {
		return nil, err
	}
	if spec == nil {
		return nil, nil
	}
	if err := EnsureRunning(ctx, m.Docker, m.ProxyNetwork, spec); err != nil {
		_ = m.DB.UpdateServiceStatus(spec.ServiceID, db.ServiceStatusFailed)
		return nil, err
	}
	if err := m.DB.UpdateServiceHostPort(spec.ServiceID, spec.HostPort); err != nil {
		return nil, fmt.Errorf("persist pg host_port: %w", err)
	}
	if err := m.DB.UpdateServiceStatus(spec.ServiceID, db.ServiceStatusRunning); err != nil {
		return nil, fmt.Errorf("persist pg status: %w", err)
	}
	inj := BuildPostgresEnvInjection(*spec)
	return &inj, nil
}

// Delete stops the container, removes its volume, then deletes the row.
// Safe to call when the container doesn't exist; volume errors other than
// NotFound surface as wrapped errors.
func (m *Manager) Delete(ctx context.Context, project *db.Project, environment string, svcType db.ServiceType) error {
	spec, err := m.GetSpec(project, environment, svcType)
	if err != nil {
		return err
	}
	if spec == nil {
		return nil
	}
	if m.Docker != nil {
		if err := Stop(ctx, m.Docker, spec); err != nil {
			return fmt.Errorf("stop service: %w", err)
		}
		if err := m.Docker.RemoveVolume(ctx, spec.VolumeName); err != nil {
			// errdefs.IsNotFound is OK — volume already gone or never created.
			// All other errors (in-use conflict, etc.) should propagate.
			if !errdefs.IsNotFound(err) {
				return fmt.Errorf("remove volume: %w", err)
			}
		}
	} else {
		log.Printf("services.Manager.Delete: Docker is nil — DB row %s deleted, container %s and volume %s NOT cleaned up. This is a wiring bug if seen in production.",
			spec.ServiceID, spec.ContainerName, spec.VolumeName)
	}
	return m.DB.DeleteService(spec.ServiceID)
}

// specFromRow assembles a ServiceSpec from a db row + already-decrypted password.
func (m *Manager) specFromRow(project *db.Project, row *db.ProjectService, password string) *ServiceSpec {
	return &ServiceSpec{
		ServiceID:       row.ID,
		ProjectID:       project.ID,
		ProjectName:     project.Name,
		Environment:     row.Environment,
		Type:            row.ServiceType,
		Image:           row.Image,
		DBName:          row.DBName,
		DBUser:          row.DBUser,
		DBPasswordPlain: password,
		HostPort:        row.HostPort,
		ContainerName:   PostgresContainerName(project.Name, row.Environment),
		VolumeName:      PostgresVolumeName(project.Name, row.Environment),
	}
}
